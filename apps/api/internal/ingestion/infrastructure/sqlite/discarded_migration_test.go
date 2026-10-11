package sqlite_test

import (
	"database/sql"
	"io/fs"
	"testing"

	"github.com/pressly/goose/v3"
	_ "modernc.org/sqlite" // pure-Go driver, registered as "sqlite"

	"github.com/CarlosSV923/VideoExplorer/apps/api/db"
)

// discardedMigration is the migration that adds the discarded state (RF-07, RF-10a).
const discardedMigration = 10

// goTo migrates conn with goose to version (the latest when version is 0).
func goTo(t *testing.T, conn *sql.DB, version int64) *goose.Provider {
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

func TestDiscardedMigrationKeepsJobsAndTheirFilterRecords(t *testing.T) {
	t.Parallel()

	conn, err := sql.Open("sqlite", "file:"+t.TempDir()+"/jobs.db?_pragma=foreign_keys(1)")
	if err != nil {
		t.Fatal(err)
	}
	conn.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = conn.Close() })

	provider := goTo(t, conn, discardedMigration-1)
	ctx := t.Context()
	if _, err := conn.ExecContext(ctx, `
		INSERT INTO upload_batches (id, name, relation, created_at) VALUES ('b-1', '', 'none', '2026-10-10T12:00:00.000000000Z');
		INSERT INTO upload_jobs (id, batch_id, file_name, relative_path, size, status, error, created_at, updated_at, received, progress, parts_total, volume_set, volume_index, volume_group, filter_done)
			VALUES ('j-1', 'b-1', 'Movie.mkv', 'Folder/Movie.mkv', 10, 'filtering', '', '2026-10-10T12:00:00.000000000Z', '2026-10-10T12:00:01.000000000Z', 10, 0, 0, '', 0, '', 1),
			       ('j-2', 'b-1', 'notes.txt', '', 5, 'failed', 'no_space', '2026-10-10T12:00:00.000000000Z', '2026-10-10T12:00:01.000000000Z', 5, 0, 0, '', 0, '', 0);
		INSERT INTO filter_files (job_id, path, size, kind, trashed) VALUES ('j-1', 'Folder/Movie.mkv', 10, 'video', 0), ('j-1', 'Folder/x.nfo', 3, 'discard', 1);`); err != nil {
		t.Fatalf("insert before the migration: %v", err)
	}

	goTo(t, conn, 0)
	var status string
	var filterDone, received, files int
	if err := conn.QueryRowContext(ctx, `SELECT status, filter_done, received FROM upload_jobs WHERE id = 'j-1'`).Scan(&status, &filterDone, &received); err != nil {
		t.Fatal(err)
	}
	if status != "filtering" || filterDone != 1 || received != 10 {
		t.Fatalf("job after the migration = %s, filter done %d, received %d; want filtering, done, 10", status, filterDone, received)
	}
	if err := conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM filter_files WHERE job_id = 'j-1'`).Scan(&files); err != nil {
		t.Fatal(err)
	}
	if files != 2 {
		t.Fatalf("filter records after the migration = %d, want 2", files)
	}
	if _, err := conn.ExecContext(ctx, `UPDATE upload_jobs SET status = 'discarded' WHERE id = 'j-1'`); err != nil {
		t.Fatalf("discarded is refused after the migration: %v", err)
	}
	if _, err := conn.ExecContext(ctx, `UPDATE upload_jobs SET status = 'unknown' WHERE id = 'j-1'`); err == nil {
		t.Error("an unknown status was accepted")
	}
	if _, err := conn.ExecContext(ctx, `INSERT INTO filter_files (job_id, path, size, kind, trashed) VALUES ('no-such-job', 'a.mkv', 1, 'video', 0)`); err == nil {
		t.Error("filter records of an unknown job were accepted: the foreign key was lost")
	}

	// Going back keeps the rows; a discarded job becomes a failed one with no_video.
	if _, err := provider.DownTo(ctx, discardedMigration-1); err != nil {
		t.Fatalf("DownTo: %v", err)
	}
	if err := conn.QueryRowContext(ctx, `SELECT status, error FROM upload_jobs WHERE id = 'j-1'`).Scan(&status, new(string)); err != nil {
		t.Fatal(err)
	}
	if status != "failed" {
		t.Errorf("status after the down migration = %s, want failed", status)
	}
	if err := conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM filter_files`).Scan(&files); err != nil {
		t.Fatal(err)
	}
	if files != 2 {
		t.Errorf("filter records after the down migration = %d, want 2", files)
	}
}
