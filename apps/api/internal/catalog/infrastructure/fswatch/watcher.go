// Package fswatch adapts recursive Linux filesystem notifications to the catalog watcher port.
package fswatch

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
	"golang.org/x/sys/unix"
)

const defaultSettleWindow = 10 * time.Second

func stable(at, now time.Time, settle time.Duration, writers int) bool {
	return writers == 0 && now.Sub(at) >= settle
}

// Change is one entry or title folder whose filesystem activity settled.
type Change struct {
	Area   string
	Root   string
	Group  string // entry or title folder, relative to Root.Path
	Path   string
	Stable bool
}

// Root is a watched subtree and the kind of catalog content it contains.
type Root struct{ Path, Area string }

// Watcher delivers grouped stable filesystem changes from inotify.
type Watcher struct {
	roots    []Root
	settle   time.Duration
	now      func() time.Time
	newFS    func() (watchFS, error)
	ignore   func(string) bool
	watching func(string)
}

type watchFS interface {
	Add(string) error
	Remove(string) error
	Close() error
	Events() <-chan fsnotify.Event
	Errors() <-chan error
}

type nativeFS struct{ *fsnotify.Watcher }

func (n nativeFS) Events() <-chan fsnotify.Event { return n.Watcher.Events }
func (n nativeFS) Errors() <-chan error          { return n.Watcher.Errors }

type writeState struct {
	path  string
	open  bool
	close bool
}

// New builds a recursive watcher with the RF-28a ten-second window.
func New(roots []Root) *Watcher {
	return NewWithIgnore(roots, nil)
}

// NewWithIgnore builds a watcher that also drops paths recognized as app-owned writes.
func NewWithIgnore(roots []Root, ignore func(string) bool) *Watcher {
	w := newWith(roots, defaultSettleWindow, time.Now, func() (watchFS, error) {
		w, err := fsnotify.NewWatcher()
		if err != nil {
			return nil, err
		}
		return nativeFS{w}, nil
	})
	w.ignore = ignore
	return w
}

func newWith(roots []Root, settle time.Duration, now func() time.Time, factory func() (watchFS, error)) *Watcher {
	if now == nil {
		now = time.Now
	}
	return &Watcher{roots: roots, settle: settle, now: now, newFS: factory}
}

