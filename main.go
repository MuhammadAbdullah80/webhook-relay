// Command webhook-relay fans a single inbound webhook out to many downstream
// targets, retrying each one independently so a slow consumer can never block
// or drop delivery for the others.
package main

import (
	"flag"
	"log"
	"net/http"
	"os"
	"strings"
	"time"
)

func main() {
	addr := flag.String("addr", ":8080", "listen address")
	targetList := flag.String("targets", "", "comma-separated downstream URLs")
	workers := flag.Int("workers", 4, "delivery workers per target")
	flag.Parse()

	if env := os.Getenv("RELAY_TARGETS"); *targetList == "" && env != "" {
		*targetList = env
	}
	targets := splitTargets(*targetList)
	if len(targets) == 0 {
		log.Fatal("webhook-relay: no targets configured (use -targets or RELAY_TARGETS)")
	}

	relay := NewRelay(targets, *workers, &http.Client{Timeout: 10 * time.Second})
	defer relay.Shutdown()

	mux := http.NewServeMux()
	mux.HandleFunc("/hook", relay.Handle)
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	log.Printf("webhook-relay listening on %s, fanning out to %d target(s)", *addr, len(targets))
	srv := &http.Server{
		Addr:              *addr,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("webhook-relay: %v", err)
	}
}

func splitTargets(raw string) []string {
	var out []string
	for _, part := range strings.Split(raw, ",") {
		if t := strings.TrimSpace(part); t != "" {
			out = append(out, t)
		}
	}
	return out
}
