package sqlite_test

import (
	"testing"
	"time"

	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/metadata/application"
	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/metadata/infrastructure/sqlite"
	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/platform/database"
)

func TestResponseStoreRoundTrip(t *testing.T) {
	t.Parallel()

	conn, err := database.Open(t.Context(), t.TempDir())
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	store := sqlite.NewResponseStore(conn)
	ctx := t.Context()

	expires := time.Date(2026, time.October, 11, 12, 0, 0, 0, time.UTC)
	const key = "v1 movie/1726?lang=es-MX"

	if _, found, err := store.Get(ctx, key); err != nil || found {
		t.Fatalf("Get on an empty store = found %v, err %v; want not found and no error", found, err)
	}

	first := application.CachedResponse{Payload: []byte(`{"title":"Iron Man"}`), ExpiresAt: expires}
	if err := store.Put(ctx, key, first); err != nil {
		t.Fatalf("Put: %v", err)
	}
	got, found, err := store.Get(ctx, key)
	if err != nil || !found {
		t.Fatalf("Get after Put = found %v, err %v", found, err)
	}
	if string(got.Payload) != string(first.Payload) {
		t.Errorf("payload = %q, want %q", got.Payload, first.Payload)
	}
	if !got.ExpiresAt.Equal(expires) {
		t.Errorf("ExpiresAt = %v, want %v", got.ExpiresAt, expires)
	}

	// A second Put under the same key replaces the entry instead of adding one.
	second := application.CachedResponse{Payload: []byte(`{"title":"Iron Man 2"}`), ExpiresAt: expires.Add(time.Hour)}
	if err := store.Put(ctx, key, second); err != nil {
		t.Fatalf("second Put: %v", err)
	}
	got, _, err = store.Get(ctx, key)
	if err != nil {
		t.Fatal(err)
	}
	if string(got.Payload) != string(second.Payload) || !got.ExpiresAt.Equal(second.ExpiresAt) {
		t.Errorf("after the replacement = %q at %v, want %q at %v", got.Payload, got.ExpiresAt, second.Payload, second.ExpiresAt)
	}
}

func TestResponseStoreKeysAreSeparate(t *testing.T) {
	t.Parallel()

	conn, err := database.Open(t.Context(), t.TempDir())
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	store := sqlite.NewResponseStore(conn)
	ctx := t.Context()

	at := time.Date(2026, time.October, 11, 0, 0, 0, 0, time.UTC)
	if err := store.Put(ctx, "v1 movie/1?lang=es-MX", application.CachedResponse{Payload: []byte("es"), ExpiresAt: at}); err != nil {
		t.Fatal(err)
	}
	if err := store.Put(ctx, "v1 movie/1?lang=en-US", application.CachedResponse{Payload: []byte("en"), ExpiresAt: at}); err != nil {
		t.Fatal(err)
	}

	for key, want := range map[string]string{"v1 movie/1?lang=es-MX": "es", "v1 movie/1?lang=en-US": "en"} {
		got, found, err := store.Get(ctx, key)
		if err != nil || !found || string(got.Payload) != want {
			t.Errorf("Get(%q) = %q, found %v, err %v; want %q", key, got.Payload, found, err, want)
		}
	}
}
