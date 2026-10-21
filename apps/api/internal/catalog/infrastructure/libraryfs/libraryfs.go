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
	"sync"
	"syscall"
	"time"

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
	rename     func(from, to string) error
	selfMu     sync.Mutex
	selfWrites map[string]time.Time
}

var (
	_ application.Files         = (*FS)(nil)
	_ application.DownloadFiles = (*FS)(nil)
)

// New builds the file system of a library root. Its trash and its operations' scratch
// area are .videoexplorer/trash and .videoexplorer/ops.
func New(root string) *FS {
	root = filepath.Clean(root)
	return &FS{
		root:       root,
		trash:      filepath.Join(root, metaDir, "trash"),
		ops:        filepath.Join(root, metaDir, "ops"),
		rename:     os.Rename,
		selfWrites: map[string]time.Time{},
	}
}

// IsAppWrite reports paths recently changed by the journaled operation engine.
func (f *FS) IsAppWrite(p string) bool {
	p, _ = filepath.Abs(p)
	f.selfMu.Lock()
	defer f.selfMu.Unlock()
	now := time.Now()
	for path, until := range f.selfWrites {
		if now.After(until) {
			delete(f.selfWrites, path)
			continue
		}
		if p == path || strings.HasPrefix(p, path+string(filepath.Separator)) {
			return true
		}
	}
	return false
}

func (f *FS) markAppWrite(paths ...string) {
	f.selfMu.Lock()
	defer f.selfMu.Unlock()
	until := time.Now().Add(time.Minute)
	for _, p := range paths {
		abs, err := filepath.Abs(p)
		if err == nil {
			f.selfWrites[abs] = until
		}
	}
}

// TrashPath implements application.Files.
func (f *FS) TrashPath(elem ...string) string {
	return filepath.Join(append([]string{f.trash}, elem...)...)
}

// LibraryPath implements application.Files.
func (f *FS) LibraryPath(elem ...string) string {
	return filepath.Join(append([]string{f.root}, elem...)...)
}

// Size implements application.Files: the size of a regular file, refused when it goes through a link.
func (f *FS) Size(p string) (int64, error) {
	info, err := f.Info(p)
	return info.Size, err
}

// Info implements application.DownloadFiles: metadata of a regular file,
// refused when the path is outside the library or goes through a link.
func (f *FS) Info(p string) (application.FileInfo, error) {
	if err := f.readable(p); err != nil {
		return application.FileInfo{}, err
	}
	info, err := os.Lstat(p)
	if err != nil {
		return application.FileInfo{}, err
	}
	if !info.Mode().IsRegular() {
		return application.FileInfo{}, fmt.Errorf("libraryfs: %s is not a regular file", p)
	}
	return application.FileInfo{Size: info.Size(), Modified: info.ModTime()}, nil
}

// ReadHeader returns the first n bytes of the regular file at p inside the library, fewer when the file is
// shorter (RF-27a: the archives are recognized by their first bytes). A link is never followed (RNF-02).
func (f *FS) ReadHeader(p string, n int) ([]byte, error) {
	if err := f.readable(p); err != nil {
		return nil, err
	}
	info, err := os.Lstat(p)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("libraryfs: %s is not a regular file", p)
	}
	file, err := os.Open(p) //nolint:gosec // p was checked by readable and is a regular file of the library
	if err != nil {
		return nil, err
	}
	defer func() { _ = file.Close() }()
	buf := make([]byte, n)
	m, err := io.ReadFull(file, buf)
	if err != nil && !errors.Is(err, io.ErrUnexpectedEOF) && !errors.Is(err, io.EOF) {
		return nil, err
	}
	return buf[:m], nil
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

// Tree implements application.Files: everything inside the folder, recursively, except the links
// (RNF-02). An entry that vanishes while the walk goes on is skipped, as the disk no longer holds it.
func (f *FS) Tree(dir string) ([]domain.Item, error) {
	if err := f.readable(dir); err != nil {
		return nil, err
	}
	info, err := os.Lstat(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("libraryfs: %s is not a folder", dir)
	}
	var out []domain.Item
	err = filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				return nil
			}
			return err
		}
		if p == dir {
			return nil
		}
		if d.Type()&fs.ModeSymlink != 0 {
			return nil
		}
		rel, err := filepath.Rel(dir, p)
		if err != nil {
			return err
		}
		fi, err := d.Info()
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		if err != nil {
			return err
		}
		out = append(out, domain.Item{Rel: filepath.ToSlash(rel), IsDir: d.IsDir(), Size: fi.Size(), ModTime: fi.ModTime()})
		return nil
	})
	return out, err
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

// Open implements application.DownloadFiles. A symbolic link is rejected
// before os.Open so downloads never follow links (RNF-02).
func (f *FS) Open(p string) (io.ReadSeekCloser, error) {
	if err := f.readable(p); err != nil {
		return nil, err
	}
	info, err := os.Lstat(p)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("libraryfs: %s is not a regular file", p)
	}
	return os.Open(p) //nolint:gosec // checked by readable and Lstat above
}

// RemoveAll implements application.Files.
func (f *FS) RemoveAll(p string) error {
	if err := f.inside(p); err != nil {
		return err
	}
	f.markAppWrite(p)
	return os.RemoveAll(p)
}

// RemoveCopy implements application.Files.
func (f *FS) RemoveCopy(p string) error {
	if err := f.inside(p); err != nil {
		return err
	}
	partial := p + partialSuffix
	f.markAppWrite(partial)
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
	f.markAppWrite(dir)
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
	f.markAppWrite(dir)
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
	f.markAppWrite(from, to)
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
