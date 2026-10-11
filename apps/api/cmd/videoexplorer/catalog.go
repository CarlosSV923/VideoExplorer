package main

import (
	"context"
	"database/sql"
	"path/filepath"
	"time"

	catalogapplication "github.com/CarlosSV923/VideoExplorer/apps/api/internal/catalog/application"
	catalogdomain "github.com/CarlosSV923/VideoExplorer/apps/api/internal/catalog/domain"
	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/catalog/infrastructure/libraryfs"
	catalogsqlite "github.com/CarlosSV923/VideoExplorer/apps/api/internal/catalog/infrastructure/sqlite"
	ingestionapplication "github.com/CarlosSV923/VideoExplorer/apps/api/internal/ingestion/application"
	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/platform/config"
)

// trashDir is the library's trash: .videoexplorer/trash, on the library's file
// system so that a move into it is a rename (RF-30, RNF-04).
func trashDir(libraryPath string) string {
	return filepath.Join(libraryPath, ".videoexplorer", "trash")
}

// catalogWiring is the catalog context as the composition root builds it.
type catalogWiring struct {
	// Trash sends files to the trash with their metadata (RF-06, RF-30).
	Trash *catalogapplication.Trash
	// Unassigned moves the content the ingestion keeps into No asignados (RF-08, RF-08a).
	Unassigned *catalogapplication.Unassigned
}

// newCatalog wires the catalog's trash and unassigned section over the shared database.
func newCatalog(conn *sql.DB, cfg config.Config) catalogWiring {
	files := libraryfs.New(cfg.LibraryPath, trashDir(cfg.LibraryPath))
	return catalogWiring{
		Trash:      catalogapplication.NewTrash(catalogsqlite.NewTrashRepository(conn), files, time.Now),
		Unassigned: catalogapplication.NewUnassigned(catalogsqlite.NewUnassignedRepository(conn), files, time.Now),
	}
}

// trashForIngestion gives the ingestion context the trash through its port
// (RF-06): the catalog's types stay out of ingestion.
type trashForIngestion struct {
	trash *catalogapplication.Trash
}

var _ ingestionapplication.Trash = trashForIngestion{}

// Put implements ingestionapplication.Trash.
func (t trashForIngestion) Put(ctx context.Context, p ingestionapplication.TrashPut) error {
	_, err := t.trash.Put(ctx, catalogapplication.PutRequest{
		Origin:       catalogdomain.TrashOrigin(p.Origin),
		Reason:       catalogdomain.TrashReason(p.Reason),
		BatchID:      p.BatchID,
		RelativePath: p.RelativePath,
		Name:         p.Name,
		Source:       p.Source,
		Size:         p.Size,
	})
	return err
}

// unassignedForIngestion gives the ingestion context the unassigned section through its port
// (RF-08a): the catalog's types stay out of ingestion.
type unassignedForIngestion struct {
	unassigned *catalogapplication.Unassigned
}

var _ ingestionapplication.Library = unassignedForIngestion{}

// PutAside implements ingestionapplication.Library.
func (u unassignedForIngestion) PutAside(ctx context.Context, req ingestionapplication.AsideRequest) (map[string]string, error) {
	files := make([]catalogapplication.AsideFile, 0, len(req.Files))
	for _, f := range req.Files {
		files = append(files, catalogapplication.AsideFile{Source: f.Source, Rel: f.Rel, Size: f.Size})
	}
	return u.unassigned.Put(ctx, catalogapplication.AsideRequest{
		Source: req.Source, Name: req.Name, Loose: req.Loose,
		BatchID: req.BatchID, Relation: req.Relation, Files: files,
	})
}
