package application

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"time"

	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/catalog/domain"
)

// ErrNoFiles is returned when an entry is asked for without any file.
var ErrNoFiles = errors.New("an unassigned entry needs at least one file")

// UnassignedRepository reads the entries of No asignados. Their entries are written by the
// commit of the change that moves them in (see Records).
type UnassignedRepository interface {
	// BySource returns the entry recorded for source, with its files, and whether there is one.
	BySource(ctx context.Context, source string) (domain.UnassignedEntry, bool, error)
	// ByID returns the entry with this id, with its files and its lock, and whether there is one.
	ByID(ctx context.Context, id int64) (domain.UnassignedEntry, bool, error)
	// Lock takes the assignment lock of entry id for token until until, when the entry is free,
	// its lock has expired at now, or token holds it (a renewal). It reports whether it did.
	Lock(ctx context.Context, id int64, token string, now, until time.Time) (bool, error)
	// Unlock frees the lock of entry id when token holds it; another token changes nothing.
	Unlock(ctx context.Context, id int64, token string) error
	// Names returns the names of every recorded entry.
	Names(ctx context.Context) ([]string, error)
}

// Unassigned moves what the ingestion keeps into No asignados (spec RF-08, RF-08a). It is
// the only one that creates entries of the section, so the names are decided here, while
// the engine holds the library.
type Unassigned struct {
	repo   UnassignedRepository
	files  Files
	engine *Engine
	now    func() time.Time
}

// NewUnassigned builds the section's moves over the repository, the library's files and the
// engine that changes them. now may be nil (time.Now).
func NewUnassigned(repo UnassignedRepository, files Files, engine *Engine, now func() time.Time) *Unassigned {
	if now == nil {
		now = time.Now
	}
	return &Unassigned{repo: repo, files: files, engine: engine, now: now}
}

// AsideRequest is the content a batch keeps, to move into No asignados as one entry.
type AsideRequest struct {
	// Source is what the move is for (the batch, or the unit of a batch without relation):
	// a second request with the same source adds to the same entry (RF-08a).
	Source string
	// Name is the name the entry is given, before sanitizing: a folder's name, or a loose
	// file's name with its extension (RF-08).
	Name string
	// Loose keeps a single file as it is, without a folder (RF-08); it takes one file only.
	Loose    bool
	BatchID  string
	Relation string
	Files    []AsideFile
}

// AsideFile is one file of a AsideRequest.
type AsideFile struct {
	// Source is the file's staging path, and Rel its place in the entry, with "/" separators
	// (ignored for a loose entry, whose file is named after the entry).
	Source string
	Rel    string
	Size   int64
}

// Put moves the files of r into their entry in _unassigned/ (RF-08, RF-08a), as one change
// of the library: the entry and its files are recorded in the commit that follows their
// moves. A failed call leaves nothing behind, and a repeated one goes on from what the
// entry holds: a file already in the entry is not moved again. Put never overwrites a file.
// It returns the place of every file of r, by source.
func (u *Unassigned) Put(ctx context.Context, r AsideRequest) (map[string]string, error) {
	if len(r.Files) == 0 {
		return nil, ErrNoFiles
	}
	places := map[string]string{}
	err := u.engine.Do(ctx, "unassigned.put", r.Source, func(b *Builder) error {
		clear(places)
		entry, found, err := u.repo.BySource(ctx, r.Source)
		if err != nil {
			return err
		}
		if found {
			return u.addMissing(ctx, b, entry, r, places)
		}
		return u.record(ctx, b, r, places)
	})
	if err != nil {
		return nil, err
	}
	return places, nil
}

// record plans a new entry: its name, the place of each file, and the moves that bring them
// there. Its record is written when they are all done.
func (u *Unassigned) record(ctx context.Context, b *Builder, r AsideRequest, places map[string]string) error {
	names, err := u.takenNames(ctx)
	if err != nil {
		return err
	}
	name := domain.FreeName(domain.FitName(domain.EntryNameFor(r.Name, r.Loose), r.Loose), r.Loose, names)
	entry, err := domain.NewUnassignedEntry(r.Source, name, r.Loose, r.BatchID, r.Relation, u.now())
	if err != nil {
		return err
	}
	if r.Loose && len(r.Files) != 1 {
		return fmt.Errorf("catalog: a loose entry holds one file, got %d", len(r.Files))
	}
	taken := domain.NameSet{}
	for _, f := range r.Files {
		file, err := u.planFile(f, taken, r.Loose)
		if err != nil {
			return err
		}
		if err := u.move(b, f.Source, entry, file, places); err != nil {
			return err
		}
		entry.Files = append(entry.Files, file)
	}
	b.Record(func(rec Records) error {
		id, err := rec.InsertUnassigned(ctx, entry)
		if err != nil {
			return fmt.Errorf("catalog: record unassigned entry: %w", err)
		}
		entry.ID = id
		return nil
	})
	return nil
}