// Run watches each tree recursively, extending watches as directories are created.
// Watcher failures are returned to the caller, which launches the RF-28 backup scan.
func (w *Watcher) Run(ctx context.Context, onChange func(Change)) error {
	watcher, err := w.newFS()
	if err != nil {
		return fmt.Errorf("create inotify watcher: %w", err)
	}
	defer watcher.Close()
	closeFD, err := unix.InotifyInit()
	if err != nil {
		return fmt.Errorf("create close-write watcher: %w", err)
	}
	defer unix.Close(closeFD)
	writeStates := make(chan writeState, 256)
	closeErrors := make(chan error, 1)
	var closeDirs sync.Map
	go readCloseWrites(ctx, closeFD, writeStates, closeErrors, &closeDirs)
	var mu sync.Mutex
	watched := map[string]bool{}
	closeWatched := map[string]uint32{}
	addTree := func(root Root, start string) error {
		return filepath.WalkDir(start, func(p string, d fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				if errors.Is(walkErr, fs.ErrNotExist) {
					return nil
				}
				return walkErr
			}
			if !d.IsDir() || ignored(root, p) {
				if d.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}
			mu.Lock()
			already := watched[p]
			mu.Unlock()
			if already {
				return nil
			}
			if err := watcher.Add(p); err != nil {
				return err
			}
			mu.Lock()
			_, closeAlready := closeWatched[p]
			mu.Unlock()
			if !closeAlready {
				wd, err := unix.InotifyAddWatch(closeFD, p, unix.IN_OPEN|unix.IN_CLOSE_NOWRITE|unix.IN_CLOSE_WRITE)
				if err != nil {
					return err
				}
				if wd < 0 || uint64(wd) > uint64(^uint32(0)) {
					return fmt.Errorf("inotify watch descriptor out of range: %d", wd)
				}
				closeWD := uint32(wd)
				closeDirs.Store(closeWD, p)
				mu.Lock()
				closeWatched[p] = closeWD
				mu.Unlock()
			}
			mu.Lock()
			watched[p] = true
			mu.Unlock()
			if w.watching != nil {
				w.watching(p)
			}
			return nil
		})
	}
	removeTree := func(path string) {
		mu.Lock()
		var dirs []string
		for watchedPath := range watched {
			if watchedPath == path || strings.HasPrefix(watchedPath, path+string(filepath.Separator)) {
				dirs = append(dirs, watchedPath)
			}
		}
		for _, dir := range dirs {
			delete(watched, dir)
		}
		mu.Unlock()
		for _, dir := range dirs {
			_ = watcher.Remove(dir)
			mu.Lock()
			wd := closeWatched[dir]
			delete(closeWatched, dir)
			mu.Unlock()
			if wd != 0 {
				_, _ = unix.InotifyRmWatch(closeFD, wd)
				closeDirs.Delete(wd)
			}
		}
	}
	for _, root := range w.roots {
		start := root.Path
		if _, statErr := os.Stat(start); errors.Is(statErr, fs.ErrNotExist) {
			start = filepath.Dir(start)
		}
		anchor := root
		if start != root.Path {
			anchor.Path = start
		}
		if err := addTree(anchor, start); err != nil {
			return fmt.Errorf("watch %s: %w", root.Path, err)
		}
	}
	type pending struct {
		change    Change
		at        time.Time
		writing   map[string]bool
		signature string
		sampled   bool
	}
	items := map[string]*pending{}
	activeWrites := map[string]int{}
	keyFor := func(root Root, path string) (string, string) {
		rel, err := filepath.Rel(root.Path, path)
		if err != nil || rel == "." {
			return "", ""
		}
		parts := strings.Split(filepath.ToSlash(rel), "/")
		if len(parts) == 0 || parts[0] == "" {
			return "", ""
		}
		group := parts[0]
		return root.Area + string(filepath.Separator) + group, group
	}
	tick := time.NewTicker(200 * time.Millisecond)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case err, ok := <-watcher.Errors():
			if !ok {
				return errors.New("inotify error channel closed")
			}
			if errors.Is(err, fsnotify.ErrEventOverflow) {
				return fmt.Errorf("inotify event overflow: %w", err)
			}
			return fmt.Errorf("inotify watcher failed: %w", err)
		case err := <-closeErrors:
			return fmt.Errorf("inotify close-write watcher failed: %w", err)
		case state := <-writeStates:
			root, ok := w.rootFor(state.path)
			if !ok || ignored(root, state.path) || (w.ignore != nil && w.ignore(state.path)) {
				continue
			}
			key, _ := keyFor(root, state.path)
			if p := items[key]; p != nil {
				if state.open {
					p.writing[state.path] = true
				}
				if state.close {
					delete(p.writing, state.path)
				}
			}
			if state.open {
				activeWrites[state.path]++
			}
			if state.close {
				activeWrites[state.path]--
				if activeWrites[state.path] <= 0 {
					delete(activeWrites, state.path)
				}
			}
		case event, ok := <-watcher.Events():
			if !ok {
				return errors.New("inotify event channel closed")
			}
			path, err := filepath.Abs(event.Name)
			if err != nil {
				continue
			}
			root, ok := w.rootFor(path)
			if !ok || ignored(root, path) {
				continue
			}
			if event.Op&fsnotify.Create != 0 {
				if fi, statErr := os.Lstat(path); statErr == nil && fi.IsDir() && fi.Mode()&fs.ModeSymlink == 0 {
					if err := addTree(root, path); err != nil {
						return fmt.Errorf("watch new directory %s: %w", path, err)
					}
				}
			}
			if w.ignore != nil && w.ignore(path) {
				continue
			}
			key, group := keyFor(root, path)
			if key == "" {
				continue
			}
			p := items[key]
			if p == nil {
				p = &pending{writing: map[string]bool{}}
				items[key] = p
				for openPath, count := range activeWrites {
					if count > 0 {
						openRoot, inRoot := w.rootFor(openPath)
						openKey, _ := keyFor(openRoot, openPath)
						if inRoot && openKey == key {
							p.writing[openPath] = true
						}
					}
				}
			}
			p.change = Change{Area: root.Area, Root: filepath.Base(root.Path), Group: group, Path: path}
			p.at, p.signature, p.sampled = w.now(), "", false
			if event.Op&fsnotify.Write != 0 {
				if activeWrites[path] > 0 {
					p.writing[path] = true
				}
			}
			onChange(p.change)
			if event.Op&(fsnotify.Remove|fsnotify.Rename) != 0 {
				delete(p.writing, path)
				removeTree(path)
			}
		case <-tick.C:
			now := w.now()
			for key, p := range items {
				if !stable(p.at, now, w.settle, len(p.writing)) {
					continue
				}
				root, ok := w.rootFor(p.change.Path)
				if !ok {
					root = w.rootByArea(p.change.Area, p.change.Path)
				}
				if root.Path == "" {
					continue
				}
				sig := signature(filepath.Join(root.Path, p.change.Group))
				if !p.sampled || sig != p.signature {
					p.signature, p.sampled, p.at = sig, true, now
					continue
				}
				p.change.Stable = true
				onChange(p.change)
				delete(items, key)
			}
		}
	}
}

