// Command videoexplorer runs the VideoExplorer HTTP API.
//
// This is the composition root: configuration, adapters and bounded contexts
// are wired here and nowhere else. Phase 0 serves only the liveness probe; the
// SPA, the bounded contexts and the persistence arrive in later phases.
//
// Usage:
//
//	videoexplorer    run the server (PORT, LOG_LEVEL)
package main

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/platform/config"
	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/platform/httpserver"
	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/platform/system"
)

const shutdownTimeout = 30 * time.Second

func main() {
	bootLog := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	cfg, err := config.Load(os.Getenv)
	if err != nil {
		bootLog.Error("invalid configuration", "error", err)
		os.Exit(1)
	}
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: cfg.LogLevel}))

	if err := run(cfg, log); err != nil {
		log.Error("server stopped with error", "error", err)
		os.Exit(1)
	}
}

// run serves until SIGINT or SIGTERM, then shuts down in order.
func run(cfg config.Config, log *slog.Logger) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	mux := http.NewServeMux()
	system.RegisterRoutes(mux)

	srv := httpserver.New(net.JoinHostPort("", strconv.Itoa(cfg.Port)), mux)

	var lc net.ListenConfig
	ln, err := lc.Listen(ctx, "tcp", srv.Addr)
	if err != nil {
		return fmt.Errorf("listen on %s: %w", srv.Addr, err)
	}
	log.Info("listening", "addr", ln.Addr().String())

	serveErr := make(chan error, 1)
	go func() { serveErr <- httpserver.Serve(ctx, srv, ln, shutdownTimeout) }()

	select {
	case err := <-serveErr:
		return err
	case <-ctx.Done():
		log.Info("shutting down")
	}
	if err := <-serveErr; err != nil {
		return err
	}
	log.Info("stopped")
	return nil
}
