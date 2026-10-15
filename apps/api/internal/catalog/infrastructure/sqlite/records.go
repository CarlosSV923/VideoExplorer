package sqlite

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/catalog/application"
	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/catalog/domain"
	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/catalog/infrastructure/sqlite/sqlcgen"
)

// records implements application.Records inside the transaction of a commit.
type records struct {
	q *sqlcgen.Queries
}

var _ application.Records = records{}

// InsertTrash implements application.Records.
func (r records) InsertTrash(ctx context.Context, entry domain.TrashEntry) (int64, error) {
	id, err := r.q.InsertTrashEntry(ctx, sqlcgen.InsertTrashEntryParams{
		Origin:       string(entry.Origin),
		Reason:       string(entry.Reason),
		Dir:          entry.Dir,
		TrashedAt:    formatTime(entry.TrashedAt),
		BatchID:      entry.BatchID,
		RelativePath: entry.RelativePath,
	})
	if err != nil {
		return 0, fmt.Errorf("catalog: insert trash entry: %w", err)
	}
	for _, f := range entry.Files {
		if err := r.q.InsertTrashFile(ctx, sqlcgen.InsertTrashFileParams{EntryID: id, Name: f.Name, Size: f.Size}); err != nil {
			return 0, fmt.Errorf("catalog: insert trash file: %w", err)
		}
	}
	return id, nil
}

// InsertUnassigned implements application.Records.
func (r records) InsertUnassigned(ctx context.Context, entry domain.UnassignedEntry) (int64, error) {
	id, err := r.q.InsertUnassignedEntry(ctx, sqlcgen.InsertUnassignedEntryParams{
		Source:    entry.Source,
		Name:      entry.Name,
		Loose:     boolInt(entry.Loose),
		BatchID:   entry.BatchID,
		Relation:  entry.Relation,
		Origin:    entry.Origin,
		CreatedAt: formatTime(entry.CreatedAt),
	})
	if err != nil {
		return 0, fmt.Errorf("catalog: insert unassigned entry: %w", err)
	}
	if err := insertFiles(ctx, r.q, id, entry.Files); err != nil {
		return 0, err
	}
	return id, nil
}

// AddUnassignedFiles implements application.Records.
func (r records) AddUnassignedFiles(ctx context.Context, entryID int64, files []domain.UnassignedFile) error {
	return insertFiles(ctx, r.q, entryID, files)
}

// insertFiles stores the files of an entry, as the queries of the transaction q.
func insertFiles(ctx context.Context, q *sqlcgen.Queries, entryID int64, files []domain.UnassignedFile) error {
	for _, f := range files {
		if err := q.InsertUnassignedFile(ctx, sqlcgen.InsertUnassignedFileParams{
			EntryID: entryID, Source: f.Source, Rel: f.Rel, Size: f.Size, MtimeMs: nullInt(f.Mtime),
		}); err != nil {
			return fmt.Errorf("catalog: insert unassigned file: %w", err)
		}
	}
	return nil
}

// UpdateUnassignedFile implements application.Records: the place, size and reading of a file of an entry.
func (r records) UpdateUnassignedFile(ctx context.Context, entryID int64, f domain.UnassignedFile) error {
	if err := r.q.UpdateUnassignedFile(ctx, sqlcgen.UpdateUnassignedFileParams{
		Rel: f.Rel, Size: f.Size, MtimeMs: nullInt(f.Mtime), EntryID: entryID, Source: f.Source,
	}); err != nil {
		return fmt.Errorf("catalog: update unassigned file: %w", err)
	}
	return nil
}

// boolInt stores a flag as the 0 or 1 the schema checks.
func boolInt(b bool) int64 {
	if b {
		return 1
	}
	return 0
}

// InsertLibraryTitle implements application.Records.
func (r records) InsertLibraryTitle(ctx context.Context, t domain.LibraryTitle) (int64, error) {
	id, err := r.q.InsertLibraryTitle(ctx, sqlcgen.InsertLibraryTitleParams{
		Kind:      string(t.Kind),
		Title:     t.Title,
		DiscTitle: t.DiscTitle,
		Year:      int64(t.Year),
		TmdbID:    nullInt(t.TMDBID),
		IDTag:     boolInt(t.IDTag),
		Folder:    t.Folder,
		CreatedAt: formatTime(t.CreatedAt),
		UpdatedAt: formatTime(t.UpdatedAt),
	})
	if err != nil {
		return 0, fmt.Errorf("catalog: insert library title: %w", err)
	}
	return id, nil
}