func readCloseWrites(ctx context.Context, fd int, out chan<- writeState, errs chan<- error, dirs *sync.Map) {
	file := os.NewFile(uintptr(fd), "inotify-close-write")
	buf := make([]byte, 64*1024)
	for {
		n, err := file.Read(buf)
		if err != nil {
			if ctx.Err() == nil {
				select {
				case errs <- err:
				default:
				}
			}
			return
		}
		for offset := 0; offset+unix.SizeofInotifyEvent <= n; {
			wd := binary.NativeEndian.Uint32(buf[offset : offset+4])
			mask := binary.NativeEndian.Uint32(buf[offset+4 : offset+8])
			nameLen := binary.NativeEndian.Uint32(buf[offset+12 : offset+16])
			nameStart := offset + unix.SizeofInotifyEvent
			nameEnd := nameStart + int(nameLen)
			if nameEnd > n {
				break
			}
			if mask&unix.IN_Q_OVERFLOW != 0 {
				select {
				case errs <- fsnotify.ErrEventOverflow:
				default:
				}
				return
			}
			if mask&(unix.IN_OPEN|unix.IN_CLOSE_NOWRITE|unix.IN_CLOSE_WRITE) != 0 {
				name := strings.TrimRight(string(buf[nameStart:nameEnd]), "\x00")
				dir, _ := dirs.Load(wd)
				if directory, ok := dir.(string); ok {
					state := writeState{
						path:  filepath.Join(directory, name),
						open:  mask&unix.IN_OPEN != 0,
						close: mask&(unix.IN_CLOSE_NOWRITE|unix.IN_CLOSE_WRITE) != 0,
					}
					select {
					case out <- state:
					case <-ctx.Done():
						return
					}
				}
			}
			offset = nameEnd
		}
	}
}

func (w *Watcher) rootFor(p string) (Root, bool) {
	var found Root
	for _, r := range w.roots {
		rel, err := filepath.Rel(r.Path, p)
		if err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && len(r.Path) > len(found.Path) {
			found = r
		}
	}
	return found, found.Path != ""
}

func (w *Watcher) rootByArea(area, p string) Root {
	for _, r := range w.roots {
		if r.Area == area && strings.HasPrefix(p, r.Path+string(filepath.Separator)) {
			return r
		}
	}
	return Root{}
}

func signature(root string) string {
	var b strings.Builder
	_ = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.Type()&fs.ModeSymlink != 0 {
			return filepath.SkipDir
		}
		fi, err := d.Info()
		if err == nil {
			fmt.Fprintf(&b, "%s:%d:%d;", p, fi.Size(), fi.ModTime().UnixNano())
		}
		return nil
	})
	return b.String() // empty also represents a stably-removed entry
}

func ignored(root Root, p string) bool {
	rel, err := filepath.Rel(root.Path, p)
	if err != nil {
		return true
	}
	for _, part := range strings.Split(filepath.ToSlash(rel), "/") {
		if part == "." || part == "" {
			continue
		}
		if strings.HasPrefix(part, ".") || strings.HasPrefix(part, "@") || strings.HasPrefix(part, "#") || strings.HasPrefix(part, "$") {
			return true
		}
		switch strings.ToLower(part) {
		case "thumbs.db", "desktop.ini", ".ds_store":
			return true
		}
	}
	return false
}
