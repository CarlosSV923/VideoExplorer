package sqlite_test

import (
	"database/sql"
	"testing"

	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/catalog/application"
	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/catalog/domain"
	catalogsqlite "github.com/CarlosSV923/VideoExplorer/apps/api/internal/catalog/infrastructure/sqlite"
)

// replacedMigration is the migration that adds the library origin and the replaced reason (RF-24, RF-30).
const replacedMigration = 17

func TestReplacedTrashMigrationKeepsTheEntriesAndTheLibraryLinks(t *testing.T) {
	t.Parallel()

	conn, err := sql.Open("sqlite", "file:"+t.TempDir()+"/replaced.db?_pragma=foreign_keys(1)")
	if err != nil {
		t.Fatal(err)
	}
	conn.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = conn.Close() })

	provider := migrateTo(t, conn, replacedMigration-1)
	if _, err := conn.ExecContext(t.Context(), `
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
	got, err := catalogsqlite.NewTrashRepository(conn).Entries(t.Context())
	if err != nil {
		t.Fatalf("Entries: %v", err)
	}
	if len(got) != 1 || got[0].ID != 7 || got[0].BatchID != "b1" || got[0].RelativePath != "Movie/sample.mkv" ||
		len(got[0].Files) != 1 || got[0].Files[0] != (domain.TrashFile{Name: "sample.mkv", Size: 25}) {
		t.Fatalf("entries after the migration = %+v, want the old entry with its batch and its file", got)
	}
	var linked int64
	if err := conn.QueryRowContext(t.Context(), `SELECT trash_entry_id FROM library_files WHERE id = 5`).Scan(&linked); err != nil || linked != 7 {
		t.Fatalf("library link after the migration = %d, %v; want entry 7 kept", linked, err)
	}

	// The new origin and reason are accepted: a replaced file goes to the trash with its subtitle.
	replaced, err := domain.NewTrashEntryOf(domain.TrashOriginLibrary, domain.TrashReasonReplaced, "e2",
		domain.Provenance{RelativePath: "Movies/Dune (2021)/Dune (2021).mkv"},
		[]domain.TrashFile{{Name: "Dune (2021).mkv", Size: 9}, {Name: "Dune (2021).en.srt", Size: 1}}, libraryNow)
	if err != nil {
		t.Fatal(err)
	}
	var replacedID int64
	err = commitLibrary(t.Context(), conn, func(r application.Records) error {
		var err error
		replacedID, err = r.InsertTrash(t.Context(), replaced)
		if err != nil {
			return err
		}
		return r.TrashLibraryFiles(t.Context(), []int64{5}, replacedID)
	})
	if err != nil {
		t.Fatalf("replaced entry after the migration: %v", err)
	}

	// Going back removes the replaced entries and their links, and keeps the others.
	if _, err := provider.DownTo(t.Context(), replacedMigration-1); err != nil {
		t.Fatalf("DownTo: %v", err)
	}
	var replacedLeft int
	if err := conn.QueryRowContext(t.Context(), `SELECT COUNT(*) FROM trash_entries WHERE id = ?`, replacedID).Scan(&replacedLeft); err != nil {
		t.Fatal(err)
	}
	if replacedLeft != 0 {
		t.Errorf("replaced entry after the down migration is still there")
	}
	var kept int
	if err := conn.QueryRowContext(t.Context(), `SELECT COUNT(*) FROM trash_entries WHERE id = 7`).Scan(&kept); err != nil {
		t.Fatal(err)
	}
	if kept != 1 {
		t.Errorf("the upload entry after the down migration = %d, want 1", kept)
	}
	// The file that was replaced went with its entry, so it is no longer linked to the trash; the
	// upload entry is still there, and its files are still in it.
	var link sql.NullInt64
	if err := conn.QueryRowContext(t.Context(), `SELECT trash_entry_id FROM library_files WHERE id = 5`).Scan(&link); err != nil || link.Valid {
		t.Errorf("library link of the replaced file after the down migration = %+v, %v; want none", link, err)
	}
	var uploadFiles int
	if err := conn.QueryRowContext(t.Context(), `SELECT COUNT(*) FROM trash_files WHERE entry_id = 7`).Scan(&uploadFiles); err != nil || uploadFiles != 1 {
		t.Errorf("files of the upload entry after the down migration = %d, %v; want 1", uploadFiles, err)
	}
}
