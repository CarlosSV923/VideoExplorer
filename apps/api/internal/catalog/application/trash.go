// Package application holds the catalog use cases and the ports they need.
package application

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/catalog/domain"
)

// Files is the file system the catalog moves files on: the library and its
// trash, which live on the same dataset so that a move is a rename (RNF-04).
// Every path is built by the server; the adapter refuses anything outside the library.
type Files interface {
	// TrashPath is the path of an entry of the trash, given its elements.
	TrashPath(elem ...string) string
	// MkdirAll creates a folder and its missing parents inside the library.
	MkdirAll(dir string) error
	// Move renames from to to; across file systems it copies and then deletes.
	// It never replaces an existing file.
	Move(from, to string) error
	// RemoveAll deletes a path inside the library, if it exists.
	RemoveAll(path string) error
	// UnassignedPath is the path of an element of the unassigned section (_unassigned/).
	UnassignedPath(elem ...string) string
	// Names lists the names in a folder inside the library; none when the folder is missing.
	Names(dir string) ([]string, error)
	// Exists reports whether anything (a file or a folder) is at path inside the library.
	Exists(path string) (bool, error)
}

// TrashRepository persists the metadata of the trash.
type TrashRepository interface {
	// Insert stores an entry with its files and returns it with its id.
	Insert(ctx context.Context, entry domain.TrashEntry) (domain.TrashEntry, error)
	// Entries returns every entry, oldest first.
	Entries(ctx context.Context) ([]domain.TrashEntry, error)
}

// Trash sends files to the trash with their metadata (RF-30). Listing, restoring
// and purging the trash come in the trash phase (phase 7).
type Trash struct {
	repo  TrashRepository
	files Files
	now   func() time.Time
	newID func() string
}

// NewTrash builds the trash. now may be nil (time.Now).
func NewTrash(repo TrashRepository, files Files, now func() time.Time) *Trash {
	if now == nil {
		now = time.Now
	}
	return &Trash{repo: repo, files: files, now: now, newID: randomName}
}

// randomName returns a random folder name of the trash: hex, never a path.
func randomName() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
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

// Put moves the file at r.Source into a new folder of the trash and records the
// entry. If the record cannot be saved the file goes back to where it was, so a
// file is never in the trash without its metadata.
func (t *Trash) Put(ctx context.Context, r PutRequest) (domain.TrashEntry, error) {
	dir := t.newID()
	entry, err := domain.NewTrashEntry(r.Origin, r.Reason, dir,
		domain.Provenance{BatchID: r.BatchID, RelativePath: r.RelativePath},
		domain.TrashFile{Name: r.Name, Size: r.Size}, t.now())
	if err != nil {
		return domain.TrashEntry{}, err
	}
	folder := t.files.TrashPath(dir)
	if err := t.files.MkdirAll(folder); err != nil {
		return domain.TrashEntry{}, fmt.Errorf("trash folder: %w", err)
	}
	dest := t.files.TrashPath(dir, r.Name)
	if err := t.files.Move(r.Source, dest); err != nil {
		_ = t.files.RemoveAll(folder)
		return domain.TrashEntry{}, fmt.Errorf("move to trash: %w", err)
	}
	saved, err := t.repo.Insert(ctx, entry)
	if err != nil {
		if undo := t.files.Move(dest, r.Source); undo != nil {
			return domain.TrashEntry{}, fmt.Errorf("record trash entry: %w (and the file could not go back: %w)", err, undo)
		}
		_ = t.files.RemoveAll(folder)
		return domain.TrashEntry{}, fmt.Errorf("record trash entry: %w", err)
	}
	return saved, nil
}

// Entries returns the trash entries, oldest first.
func (t *Trash) Entries(ctx context.Context) ([]domain.TrashEntry, error) {
	return t.repo.Entries(ctx)
}
