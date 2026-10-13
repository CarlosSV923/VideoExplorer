package application

import (
	"context"

	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/catalog/domain"
)

// LibraryRepository reads the registry of the library: its titles and their files (RF-21,
// RF-24). Its writes are the records of a change, written by the commit of that change
// (see Records), so an interrupted change leaves no record behind (RNF-04).
type LibraryRepository interface {
	// TitleByFolder returns the title whose folder is folder, in any letter case; false when none has it.
	TitleByFolder(ctx context.Context, folder string) (domain.LibraryTitle, bool, error)
	// Files returns every file of the title, trashed ones included (their TrashEntryID is set).
	Files(ctx context.Context, titleID int64) ([]domain.LibraryFile, error)
}
