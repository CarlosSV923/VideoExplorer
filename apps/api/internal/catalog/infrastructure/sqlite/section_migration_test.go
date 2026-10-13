package sqlite_test

import (
	"database/sql"
	"testing"

	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/catalog/application"
	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/catalog/domain"
	catalogsqlite "github.com/CarlosSV923/VideoExplorer/apps/api/internal/catalog/infrastructure/sqlite"
)

// sectionMigration is the migration that adds the samba origin, the removed reason and the readings
// of the files of No asignados (RF-27).
const sectionMigration = 19

func TestSectionMigrationKeepsTheEntriesTheFilesAndTheTrash(t *testing.T) {
	t.Parallel()

	conn, err := sql.Open("sqlite", "file:"+t.TempDir()+"/section.db?_pragma=foreign_keys(1)")
	if err != nil {
		t.Fatal(err)
	}
	conn.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = conn.Close() })

	provider := migrateTo(t, conn, sectionMigration-1)
	if _, err := conn.ExecContext(t.Context(), `
		INSERT INTO unassigned_entries (id, source, name, loose, batch_id, relation, origin, created_at)
			VALUES (4, 'batch-1', 'Movie', 0, 'b1', 'parts', 'upload', '2026-10-10T12:00:00.000000000Z');
		INSERT INTO unassigned_files (entry_id, source, rel, size)
			VALUES (4, '/staging/a.mkv', 'Movie/a.mkv', 5);
		INSERT INTO unassigned_entries (id, source, name, loose, batch_id, relation, origin, created_at)
			VALUES (9, 'batch-9', 'Gone', 1, 'b9', 'none', 'upload', '2026-10-10T12:00:00.000000000Z');
		DELETE FROM unassigned_entries WHERE id = 9;
		INSERT INTO trash_entries (id, origin, reason, dir, trashed_at, batch_id, relative_path)
			VALUES (7, 'upload', 'filtered', 'aa', '2026-10-10T12:00:00.000000000Z', 'b1', 'Movie/sample.mkv');
		INSERT INTO trash_files (id, entry_id, name, size) VALUES (3, 7, 'sample.mkv', 25);
		INSERT INTO library_titles (id, kind, title, disc_title, year, tmdb_id, id_tag, folder, created_at, updated_at)
			VALUES (1, 'movie', 'Dune', 'Dune', 2021, NULL, 0, 'Movies/Dune (2021)', '2026-10-10T12:00:00.000000000Z', '2026-10-10T12:00:00.000000000Z');
		INSERT INTO library_files (id, title_id, rel, kind, part, version, language, forced, sdh, size, partial_hash, trash_entry_id, created_at)
			VALUES (5, 1, 'Movies/Dune (2021)/Dune (2021).mkv', 'video', 0, '', '', 0, 0, 9, '', 7, '2026-10-10T12:00:00.000000000Z');`); err != nil {
		t.Fatalf("insert before the migration: %v", err)
	}

	if _, err := provider.Up(t.Context()); err != nil {
		t.Fatalf("Up: %v", err)
	}

	var origin string
	var mtime sql.NullInt64
	if err := conn.QueryRowContext(t.Context(), `SELECT origin FROM unassigned_entries WHERE id = 4`).Scan(&origin); err != nil || origin != "upload" {
		t.Fatalf("entry 4 after the migration: origin %q, %v; want upload", origin, err)
	}
	if err := conn.QueryRowContext(t.Context(), `SELECT mtime_ms FROM unassigned_files WHERE entry_id = 4`).Scan(&mtime); err != nil || mtime.Valid {
		t.Fatalf("file reading after the migration = %+v, %v; want none yet", mtime, err)
	}
	var linked int64
	if err := conn.QueryRowContext(t.Context(), `SELECT trash_entry_id FROM library_files WHERE id = 5`).Scan(&linked); err != nil || linked != 7 {
		t.Fatalf("library link after the migration = %d, %v; want entry 7 kept", linked, err)
	}
	got, err := catalogsqlite.NewTrashRepository(conn).Entries(t.Context())
	if err != nil || len(got) != 1 || got[0].ID != 7 || len(got[0].Files) != 1 {
		t.Fatalf("trash after the migration = %+v, %v; want the old entry", got, err)
	}

	// The ids keep counting: the id of the entry deleted before the migration is not reused.
	var nextID int64
	if _, err := conn.ExecContext(t.Context(), `
		INSERT INTO unassigned_entries (source, name, loose, batch_id, relation, origin, created_at)
			VALUES ('batch-10', 'Next', 0, 'b10', 'none', 'upload', '2026-10-10T12:00:00.000000000Z')`); err != nil {
		t.Fatal(err)
	}
	if err := conn.QueryRowContext(t.Context(), `SELECT id FROM unassigned_entries WHERE source = 'batch-10'`).Scan(&nextID); err != nil || nextID != 10 {
		t.Fatalf("next entry id = %d, %v; want 10 (9 was used)", nextID, err)
	}

	// The new origin and reason are accepted: a Samba file sent to the trash from the section.
	removed, err := domain.NewTrashEntry(domain.TrashOriginSamba, domain.TrashReasonRemoved, "e2",
		domain.Provenance{RelativePath: "Show/x.mkv"}, domain.TrashFile{Name: "x.mkv", Size: 3}, libraryNow)
	if err != nil {
		t.Fatal(err)
	}
	if err := commitLibrary(t.Context(), conn, func(r application.Records) error {
		_, err := r.InsertTrash(t.Context(), removed)
		return err
	}); err != nil {
		t.Fatalf("removed entry after the migration: %v", err)
	}
	// A samba entry with a file and a reading (RF-27).
	if _, err := conn.ExecContext(t.Context(), `
		INSERT INTO unassigned_entries (source, name, loose, batch_id, relation, origin, created_at)
			VALUES ('samba:Show', 'Show', 0, '', 'none', 'samba', '2026-10-10T12:00:00.000000000Z');
		INSERT INTO unassigned_files (entry_id, source, rel, size, mtime_ms)
			VALUES ((SELECT id FROM unassigned_entries WHERE source = 'samba:Show'), '_unassigned/Show/x.mkv', 'x.mkv', 3, 1700000000000);`); err != nil {
		t.Fatalf("samba entry after the migration: %v", err)
	}

	// Going back removes what only the new origin and reason allow, and keeps the rest.
	if _, err := provider.DownTo(t.Context(), sectionMigration-1); err != nil {
		t.Fatalf("DownTo: %v", err)
	}
	var samba, removedLeft int
	if err := conn.QueryRowContext(t.Context(), `SELECT COUNT(*) FROM unassigned_entries WHERE origin = 'samba'`).Scan(&samba); err != nil || samba != 0 {
		t.Errorf("samba entries after the down migration = %d, %v; want none", samba, err)
	}
	if err := conn.QueryRowContext(t.Context(), `SELECT COUNT(*) FROM trash_entries WHERE reason = 'removed'`).Scan(&removedLeft); err != nil || removedLeft != 0 {
		t.Errorf("removed entries after the down migration = %d, %v; want none", removedLeft, err)
	}
	var upload, files int
	if err := conn.QueryRowContext(t.Context(), `SELECT COUNT(*) FROM unassigned_entries WHERE id = 4`).Scan(&upload); err != nil || upload != 1 {
		t.Errorf("upload entry after the down migration = %d, %v; want 1", upload, err)
	}
	if err := conn.QueryRowContext(t.Context(), `SELECT COUNT(*) FROM unassigned_files WHERE entry_id = 4`).Scan(&files); err != nil || files != 1 {
		t.Errorf("files of the upload entry after the down migration = %d, %v; want 1", files, err)
	}
	var link int64
	if err := conn.QueryRowContext(t.Context(), `SELECT trash_entry_id FROM library_files WHERE id = 5`).Scan(&link); err != nil || link != 7 {
		t.Errorf("library link after the down migration = %d, %v; want entry 7 kept", link, err)
	}
}
