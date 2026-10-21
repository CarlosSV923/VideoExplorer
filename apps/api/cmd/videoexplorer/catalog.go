package main

import (
	"context"
	"database/sql"
	"log/slog"
	"time"

	catalogapplication "github.com/CarlosSV923/VideoExplorer/apps/api/internal/catalog/application"
	catalogdomain "github.com/CarlosSV923/VideoExplorer/apps/api/internal/catalog/domain"
	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/catalog/infrastructure/libraryfs"
	catalogsqlite "github.com/CarlosSV923/VideoExplorer/apps/api/internal/catalog/infrastructure/sqlite"
	ingestionapplication "github.com/CarlosSV923/VideoExplorer/apps/api/internal/ingestion/application"
	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/platform/config"
)

// catalogWiring is the catalog context as the composition root builds it.
type catalogWiring struct {
	Files *libraryfs.FS
	// Engine performs every change of the library, through its journal (RNF-04). Recover
	// undoes what a restart interrupted, and must run before anything else touches the library.
	Engine *catalogapplication.Engine
	// Trash sends files to the trash with their metadata (RF-06, RF-30).
	Trash *catalogapplication.Trash
	// Unassigned moves the content the ingestion keeps into No asignados (RF-08, RF-08a).
	Unassigned *catalogapplication.Unassigned
	// Section is No asignados as the disk holds it, and the changes of its entries (RF-27, RF-27a).
	Section *catalogapplication.Section
}

// newCatalog wires the catalog's engine, trash, unassigned section and section over the shared
// database. The trash and the operations live inside the library, so a move is a rename
// (RF-30, RNF-04).
func newCatalog(conn *sql.DB, cfg config.Config, log *slog.Logger) catalogWiring {
	files := libraryfs.New(cfg.LibraryPath)
	engine := catalogapplication.NewEngine(files, catalogsqlite.NewJournal(conn), time.Now, log)
	unassigned := catalogsqlite.NewUnassignedRepository(conn)
	return catalogWiring{
		Files:      files,
		Engine:     engine,
		Trash:      catalogapplication.NewTrash(catalogsqlite.NewTrashRepository(conn), files, engine, time.Now),
		Unassigned: catalogapplication.NewUnassigned(unassigned, files, engine, time.Now),
		Section: catalogapplication.NewSection(catalogapplication.SectionDeps{
			Unassigned: unassigned, Files: files, Downloads: files, Engine: engine,
			VideoExts: cfg.VideoExtensions, SubtitleExts: cfg.SubtitleExtensions, Now: time.Now, Log: log,
		}),
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
