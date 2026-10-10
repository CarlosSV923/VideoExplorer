package database_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/platform/database"
	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/platform/database/sqlcgen"
)

func TestOpenMigratesAndUsesWAL(t *testing.T) {
	t.Parallel()

	conn, err := database.Open(t.Context(), t.TempDir())
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	// goose records version 0 as well; only the real migrations (version > 0) count.
	var migrations int
	if err := conn.QueryRowContext(t.Context(), "SELECT count(*) FROM goose_db_version WHERE is_applied AND version_id > 0").Scan(&migrations); err != nil || migrations < 1 {
		t.Fatalf("goose_db_version: %d applied migrations, %v", migrations, err)
	}

	var journal string
	if err := conn.QueryRowContext(t.Context(), "PRAGMA journal_mode").Scan(&journal); err != nil {
		t.Fatalf("journal_mode: %v", err)
	}
	if journal != "wal" {
		t.Errorf("journal_mode = %q, want wal", journal)
	}

	ok, err := sqlcgen.New(conn).Ping(t.Context())
	if err != nil || ok != 1 {
		t.Fatalf("Ping = %d, %v; want 1, nil", ok, err)
	}
}

func TestOpenIsIdempotent(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	for range 2 {
		conn, err := database.Open(t.Context(), dir)
		if err != nil {
			t.Fatalf("Open: %v", err)
		}
		_ = conn.Close()
	}
	if _, err := os.Stat(filepath.Join(dir, database.FileName)); err != nil {
		t.Errorf("database file not created: %v", err)
	}
}

func TestOpenFailsWhenDataPathIsAFile(t *testing.T) {
	t.Parallel()

	file := filepath.Join(t.TempDir(), "not-a-dir")
	if err := os.WriteFile(file, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if conn, err := database.Open(t.Context(), file); err == nil {
		_ = conn.Close()
		t.Fatal("Open on a file path: expected an error")
	}
}