// addMissing plans the files of r that the entry does not have yet, each with a place free
// in the entry. A loose entry has no room for another file.
func (u *Unassigned) addMissing(ctx context.Context, b *Builder, entry domain.UnassignedEntry, r AsideRequest, places map[string]string) error {
	known := map[string]domain.UnassignedFile{}
	taken := domain.NameSet{}
	for _, f := range entry.Files {
		known[f.Source] = f
		taken.Add(f.Rel)
	}
	var added []domain.UnassignedFile
	for _, f := range r.Files {
		if k, ok := known[f.Source]; ok {
			places[f.Source] = u.placeOf(entry, k)
			continue
		}
		if entry.Loose {
			return fmt.Errorf("catalog: entry %q is a loose file and takes no other file", entry.Name)
		}
		file, err := u.planFile(f, taken, false)
		if err != nil {
			return err
		}
		if err := u.move(b, f.Source, entry, file, places); err != nil {
			return err
		}
		added = append(added, file)
		known[f.Source] = file
		entry.Files = append(entry.Files, file)
	}
	if len(added) == 0 {
		return nil
	}
	entryID := entry.ID
	b.Record(func(rec Records) error {
		return rec.AddUnassignedFiles(ctx, entryID, added)
	})
	return nil
}

// planFile checks a file of r and returns its entry of No asignados. Its rel is free among
// taken, which it joins.
func (u *Unassigned) planFile(f AsideFile, taken domain.NameSet, loose bool) (domain.UnassignedFile, error) {
	staged, err := u.files.Exists(f.Source)
	if err != nil {
		return domain.UnassignedFile{}, fmt.Errorf("catalog: look at %s: %w", f.Source, err)
	}
	if !staged {
		return domain.UnassignedFile{}, fmt.Errorf("catalog: %s is not in staging", f.Source)
	}
	rel := ""
	if !loose {
		rel = domain.FreeRel(domain.SanitizePath(f.Rel), taken)
		taken.Add(rel)
	}
	return domain.NewUnassignedFile(f.Source, rel, f.Size, loose)
}

// move plans the rename of a staged file to its place in the entry, and the folders it needs.
func (u *Unassigned) move(b *Builder, src string, entry domain.UnassignedEntry, file domain.UnassignedFile, places map[string]string) error {
	dst := u.placeOf(entry, file)
	if err := b.Mkdir(filepath.Dir(dst)); err != nil {
		return fmt.Errorf("catalog: create %s: %w", filepath.Dir(dst), err)
	}
	if err := b.Move(src, dst); err != nil {
		return fmt.Errorf("catalog: move to %s: %w", domain.UnassignedDir, err)
	}
	places[src] = dst
	return nil
}

// takenNames returns the names the section already has: the recorded entries and the
// folders and files of _unassigned/ itself, in any letter case.
func (u *Unassigned) takenNames(ctx context.Context) (domain.NameSet, error) {
	recorded, err := u.repo.Names(ctx)
	if err != nil {
		return nil, err
	}
	onDisk, err := u.files.Names(u.files.UnassignedPath())
	if err != nil {
		return nil, fmt.Errorf("catalog: list %s: %w", domain.UnassignedDir, err)
	}
	names := domain.NewNameSet(recorded...)
	for _, n := range onDisk {
		names.Add(n)
	}
	return names, nil
}

// placeOf is the path a file of the entry has in _unassigned/.
func (u *Unassigned) placeOf(entry domain.UnassignedEntry, f domain.UnassignedFile) string {
	if entry.Loose {
		return u.files.UnassignedPath(entry.Name)
	}
	return u.files.UnassignedPath(entry.Name, filepath.FromSlash(f.Rel))
}
