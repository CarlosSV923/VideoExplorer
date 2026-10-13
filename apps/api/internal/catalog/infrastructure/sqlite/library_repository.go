package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/catalog/application"
	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/catalog/domain"
	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/catalog/infrastructure/sqlite/sqlcgen"
)

// LibraryRepository implements application.LibraryRepository. The registry is written by the
// commit of the change that alters it (see records).
type LibraryRepository struct {
	q *sqlcgen.Queries
}

var _ application.LibraryRepository = (*LibraryRepository)(nil)

// NewLibraryRepository builds the repository over an open database.
func NewLibraryRepository(db *sql.DB) *LibraryRepository {
	return &LibraryRepository{q: sqlcgen.New(db)}
}

// TitleByFolder implements application.LibraryRepository.
func (r *LibraryRepository) TitleByFolder(ctx context.Context, folder string) (domain.LibraryTitle, bool, error) {
	row, err := r.q.GetLibraryTitleByFolder(ctx, folder)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.LibraryTitle{}, false, nil
	}
	if err != nil {
		return domain.LibraryTitle{}, false, fmt.Errorf("catalog: read library title: %w", err)
	}
	created, err := time.Parse(timeLayout, row.CreatedAt)
	if err != nil {
		return domain.LibraryTitle{}, false, fmt.Errorf("catalog: library title time: %w", err)
	}
	updated, err := time.Parse(timeLayout, row.UpdatedAt)
	if err != nil {
		return domain.LibraryTitle{}, false, fmt.Errorf("catalog: library title time: %w", err)
	}
	return domain.LibraryTitle{
		ID: row.ID, Kind: domain.TitleKind(row.Kind), Title: row.Title, DiscTitle: row.DiscTitle,
		Year: int(row.Year), TMDBID: row.TmdbID.Int64, IDTag: row.IDTag != 0, Folder: row.Folder,
		CreatedAt: created, UpdatedAt: updated,
	}, true, nil
}

// Files implements application.LibraryRepository: every file of the title, trashed ones included.
func (r *LibraryRepository) Files(ctx context.Context, titleID int64) ([]domain.LibraryFile, error) {
	rows, err := r.q.ListLibraryFiles(ctx, titleID)
	if err != nil {
		return nil, fmt.Errorf("catalog: list library files: %w", err)
	}
	files := make([]domain.LibraryFile, 0, len(rows))
	for _, row := range rows {
		created, err := time.Parse(timeLayout, row.CreatedAt)
		if err != nil {
			return nil, fmt.Errorf("catalog: library file time: %w", err)
		}
		files = append(files, domain.LibraryFile{
			ID: row.ID, TitleID: row.TitleID, Rel: row.Rel, Kind: domain.FileKind(row.Kind),
			Place: domain.Place{
				Episode: row.Season.Valid,
				Season:  int(row.Season.Int64),
				First:   int(row.FirstChapter.Int64),
				Last:    int(row.LastChapter.Int64),
				Part:    int(row.Part),
			},
			Version: row.Version, Language: row.Language, Forced: row.Forced != 0, SDH: row.Sdh != 0,
			VideoRel: row.VideoRel.String, Size: row.Size, PartialHash: row.PartialHash,
			TrashEntryID: row.TrashEntryID.Int64, CreatedAt: created,
		})
	}
	return files, nil
}
