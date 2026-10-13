// Package app builds the application and runs it.
package app

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/pprof"
	"os"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/rs/zerolog"

	"github.com/mster429/web-page-analyzer/internal/adapter/fetch"
	"github.com/mster429/web-page-analyzer/internal/adapter/linkcheck"
	"github.com/mster429/web-page-analyzer/internal/adapter/linkstore"
	"github.com/mster429/web-page-analyzer/internal/adapter/web"
	"github.com/mster429/web-page-analyzer/internal/analysis"
	"github.com/mster429/web-page-analyzer/internal/config"
)

const (
	linkStoreSize = 1000
	linkStoreTTL  = 2 * time.Minute
)

// App holds what the server needs to run.
type App struct {
	cfg      config.Config
	logger   zerolog.Logger
	registry *prometheus.Registry
	server   *http.Server
}

// New loads the config and does the initial setup of the application
func New() (*App, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, fmt.Errorf("load config: %w", err)
	}

	logger := zerolog.New(os.Stdout).Level(cfg.LogLevel).With().Timestamp().Logger()

	registry := prometheus.NewRegistry()
	registry.MustRegister(
		collectors.NewGoCollector(),
		collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
	)

	client := &http.Client{}
	fetcher := fetch.New(client, cfg.PageFetchTimeout, int64(cfg.PageSizeLimitMB)<<20)
	checker := linkcheck.New(client, cfg.LinkCheckWorkers, cfg.SingleLinkCheckTimeout)
	store := linkstore.New(linkStoreSize, linkStoreTTL)
	service := analysis.NewService(fetcher, checker, store, cfg.TotalLinkCheckTimeout)
	pages := web.New(service, logger, registry)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", pages.Index)
	mux.Handle("GET /static/", pages.Static())
	mux.Handle("GET /metrics", promhttp.HandlerFor(registry, promhttp.HandlerOpts{}))
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc("GET /analyze", pages.Analyze)
	mux.HandleFunc("GET /analyze/links", pages.Links)

	if cfg.EnableProfiling {
		mux.HandleFunc("GET /debug/pprof/", pprof.Index)
		mux.HandleFunc("GET /debug/pprof/cmdline", pprof.Cmdline)
		mux.HandleFunc("GET /debug/pprof/profile", pprof.Profile)
		mux.HandleFunc("GET /debug/pprof/symbol", pprof.Symbol)
		mux.HandleFunc("GET /debug/pprof/trace", pprof.Trace)
	}

	server := &http.Server{
		Handler:           pages.Middleware(mux),
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       60 * time.Second,
		WriteTimeout:      max(cfg.PageFetchTimeout, cfg.TotalLinkCheckTimeout) + 10*time.Second,
	}

	return &App{cfg: cfg, logger: logger, registry: registry, server: server}, nil
}

// Run serves until ctx is canceled, then shuts down gracefully.
func (a *App) Run(ctx context.Context) error {
	var listenConfig net.ListenConfig
	listener, err := listenConfig.Listen(ctx, "tcp", a.cfg.HostPort)
	if err != nil {
		return fmt.Errorf("listen on %s: %w", a.cfg.HostPort, err)
	}
	a.logger.Info().Str("addr", listener.Addr().String()).Bool("pprof", a.cfg.EnableProfiling).Msg("server started")

	serveErr := make(chan error, 1)
	go func() { serveErr <- a.server.Serve(listener) }()

	select {
	case err = <-serveErr:
		return fmt.Errorf("server exited while running: %w", err)
	case <-ctx.Done():
	}

	a.logger.Info().Msg("server shutting down")
	// total analyze timeout is 30s (default) + 5 extra seconds
	// docker stop_grace_period/stop timeout increased to 40s
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 35*time.Second)
	defer cancel()
	if err = a.server.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("server shutdown failed: %w", err)
	}
	if err = <-serveErr; !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("serve: %w", err)
	}
	return nil
}
