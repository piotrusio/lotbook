// Package server runs a role's HTTP server with the probes Kubernetes asks of
// every pod, and stops it the way a rolling update needs.
package server

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"sync/atomic"
	"time"
)

// Config says what a server serves besides its probes and how it stops.
type Config struct {
	// Handler serves every path but the probes; nil serves none.
	Handler http.Handler
	// Drain is how long the server goes on serving once it is told to stop,
	// answering /readyz with 503, so that traffic is routed away before it
	// stops accepting.
	Drain time.Duration
	// Grace bounds how long the requests in flight may take to finish once
	// the server stops accepting new ones.
	Grace time.Duration
}

// Run serves on ln until ctx is done, then drains and stops. /healthz answers
// 200 for as long as the server serves; /readyz answers 200 until it begins to
// drain and 503 after. Run returns nil once every request in flight has
// finished, and an error when one outlives the grace or the server fails.
func Run(ctx context.Context, ln net.Listener, cfg Config) error {
	var draining atomic.Bool
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc("GET /readyz", func(w http.ResponseWriter, _ *http.Request) {
		if draining.Load() {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
	})
	if cfg.Handler != nil {
		mux.Handle("/", cfg.Handler)
	}

	srv := &http.Server{Handler: mux, ReadHeaderTimeout: 10 * time.Second}
	served := make(chan error, 1)
	go func() { served <- srv.Serve(ln) }()

	select {
	case err := <-served:
		return fmt.Errorf("server: %w", err)
	case <-ctx.Done():
	}

	draining.Store(true)
	select {
	case err := <-served:
		return fmt.Errorf("server: %w", err)
	case <-time.After(cfg.Drain):
	}

	stopping, cancel := context.WithTimeout(context.WithoutCancel(ctx), cfg.Grace)
	defer cancel()
	if err := srv.Shutdown(stopping); err != nil {
		return errors.Join(
			fmt.Errorf("server: requests in flight outlived the grace of %s: %w", cfg.Grace, err),
			srv.Close(),
		)
	}
	if err := <-served; !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("server: %w", err)
	}
	return nil
}
