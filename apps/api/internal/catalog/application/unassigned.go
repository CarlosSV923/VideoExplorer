package application

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"sync"
	"time"

	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/catalog/domain"
)

// ErrNoFiles is returned when an entry is asked for without any file.
var ErrNoFiles = errors.New("an unassigned entry needs at least one file")

// UnassignedRepository persists the entries of No asignados and their files.
type UnassignedRepository interface {
	// BySource returns the entry recorded for source, with its files, and whether there is one.
	BySource(ctx context.Context, source string) (domain.UnassignedEntry, bool, error)
	// Names returns the names of every recorded entry.
	Names(ctx context.Context) ([]string, error)
	// Record stores a new entry with its files, in one transaction, and returns it with its id.
	Record(ctx context.Context, entry domain.UnassignedEntry) (domain.UnassignedEntry, error)
	// AddFiles stores the files that an existing entry did not have yet.
	AddFiles(ctx context.Context, entryID int64, files []domain.UnassignedFile) error
	// MarkMoved records that the file from source is in its place.
	MarkMoved(ctx context.Context, entryID int64, source string) error
}

// Unassigned moves what the ingestion keeps into No asignados (spec RF-08, RF-08a). It
// is the only one that creates entries of the section, so the names are decided here.
type Unassigned struct {
	repo  UnassignedRepository
	files Files
	now   func() time.Time
	// mu serializes the moves: the entries and their files are recorded one move at a time.
	mu sync.Mutex
}

// NewUnassigned builds the section's moves over the repository and the library's files.
// now may be nil (time.Now).
func NewUnassigned(repo UnassignedRepository, files Files, now func() time.Time) *Unassigned {
	if now == nil {
		now = time.Now
	}
	return &Unassigned{repo: repo, files: files, now: now}
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

// Put moves the files of r into their entry in _unassigned/ (RF-08, RF-08a). The entry is
// recorded first, with its name (sanitized, and suffixed when taken) and the place of each
// file; then every file not moved yet is renamed into its place. A file already in its
// place is recorded as moved, never moved twice, so a failed call and a restart go on
// where they stopped. Put never overwrites a file. It returns the place of every file of
// the entry, by source.
func (u *Unassigned) Put(ctx context.Context, r AsideRequest) (map[string]string, error) {
	u.mu.Lock()
	defer u.mu.Unlock()

	if len(r.Files) == 0 {
		return nil, ErrNoFiles
	}
	entry, found, err := u.repo.BySource(ctx, r.Source)
	if err != nil {
		return nil, err
	}
	if found {
		if err := u.addMissing(ctx, &entry, r); err != nil {
			return nil, err
		}
	} else if entry, err = u.record(ctx, r); err != nil {
		return nil, err
	}

	places := make(map[string]string, len(entry.Files))
	for i := range entry.Files {
		f := &entry.Files[i]
		dst := u.placeOf(entry, *f)
		if !f.Moved {
			if err := u.moveOne(ctx, entry.ID, f.Source, dst); err != nil {
				return nil, err
			}
			f.Moved = true
		}
		places[f.Source] = dst
	}
	return places, nil
}

// record chooses the entry's name and the place of each file, and stores the entry.
func (u *Unassigned) record(ctx context.Context, r AsideRequest) (domain.UnassignedEntry, error) {
	names, err := u.takenNames(ctx)
	if err != nil {
		return domain.UnassignedEntry{}, err
	}
	name := domain.FreeName(domain.FitName(domain.EntryNameFor(r.Name, r.Loose), r.Loose), r.Loose, names)
	entry, err := domain.NewUnassignedEntry(r.Source, name, r.Loose, r.BatchID, r.Relation, u.now())
	if err != nil {
		return domain.UnassignedEntry{}, err
	}
	if r.Loose && len(r.Files) != 1 {
		return domain.UnassignedEntry{}, fmt.Errorf("catalog: a loose entry holds one file, got %d", len(r.Files))
	}
	taken := domain.NameSet{}
	for _, f := range r.Files {
		// A new entry's files are all still in staging: nothing was moved before the record.
		staged, err := u.files.Exists(f.Source)
		if err != nil {
			return domain.UnassignedEntry{}, fmt.Errorf("catalog: look at %s: %w", f.Source, err)
		}
		if !staged {
			return domain.UnassignedEntry{}, fmt.Errorf("catalog: %s is not in staging", f.Source)
		}
		rel := ""
		if !r.Loose {
			rel = domain.FreeRel(domain.SanitizePath(f.Rel), taken)
			taken.Add(rel)
		}
		file, err := domain.NewUnassignedFile(f.Source, rel, f.Size, r.Loose)
		if err != nil {
			return domain.UnassignedEntry{}, err
		}
		entry.Files = append(entry.Files, file)
	}
	return u.repo.Record(ctx, entry)
}

// addMissing records the files of r that the entry does not have yet, each with a place
// free in the entry. A loose entry has no room for another file.
func (u *Unassigned) addMissing(ctx context.Context, entry *domain.UnassignedEntry, r AsideRequest) error {
	known := map[string]bool{}
	taken := domain.NameSet{}
	for _, f := range entry.Files {
		known[f.Source] = true
		taken.Add(f.Rel)
	}
	var add []domain.UnassignedFile
	for _, f := range r.Files {
		if known[f.Source] {
			continue
		}
		if entry.Loose {
			return fmt.Errorf("catalog: entry %q is a loose file and takes no other file", entry.Name)
		}
		rel := domain.FreeRel(domain.SanitizePath(f.Rel), taken)
		taken.Add(rel)
		file, err := domain.NewUnassignedFile(f.Source, rel, f.Size, false)
		if err != nil {
			return err
		}
		add = append(add, file)
		known[f.Source] = true
	}
	if len(add) == 0 {
		return nil
	}
	if err := u.repo.AddFiles(ctx, entry.ID, add); err != nil {
		return err
	}
	entry.Files = append(entry.Files, add...)
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

// moveOne renames the file at src to dst and records it as moved. A file that is not at
// src but is at dst was moved before a failure or a restart: only its record was missing.
func (u *Unassigned) moveOne(ctx context.Context, entryID int64, src, dst string) error {
	inStaging, err := u.files.Exists(src)
	if err != nil {
		return fmt.Errorf("catalog: look at %s: %w", src, err)
	}
	if !inStaging {
		inPlace, err := u.files.Exists(dst)
		if err != nil {
			return fmt.Errorf("catalog: look at %s: %w", dst, err)
		}
		if !inPlace {
			return fmt.Errorf("catalog: %s is neither in staging nor in %s", src, domain.UnassignedDir)
		}
		return u.repo.MarkMoved(ctx, entryID, src)
	}
	if err := u.files.MkdirAll(filepath.Dir(dst)); err != nil {
		return fmt.Errorf("catalog: create %s: %w", filepath.Dir(dst), err)
	}
	if err := u.files.Move(src, dst); err != nil {
		return fmt.Errorf("catalog: move to %s: %w", domain.UnassignedDir, err)
	}
	return u.repo.MarkMoved(ctx, entryID, src)
}
