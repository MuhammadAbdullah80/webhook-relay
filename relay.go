package main

import (
	"bytes"
	"io"
	"log"
	"net/http"
	"sync"
	"time"
)

// maxBody caps how much of an inbound request we buffer. Webhooks are small;
// anything larger is refused rather than held in memory across every target.
const maxBody = 1 << 20 // 1 MiB

// Relay fans each inbound payload out to every configured target. Each target
// owns an independent buffered queue, so one unhealthy downstream applies
// backpressure only to itself.
type Relay struct {
	client *http.Client
	queues []chan []byte
	wg     sync.WaitGroup
}

// NewRelay starts `workers` delivery goroutines for each target URL.
func NewRelay(targets []string, workers int, client *http.Client) *Relay {
	if workers < 1 {
		workers = 1
	}
	r := &Relay{client: client, queues: make([]chan []byte, len(targets))}
	for i, target := range targets {
		q := make(chan []byte, 256)
		r.queues[i] = q
		for w := 0; w < workers; w++ {
			r.wg.Add(1)
			go func(url string, in <-chan []byte) {
				defer r.wg.Done()
				for payload := range in {
					r.deliver(url, payload)
				}
			}(target, q)
		}
	}
	return r
}

// Handle accepts an inbound webhook and enqueues it for every target. It
// answers 202 as soon as the payload is queued: delivery is asynchronous, so
// the sender is never held open waiting on downstream systems.
func (r *Relay) Handle(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, req.Body, maxBody))
	if err != nil {
		http.Error(w, "payload too large", http.StatusRequestEntityTooLarge)
		return
	}
	dropped := 0
	for _, q := range r.queues {
		select {
		case q <- body:
		default:
			dropped++ // queue full: shed rather than block the listener
		}
	}
	if dropped > 0 {
		log.Printf("relay: shed payload for %d saturated target(s)", dropped)
	}
	w.WriteHeader(http.StatusAccepted)
}

// deliver POSTs one payload with bounded exponential backoff. 4xx responses
// other than 429 are permanent, so they are not retried.
func (r *Relay) deliver(url string, payload []byte) {
	backoff := 200 * time.Millisecond
	for attempt := 1; attempt <= 5; attempt++ {
		resp, err := r.client.Post(url, "application/json", bytes.NewReader(payload))
		if err == nil {
			code := resp.StatusCode
			_, _ = io.Copy(io.Discard, resp.Body)
			_ = resp.Body.Close()
			if code < 300 {
				return
			}
			if code < 500 && code != http.StatusTooManyRequests {
				log.Printf("relay: %s rejected payload permanently (%d)", url, code)
				return
			}
		}
		if attempt < 5 {
			time.Sleep(backoff)
			backoff *= 2
		}
	}
	log.Printf("relay: giving up on %s after 5 attempts", url)
}

// Shutdown closes every queue and waits for in-flight deliveries to finish.
func (r *Relay) Shutdown() {
	for _, q := range r.queues {
		close(q)
	}
	r.wg.Wait()
}
