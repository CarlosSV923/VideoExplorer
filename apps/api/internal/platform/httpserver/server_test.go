package httpserver_test

import (
	"context"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/platform/httpserver"
)

func TestNewKeepsTransferTimeoutsOpen(t *testing.T) {
	t.Parallel()

	srv := httpserver.New(":0", http.NewServeMux())

	if srv.ReadTimeout != 0 || srv.WriteTimeout != 0 {
		t.Fatalf("read/write timeouts must be 0 for large transfers, got %v/%v", srv.ReadTimeout, srv.WriteTimeout)
	}
	if srv.ReadHeaderTimeout == 0 {
		t.Fatal("ReadHeaderTimeout must be set")
	}
}

func TestServeShutsDownOnCancel(t *testing.T) {
	t.Parallel()

	var lc net.ListenConfig
	ln, err := lc.Listen(t.Context(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /ping", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	srv := httpserver.New(ln.Addr().String(), mux)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- httpserver.Serve(ctx, srv, ln, 5*time.Second) }()

	client := &http.Client{Timeout: 5 * time.Second}
	url := "http://" + ln.Addr().String() + "/ping"
	if status := get(t, client, url); status != http.StatusNoContent {
		t.Fatalf("before shutdown: status = %d, want 204", status)
	}

	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Serve after cancel: %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("Serve did not return after the context was cancelled")
	}

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, url, nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	if resp, err := client.Do(req); err == nil {
		_ = resp.Body.Close()
		t.Fatal("server still accepts connections after shutdown")
	}
}

func get(t *testing.T, client *http.Client, url string) int {
	t.Helper()
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, url, nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("GET %s: %v", url, err)
	}
	defer func() { _ = resp.Body.Close() }()
	return resp.StatusCode
}