// InsertLibraryFiles implements application.Records. A subtitle is stored with the id of its
// video, which must already be in the registry.
func (r records) InsertLibraryFiles(ctx context.Context, titleID int64, files []domain.LibraryFile) error {
	for _, f := range files {
		var videoID sql.NullInt64
		if f.Kind == domain.FileSubtitle {
			id, err := r.q.LibraryFileIDByRel(ctx, f.VideoRel)
			if err != nil {
				return fmt.Errorf("catalog: the video of %s is not in the library: %w", f.Rel, err)
			}
			videoID = sql.NullInt64{Int64: id, Valid: true}
		}
		if _, err := r.q.InsertLibraryFile(ctx, sqlcgen.InsertLibraryFileParams{
			TitleID:      titleID,
			Rel:          f.Rel,
			Kind:         string(f.Kind),
			Season:       episodeInt(f.Place, f.Place.Season),
			FirstChapter: episodeInt(f.Place, f.Place.First),
			LastChapter:  episodeInt(f.Place, f.Place.Last),
			Part:         int64(f.Place.Part),
			Version:      f.Version,
			Language:     f.Language,
			Forced:       boolInt(f.Forced),
			Sdh:          boolInt(f.SDH),
			VideoID:      videoID,
			Size:         f.Size,
			PartialHash:  f.PartialHash,
			CreatedAt:    formatTime(f.CreatedAt),
		}); err != nil {
			return fmt.Errorf("catalog: insert library file: %w", err)
		}
	}
	return nil
}

// MoveLibraryFile implements application.Records.
func (r records) MoveLibraryFile(ctx context.Context, id int64, rel, version string) error {
	if err := r.q.MoveLibraryFile(ctx, sqlcgen.MoveLibraryFileParams{Rel: rel, Version: version, ID: id}); err != nil {
		return fmt.Errorf("catalog: move library file: %w", err)
	}
	return nil
}

// RemoveUnassignedFiles implements application.Records: the files that left the entry.
func (r records) RemoveUnassignedFiles(ctx context.Context, entryID int64, sources []string) error {
	for _, s := range sources {
		if err := r.q.DeleteUnassignedFile(ctx, sqlcgen.DeleteUnassignedFileParams{EntryID: entryID, Source: s}); err != nil {
			return fmt.Errorf("catalog: remove unassigned file: %w", err)
		}
	}
	return nil
}

// DeleteUnassignedEntry implements application.Records.
func (r records) DeleteUnassignedEntry(ctx context.Context, entryID int64) error {
	if err := r.q.DeleteUnassignedEntry(ctx, entryID); err != nil {
		return fmt.Errorf("catalog: delete unassigned entry: %w", err)
	}
	return nil
}

// ReleaseAssignLock implements application.Records.
func (r records) ReleaseAssignLock(ctx context.Context, entryID int64) error {
	if err := r.q.ClearUnassignedLock(ctx, entryID); err != nil {
		return fmt.Errorf("catalog: clear assign lock: %w", err)
	}
	return nil
}

// TrashLibraryFiles implements application.Records.
func (r records) TrashLibraryFiles(ctx context.Context, ids []int64, entryID int64) error {
	for _, id := range ids {
		if err := r.q.TrashLibraryFile(ctx, sqlcgen.TrashLibraryFileParams{TrashEntryID: sql.NullInt64{Int64: entryID, Valid: true}, ID: id}); err != nil {
			return fmt.Errorf("catalog: trash library file: %w", err)
		}
	}
	return nil
}

// DeleteLibraryFile implements application.Records.
func (r records) DeleteLibraryFile(ctx context.Context, id int64) error {
	if err := r.q.DeleteLibraryFile(ctx, id); err != nil {
		return fmt.Errorf("catalog: delete library file: %w", err)
	}
	return nil
}

// DeleteLibraryTitle implements application.Records.
func (r records) DeleteLibraryTitle(ctx context.Context, id int64) error {
	if err := r.q.DeleteLibraryTitle(ctx, id); err != nil {
		return fmt.Errorf("catalog: delete library title: %w", err)
	}
	return nil
}

// ReplaceLibrarySeen implements application.Records: the waiting files of the last scan are replaced.
func (r records) ReplaceLibrarySeen(ctx context.Context, seen []domain.SeenFile) error {
	if err := r.q.DeleteLibrarySeen(ctx); err != nil {
		return fmt.Errorf("catalog: clear waiting files: %w", err)
	}
	for _, f := range seen {
		if err := r.q.InsertLibrarySeen(ctx, sqlcgen.InsertLibrarySeenParams{
			Rel: f.Rel, Size: f.Stamp.Size, MtimeMs: f.Stamp.ModTime.UnixMilli(),
		}); err != nil {
			return fmt.Errorf("catalog: store waiting file: %w", err)
		}
	}
	return nil
}

// nullInt stores a number that is absent when it is 0.
func nullInt(n int64) sql.NullInt64 {
	return sql.NullInt64{Int64: n, Valid: n != 0}
}

// episodeInt stores a number of a series file (its season or a chapter); NULL for a movie or a subtitle.
func episodeInt(p domain.Place, n int) sql.NullInt64 {
	return sql.NullInt64{Int64: int64(n), Valid: p.Episode}
}
