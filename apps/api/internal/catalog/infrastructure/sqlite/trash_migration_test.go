package sqlite_test

import (
	"database/sql"
	"io/fs"
	"testing"

	"github.com/pressly/goose/v3"
	_ "modernc.org/sqlite" // pure-Go driver, registered as "sqlite"

	"github.com/CarlosSV923/VideoExplorer/apps/api/db"
	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/catalog/domain"
	catalogsqlite "github.com/CarlosSV923/VideoExplorer/apps/api/internal/catalog/infrastructure/sqlite"
)

// trashMigration is the migration that rebuilds the trash tables (RF-07, RF-30).
const trashMigration = 8

// migrateTo migrates conn to version with goose, or to the latest version when
// version is 0.
func migrateTo(t *testing.T, conn *sql.DB, version int64) *goose.Provider {
	t.Helper()

	migrations, err := fs.Sub(db.Migrations, "migrations")
	if err != nil {
		t.Fatal(err)
	}
	provider, err := goose.NewProvider(goose.DialectSQLite3, conn, migrations)
	if err != nil {
		t.Fatalf("NewProvider: %v", err)
	}
	if version == 0 {
		if _, err := provider.Up(t.Context()); err != nil {
			t.Fatalf("Up: %v", err)
		}
		return provider
	}
	if _, err := provider.UpTo(t.Context(), version); err != nil {
		t.Fatalf("UpTo %d: %v", version, err)
	}
	return provider
}

func TestTrashMigrationKeepsTheEntriesAndTheirIDs(t *testing.T) {
	t.Parallel()

	conn, err := sql.Open("sqlite", "file:"+t.TempDir()+"/trash.db?_pragma=foreign_keys(1)")
	if err != nil {
		t.Fatal(err)
	}
	conn.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = conn.Close() })

	provider := migrateTo(t, conn, trashMigration-1)
	if _, err := conn.ExecContext(t.Context(), `
		INSERT INTO trash_entries (id, origin, reason, dir, trashed_at) VALUES (7, 'upload', 'extracted', 'aa', '2026-10-10T12:00:00.000000000Z');
		INSERT INTO trash_files (id, entry_id, name, size) VALUES (3, 7, 'Movie.zip', 25);`); err != nil {
		t.Fatalf("insert before the migration: %v", err)
	}

	if _, err := provider.Up(t.Context()); err != nil {
		t.Fatalf("Up: %v", err)
	}
	repo := catalogsqlite.NewTrashRepository(conn)
	got, err := repo.Entries(t.Context())
	if err != nil {
		t.Fatalf("Entries: %v", err)
	}
	if len(got) != 1 || got[0].ID != 7 || got[0].Dir != "aa" || got[0].BatchID != "" || got[0].RelativePath != "" {
		t.Fatalf("entries after the migration = %+v, want the old entry with no batch", got)
	}
	if len(got[0].Files) != 1 || got[0].Files[0] != (domain.TrashFile{Name: "Movie.zip", Size: 25}) {
		t.Fatalf("files after the migration = %+v", got[0].Files)
	}

	// The rebuilt tables still cascade and accept the new reason.
	saved, err := repo.Insert(t.Context(), domain.TrashEntry{
		Origin: domain.TrashOriginUpload, Reason: domain.TrashReasonFiltered, Dir: "bb",
		BatchID: "b1", RelativePath: "Movie/sample.mkv", Files: []domain.TrashFile{{Name: "sample.mkv", Size: 1}},
	})
	if err != nil {
		t.Fatalf("Insert after the migration: %v", err)
	}
	if saved.ID <= 7 {
		t.Errorf("new entry id = %d, want a new id after 7", saved.ID)
	}
	if _, err := conn.ExecContext(t.Context(), `DELETE FROM trash_entries WHERE id = 7`); err != nil {
		t.Fatalf("delete: %v", err)
	}
	var files int
	if err := conn.QueryRowContext(t.Context(), `SELECT COUNT(*) FROM trash_files`).Scan(&files); err != nil {
		t.Fatal(err)
	}
	if files != 1 {
		t.Errorf("files after deleting entry 7 = %d, want 1 (the cascade removes only its own)", files)
	}

	// Going back removes the filtered entries and keeps the others.
	if _, err := provider.DownTo(t.Context(), trashMigration-1); err != nil {
		t.Fatalf("DownTo: %v", err)
	}
	var reasons int
	if err := conn.QueryRowContext(t.Context(), `SELECT COUNT(*) FROM trash_entries WHERE reason = 'filtered'`).Scan(&reasons); err != nil {
		t.Fatal(err)
	}
	if reasons != 0 {
		t.Errorf("filtered entries after the down migration = %d, want 0", reasons)
	}
}
