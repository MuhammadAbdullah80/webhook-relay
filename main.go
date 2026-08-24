// Command webhook-relay fans a single inbound webhook out to many downstream
// targets, retrying each one independently so a slow consumer can never block
// or drop delivery for the others.
package main

import (
	"context"
	"errors"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"
)

// shutdownGrace bounds how long a permanently dead target may hold up exit.
const shutdownGrace = 20 * time.Second

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

	srv := &http.Server{
		Addr:              *addr,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}

	// Stop accepting on SIGINT/SIGTERM, then drain: the listener closes first so
	// no new payloads arrive, and the deferred relay.Shutdown flushes whatever is
	// still queued before the process exits.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	errCh := make(chan error, 1)
	go func() {
		log.Printf("webhook-relay listening on %s, fanning out to %d target(s)", *addr, len(targets))
		errCh <- srv.ListenAndServe()
	}()

	select {
	case err := <-errCh:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("webhook-relay: %v", err)
		}
	case <-ctx.Done():
		log.Print("webhook-relay: signal received, draining")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownGrace)
		defer cancel()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			log.Printf("webhook-relay: listener did not close cleanly: %v", err)
		}
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
