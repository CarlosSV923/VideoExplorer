// Command videoexplorer runs the VideoExplorer HTTP API.
//
// This is the composition root: configuration, adapters and bounded contexts
// are wired here and nowhere else. It opens the database and serves the API
// contract (routes.go) and the embedded SPA. Sign-in (identity.go) is wired
// here; the other bounded contexts and the diagnostics arrive in later phases.
//
// Usage:
//
//	videoexplorer                run the server (configured by environment, see .env.example)
//	videoexplorer hash-password  read a password from stdin, print its argon2id hash
package main

import (
	"bufio"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	identityinfra "github.com/CarlosSV923/VideoExplorer/apps/api/internal/identity/infrastructure"
	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/platform/config"
	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/platform/database"
	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/platform/httpserver"
)

const shutdownTimeout = 30 * time.Second

func main() {
	if len(os.Args) > 1 && os.Args[1] == "hash-password" {
		if err := hashPassword(os.Stdin, os.Stdout); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}

	bootLog := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	cfg, err := config.Load(nil)
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

// run opens the database, serves until SIGINT or SIGTERM, then shuts down in order.
func run(cfg config.Config, log *slog.Logger) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	conn, err := database.Open(ctx, cfg.DataPath)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer closeDatabase(conn, log)
	log.Info("database ready", "data", cfg.DataPath)

	identity, err := newIdentity(cfg)
	if err != nil {
		return err
	}

	srv := httpserver.New(net.JoinHostPort("", strconv.Itoa(cfg.Port)), routes(log, identity))

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

func closeDatabase(conn *sql.DB, log *slog.Logger) {
	if err := conn.Close(); err != nil {
		log.Warn("close database", "error", err)
	}
}

// hashPassword reads one line from in and writes its argon2id PHC hash to out.
func hashPassword(in io.Reader, out io.Writer) error {
	line, err := bufio.NewReader(in).ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return err
	}
	password := strings.TrimRight(line, "\r\n")
	if password == "" {
		return errors.New("empty password: pipe it on stdin, e.g. echo 'my password' | videoexplorer hash-password")
	}
	phc, err := identityinfra.HashPassword(password)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintln(out, phc)
	return err
}
