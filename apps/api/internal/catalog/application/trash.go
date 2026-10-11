// Package application holds the catalog use cases and the ports they need.
package application

import (
	"context"
	"fmt"
	"time"

	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/catalog/domain"
)

// TrashRepository reads the metadata of the trash. Its entries are written by the commit of
// the change that trashes them (see Records).
type TrashRepository interface {
	// Entries returns every entry, oldest first.
	Entries(ctx context.Context) ([]domain.TrashEntry, error)
}

// Trash sends files to the trash with their metadata (RF-30). It is a change of the
// library: it goes through the engine, so a file is never in the trash without its entry.
// Listing, restoring and purging the trash come in the trash phase (phase 7).
type Trash struct {
	repo   TrashRepository
	files  Files
	engine *Engine
	now    func() time.Time
}

// NewTrash builds the trash over the library's files and the engine that changes them.
// now may be nil (time.Now).
func NewTrash(repo TrashRepository, files Files, engine *Engine, now func() time.Time) *Trash {
	if now == nil {
		now = time.Now
	}
	return &Trash{repo: repo, files: files, engine: engine, now: now}
}

// PutRequest is one file that goes to the trash.
type PutRequest struct {
	Origin domain.TrashOrigin
	Reason domain.TrashReason
	// BatchID and RelativePath are where the file came from (RF-30): see domain.Provenance.
	BatchID      string
	RelativePath string
	// Name is the file's name, kept in the trash.
	Name string
	// Source is the file's current path inside the library.
	Source string
	Size   int64
}

// Put moves the file at r.Source into a new folder of the trash and records the entry, as
// one change of the library (RF-30, RNF-04). If the change cannot be recorded the file goes
// back to where it was.
func (t *Trash) Put(ctx context.Context, r PutRequest) (domain.TrashEntry, error) {
	dir := randomName()
	entry, err := domain.NewTrashEntry(r.Origin, r.Reason, dir,
		domain.Provenance{BatchID: r.BatchID, RelativePath: r.RelativePath},
		domain.TrashFile{Name: r.Name, Size: r.Size}, t.now())
	if err != nil {
		return domain.TrashEntry{}, err
	}
	err = t.engine.Do(ctx, "trash.put", r.Source, func(b *Builder) error {
		if err := b.Mkdir(t.files.TrashPath(dir)); err != nil {
			return err
		}
		if err := b.Move(r.Source, t.files.TrashPath(dir, r.Name)); err != nil {
			return fmt.Errorf("move to trash: %w", err)
		}
		b.Record(func(rec Records) error {
			id, err := rec.InsertTrash(ctx, entry)
			if err != nil {
				return fmt.Errorf("record trash entry: %w", err)
			}
			entry.ID = id
			return nil
		})
		return nil
	})
	if err != nil {
		return domain.TrashEntry{}, err
	}
	return entry, nil
}

// Entries returns the trash entries, oldest first.
func (t *Trash) Entries(ctx context.Context) ([]domain.TrashEntry, error) {
	return t.repo.Entries(ctx)
}
