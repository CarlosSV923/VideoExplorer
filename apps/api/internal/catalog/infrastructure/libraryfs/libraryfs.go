// Package libraryfs implements the catalog's file system port on the library
// dataset (RNF-04). The trash and the operations' scratch area live inside the library,
// so a move is a rename; across file systems (EXDEV) a move becomes a copy and a delete.
// The copy goes under a partial name that only becomes the destination once complete.
// Every path must be inside the library, and no path may go through a symbolic link.
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

// metaDir is the folder of the app inside the library: staging, trash and operations live there.
const metaDir = ".videoexplorer"

// FS implements application.Files.
type FS struct {
	root  string
	trash string
	ops   string
	// rename is os.Rename; a test replaces it to simulate a move across file systems.
	rename func(from, to string) error
}

var _ application.Files = (*FS)(nil)

// New builds the file system of a library root. Its trash and its operations' scratch
// area are .videoexplorer/trash and .videoexplorer/ops.
func New(root string) *FS {
	root = filepath.Clean(root)
	return &FS{
		root:   root,
		trash:  filepath.Join(root, metaDir, "trash"),
		ops:    filepath.Join(root, metaDir, "ops"),
		rename: os.Rename,
	}
}

// TrashPath implements application.Files.
func (f *FS) TrashPath(elem ...string) string {
	return filepath.Join(append([]string{f.trash}, elem...)...)
}

// UnassignedPath implements application.Files.
func (f *FS) UnassignedPath(elem ...string) string {
	return filepath.Join(append([]string{f.root, domain.UnassignedDir}, elem...)...)
}

// OpsPath implements application.Files.
func (f *FS) OpsPath(elem ...string) string {
	return filepath.Join(append([]string{f.ops}, elem...)...)
}

// Names implements application.Files.
func (f *FS) Names(dir string) ([]string, error) {
	if err := f.readable(dir); err != nil {
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
	if err := f.readable(p); err != nil {
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

// RemoveCopy implements application.Files.
func (f *FS) RemoveCopy(p string) error {
	if err := f.inside(p); err != nil {
		return err
	}
	partial := p + partialSuffix
	if _, err := os.Lstat(partial); errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	return os.Remove(partial)
}

// inside rejects paths outside the library, the root itself, and paths that go through a
// symbolic link. Every path is built by the server, so this is the last line of defence
// against a naming bug. Changes use it; reads use readable, which also accepts the root.
func (f *FS) inside(paths ...string) error {
	return f.check(false, paths...)
}

// readable is inside for the reads, which may look at the root itself.
func (f *FS) readable(paths ...string) error {
	return f.check(true, paths...)
}

func (f *FS) check(rootOK bool, paths ...string) error {
	for _, p := range paths {
		rel, err := filepath.Rel(f.root, p)
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || !filepath.IsAbs(p) {
			return fmt.Errorf("libraryfs: refusing %q outside the library", p)
		}
		if rel == "." {
			if rootOK {
				continue
			}
			return fmt.Errorf("libraryfs: refusing the library root %q", p)
		}
		if err := f.noLinks(rel); err != nil {
			return err
		}
	}
	return nil
}

// noLinks rejects a path (relative to the root) when one of its folders or the path itself is
// a symbolic link. The parts that do not exist yet cannot be links.
func (f *FS) noLinks(rel string) error {
	cur := f.root
	for _, part := range strings.Split(rel, string(filepath.Separator)) {
		cur = filepath.Join(cur, part)
		info, err := os.Lstat(cur)
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		if err != nil {
			return err
		}
		if info.Mode()&fs.ModeSymlink != 0 {
			return fmt.Errorf("libraryfs: refusing %q: it goes through a symbolic link", cur)
		}
	}
	return nil
}

// Mkdir implements application.Files.
func (f *FS) Mkdir(dir string) error {
	if err := f.inside(dir); err != nil {
		return err
	}
	return os.Mkdir(dir, 0o750)
}

// RemoveDir implements application.Files.
func (f *FS) RemoveDir(dir string) error {
	if err := f.inside(dir); err != nil {
		return err
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	if len(entries) > 0 {
		return fmt.Errorf("%w: %s", application.ErrNotEmpty, dir)
	}
	return os.Remove(dir)
}

// Move implements application.Files. A source that is a symbolic link is refused: the
// link would be moved, not the file it points to.
func (f *FS) Move(from, to string) error {
	if err := f.inside(from, to); err != nil {
		return err
	}
	info, err := os.Lstat(from)
	if err != nil {
		return err
	}
	if info.Mode()&fs.ModeSymlink != 0 {
		return fmt.Errorf("libraryfs: refusing to move the symbolic link %q", from)
	}
	if _, err := os.Lstat(to); err == nil {
		return fmt.Errorf("%w: %s", fs.ErrExist, to)
	}
	err = f.rename(from, to)
	if !errors.Is(err, syscall.EXDEV) {
		return err
	}
	partial := to + partialSuffix
	_ = os.RemoveAll(partial)
	if err := copyFile(from, partial); err != nil {
		_ = os.RemoveAll(partial)
		return fmt.Errorf("copy across file systems: %w", err)
	}
	if err := f.rename(partial, to); err != nil {
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
