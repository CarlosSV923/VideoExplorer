// Package database opens the SQLite database and applies its migrations.
package database

import (
	"context"
	"database/sql"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/pressly/goose/v3"
	_ "modernc.org/sqlite" // pure-Go driver, registered as "sqlite"

	"github.com/CarlosSV923/VideoExplorer/apps/api/db"
)

// FileName is the database file inside DATA_PATH.
const FileName = "videoexplorer.db"

// Open opens (creating if needed) the database in dataDir with WAL journaling
// and migrates it to the latest schema.
//
// A single connection is used on purpose: SQLite serializes writers anyway,
// and one connection avoids SQLITE_BUSY between our own goroutines.
func Open(ctx context.Context, dataDir string) (*sql.DB, error) {
	if err := os.MkdirAll(dataDir, 0o750); err != nil {
		return nil, fmt.Errorf("database: create data dir: %w", err)
	}

	dsn := "file:" + filepath.ToSlash(filepath.Join(dataDir, FileName)) +
		"?_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)&_pragma=synchronous(NORMAL)"
	conn, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("database: open: %w", err)
	}
	conn.SetMaxOpenConns(1)

	if err := Migrate(ctx, conn, db.Migrations); err != nil {
		_ = conn.Close()
		return nil, err
	}
	return conn, nil
}

// Migrate applies every pending migration found under migrations/ in fsys.
func Migrate(ctx context.Context, conn *sql.DB, fsys fs.FS) error {
	migrations, err := fs.Sub(fsys, "migrations")
	if err != nil {
		return fmt.Errorf("database: migrations dir: %w", err)
	}
	provider, err := goose.NewProvider(goose.DialectSQLite3, conn, migrations)
	if err != nil {
		return fmt.Errorf("database: migration provider: %w", err)
	}
	if _, err := provider.Up(ctx); err != nil {
		return fmt.Errorf("database: migrate: %w", err)
	}
	return nil
}
