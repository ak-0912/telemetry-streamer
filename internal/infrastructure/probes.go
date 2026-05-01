package infrastructure

import (
	"context"
	"errors"
	"log"
	"net/http"
	"sync/atomic"
	"time"

	"go.uber.org/fx"
	"telemetry-streamer/internal/infrastructure/config"
	"telemetry-streamer/internal/observability"
)

func runProbeServer(lc fx.Lifecycle, cfg config.Config) {
	var ready atomic.Bool

	mux := http.NewServeMux()
	mux.Handle("/metrics", observability.MetricsHandler())
	mux.HandleFunc("/livez", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	mux.HandleFunc("/readyz", func(w http.ResponseWriter, _ *http.Request) {
		if !ready.Load() {
			http.Error(w, "not ready", http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ready"))
	})

	server := &http.Server{
		Addr:              cfg.ProbeAddr,
		Handler:           mux,
		ReadHeaderTimeout: 2 * time.Second,
	}

	lc.Append(fx.Hook{
		OnStart: func(context.Context) error {
			ready.Store(true)
			go func() {
				if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
					log.Printf("probe server stopped with error: %v", err)
				}
			}()
			return nil
		},
		OnStop: func(ctx context.Context) error {
			ready.Store(false)
			return server.Shutdown(ctx)
		},
	})
}
