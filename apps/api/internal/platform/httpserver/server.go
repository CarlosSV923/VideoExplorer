// Package httpserver builds the HTTP server shared by every bounded context
// and runs it until the process is asked to stop.
package httpserver

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"time"
)

// New returns an HTTP server tuned for long-lived transfers.
//
// ReadTimeout and WriteTimeout stay at zero on purpose: uploads (tus chunks)
// and downloads of multi-gigabyte files can legitimately take a long time.
// Slow-client protection comes from ReadHeaderTimeout and IdleTimeout instead.
func New(addr string, handler http.Handler) *http.Server {
	return &http.Server{
		Addr:              addr,
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       120 * time.Second,
	}
}

// Serve accepts connections on ln until ctx is cancelled, then shuts srv down
// gracefully: in-flight requests get up to timeout to finish, after which the
// remaining connections are closed. It returns nil on a clean shutdown.
func Serve(ctx context.Context, srv *http.Server, ln net.Listener, timeout time.Duration) error {
	errCh := make(chan error, 1)
	go func() { errCh <- srv.Serve(ln) }()

	select {
	case err := <-errCh:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-ctx.Done():
	}

	// The parent context is already cancelled; the shutdown needs its own deadline.
	shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), timeout)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		_ = srv.Close()
		return fmt.Errorf("graceful shutdown: %w", err)
	}
	if err := <-errCh; err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}
