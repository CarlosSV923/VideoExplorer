// Package libraryfs implements the catalog's file system port on the library
// dataset (RNF-04). The trash lives inside the library, so a move is a rename;
// across file systems (EXDEV) a move becomes a copy and a delete. The copy goes
// under a partial name that only becomes the destination once complete.
package libraryfs

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/catalog/application"
	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/catalog/domain"
)

// partialSuffix marks a cross-device copy in progress.
const partialSuffix = ".videoexplorer-partial"

// FS implements application.Files.
type FS struct {
	root  string
	trash string
}

var _ application.Files = (*FS)(nil)

// New builds the file system of a library root; the trash must be inside it.
func New(root, trash string) *FS {
	return &FS{root: filepath.Clean(root), trash: filepath.Clean(trash)}
}

// TrashPath implements application.Files.
func (f *FS) TrashPath(elem ...string) string {
	return filepath.Join(append([]string{f.trash}, elem...)...)
}

// UnassignedPath implements application.Files.
func (f *FS) UnassignedPath(elem ...string) string {
	return filepath.Join(append([]string{f.root, domain.UnassignedDir}, elem...)...)
}

// Names implements application.Files.
func (f *FS) Names(dir string) ([]string, error) {
	if err := f.inside(dir); err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		names = append(names, e.Name())
	}
	return names, nil
}

// Exists implements application.Files.
func (f *FS) Exists(p string) (bool, error) {
	if err := f.inside(p); err != nil {
		return false, err
	}
	_, err := os.Lstat(p)
	switch {
	case err == nil:
		return true, nil
	case errors.Is(err, fs.ErrNotExist):
		return false, nil
	default:
		return false, err
	}
}

// RemoveAll implements application.Files.
func (f *FS) RemoveAll(p string) error {
	if err := f.inside(p); err != nil {
		return err
	}
	return os.RemoveAll(p)
}

// inside rejects paths outside the library, and the root itself. Every path is
// built by the server, so this is the last line of defence against a naming bug.
func (f *FS) inside(paths ...string) error {
	for _, p := range paths {
		rel, err := filepath.Rel(f.root, p)
		if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || !filepath.IsAbs(p) {
			return fmt.Errorf("libraryfs: refusing %q outside the library", p)
		}
	}
	return nil
}

// MkdirAll implements application.Files.
func (f *FS) MkdirAll(dir string) error {
	if err := f.inside(dir); err != nil {
		return err
	}
	var missing []string
	for d := dir; d != f.root; d = filepath.Dir(d) {
		if _, err := os.Lstat(d); err == nil {
			break
		}
		missing = append([]string{d}, missing...)
	}
	for _, d := range missing {
		if err := os.Mkdir(d, 0o750); err != nil && !errors.Is(err, fs.ErrExist) {
			return err
		}
	}
	return nil
}

// Move implements application.Files.
func (f *FS) Move(from, to string) error {
	if err := f.inside(from, to); err != nil {
		return err
	}
	if _, err := os.Lstat(to); err == nil {
		return fmt.Errorf("%w: %s", fs.ErrExist, to)
	}
	err := os.Rename(from, to)
	if !errors.Is(err, syscall.EXDEV) {
		return err
	}
	partial := to + partialSuffix
	_ = os.RemoveAll(partial)
	if err := copyFile(from, partial); err != nil {
		_ = os.RemoveAll(partial)
		return fmt.Errorf("copy across file systems: %w", err)
	}
	if err := os.Rename(partial, to); err != nil {
		_ = os.RemoveAll(partial)
		return err
	}
	return os.Remove(from)
}

// copyFile streams a regular file to a new file, syncing it before it returns.
func copyFile(from, to string) error {
	info, err := os.Lstat(from)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("%s: not a regular file", from)
	}
	in, err := os.Open(from) //nolint:gosec // paths are checked by inside()
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(to, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o640) //nolint:gosec // idem
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		return err
	}
	if err := out.Sync(); err != nil {
		_ = out.Close()
		return err
	}
	return out.Close()
}
