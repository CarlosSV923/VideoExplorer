// Package application holds the catalog use cases and the ports they need.
package application

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/catalog/domain"
)

// TrashRepository reads the metadata of the trash. Its entries are written by the commit of
// the change that trashes them (see Records).
type TrashRepository interface {
	// Entries returns every entry, newest first.
	Entries(ctx context.Context) ([]domain.TrashEntry, error)
}

// Trash sends files to the trash with their metadata (RF-30). It is a change of the
// library: it goes through the engine, so a file is never in the trash without its entry.
type Trash struct {
	repo    TrashRepository
	files   Files
	engine  *Engine
	now     func() time.Time
	restore *TrashRestore
}

// ErrTrashEntryNotFound says the requested trash entry is gone.
var ErrTrashEntryNotFound = errors.New("trash entry not found")

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

// Entries returns the trash entries, newest first.
func (t *Trash) Entries(ctx context.Context) ([]domain.TrashEntry, error) {
	return t.repo.Entries(ctx)
}

// ConfigureRestore wires restoration after the composition root has created the shared library adapters.
func (t *Trash) ConfigureRestore(d TrashRestoreDeps) { t.restore = NewTrashRestore(d) }

// PlanRestore previews an entry without changing disk or records (RF-31, RF-31a).
func (t *Trash) PlanRestore(ctx context.Context, id int64) (TrashRestorePlan, error) {
	if t.restore == nil {
		return TrashRestorePlan{}, errors.New("trash restore is not configured")
	}
	return t.restore.Plan(ctx, id)
}

// Restore commits the decisions of a trash restore through the engine (RF-31, RNF-04).
func (t *Trash) Restore(ctx context.Context, id int64, choices []TrashRestoreDecision) ([]string, error) {
	if t.restore == nil {
		return nil, errors.New("trash restore is not configured")
	}
	return t.restore.Commit(ctx, id, choices)
}

// Delete permanently removes one trash entry through the journal (RF-30, RF-31a).
func (t *Trash) Delete(ctx context.Context, id int64) error {
	return t.delete(ctx, []int64{id})
}

// Empty permanently removes every trash entry through one journaled operation (RF-30, RF-31a).
func (t *Trash) Empty(ctx context.Context) error {
	entries, err := t.repo.Entries(ctx)
	if err != nil {
		return err
	}
	ids := make([]int64, 0, len(entries))
	for _, e := range entries {
		ids = append(ids, e.ID)
	}
	return t.delete(ctx, ids)
}

// Purge removes entries older than retention through the journal (RF-30, RF-31a).
func (t *Trash) Purge(ctx context.Context, retention time.Duration) (int, error) {
	entries, err := t.repo.Entries(ctx)
	if err != nil {
		return 0, err
	}
	cutoff := t.now().Add(-retention)
	var ids []int64
	for _, e := range entries {
		if e.TrashedAt.Before(cutoff) {
			ids = append(ids, e.ID)
		}
	}
	return len(ids), t.delete(ctx, ids)
}

// RunPurge runs once in the background at startup and then every interval (RF-31a).
// The caller starts it in a goroutine so expiry work never delays serving requests.
func (t *Trash) RunPurge(ctx context.Context, interval, retention time.Duration, log *slog.Logger) {
	if interval <= 0 {
		interval = time.Hour
	}
	if log == nil {
		log = slog.New(slog.DiscardHandler)
	}
	for {
		count, err := t.Purge(ctx, retention)
		if err != nil {
			log.ErrorContext(ctx, "trash purge failed", "error", err)
		} else if count != 0 {
			log.InfoContext(ctx, "trash purged", "entries", count)
		}
		timer := time.NewTimer(interval)
		select {
		case <-ctx.Done():
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			return
		case <-timer.C:
		}
	}
}

func (t *Trash) delete(ctx context.Context, ids []int64) error {
	if len(ids) == 0 {
		return nil
	}
	entries, err := t.repo.Entries(ctx)
	if err != nil {
		return err
	}
	wanted := make(map[int64]bool, len(ids))
	for _, id := range ids {
		wanted[id] = true
	}
	var selected []domain.TrashEntry
	for _, e := range entries {
		if wanted[e.ID] {
			selected = append(selected, e)
			delete(wanted, e.ID)
		}
	}
	if len(wanted) != 0 {
		return ErrTrashEntryNotFound
	}
	return t.engine.Do(ctx, "trash.delete", "trash", func(b *Builder) error {
		if err := b.Mkdir(t.files.OpsPath(b.opID, "trash")); err != nil {
			return err
		}
		for _, e := range selected {
			if err := b.Move(t.files.TrashPath(e.Dir), t.files.OpsPath(b.opID, "trash", e.Dir)); err != nil {
				return err
			}
		}
		b.Record(func(rec Records) error {
			for _, e := range selected {
				if err := rec.DeleteTrashEntry(ctx, e.ID); err != nil {
					return err
				}
			}
			return nil
		})
		return nil
	})
}
