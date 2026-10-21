package fswatch

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/fsnotify/fsnotify"
)

func TestStable(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		name    string
		at      time.Time
		elapsed time.Duration
		writers int
		want    bool
	}{
		{name: "window not elapsed", at: now, elapsed: 9 * time.Second, want: false},
		{name: "window elapsed", at: now, elapsed: 10 * time.Second, want: true},
		{name: "write handle open", at: now, elapsed: time.Hour, writers: 1, want: false},
		{name: "clock moved backwards", at: now, elapsed: -time.Second, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := stable(tt.at, tt.at.Add(tt.elapsed), 10*time.Second, tt.writers); got != tt.want {
				t.Fatalf("stable() = %t, want %t", got, tt.want)
			}
		})
	}
}

func TestIgnored(t *testing.T) {
	t.Parallel()
	root := Root{Path: t.TempDir(), Area: "library"}
	for _, path := range []string{".videoexplorer/staging/file.mkv", "@eaDir/file.mkv", "Thumbs.db", "desktop.ini", ".hidden/file.mkv", "#recycle/file"} {
		if !ignored(root, filepath.Join(root.Path, filepath.FromSlash(path))) {
			t.Errorf("ignored(%q) = false", path)
		}
	}
	if ignored(root, filepath.Join(root.Path, "Title", "Movie.mkv")) {
		t.Fatal("ordinary title was ignored")
	}
}

func TestWatcherFindsNestedDirectoriesAndIgnoresJunk(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	events := make(chan Change, 10)
	w := newWith([]Root{{Path: root, Area: "unassigned"}}, 0, time.Now, func() (watchFS, error) {
		fsw, err := fsnotify.NewWatcher()
		if err != nil {
			return nil, err
		}
		return nativeFS{fsw}, nil
	})
	watched := make(chan string, 10)
	w.watching = func(path string) { watched <- path }
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- w.Run(ctx, func(change Change) { events <- change }) }()
	select {
	case <-watched:
	case <-time.After(10 * time.Second):
		t.Fatal("root watch was not installed")
	}

	entry := filepath.Join(root, "Slow Copy")
	if err := os.Mkdir(entry, 0o750); err != nil {
		t.Fatal(err)
	}
	select {
	case <-watched:
	case <-time.After(10 * time.Second):
		t.Fatal("new entry was not watched")
	}
	nested := filepath.Join(entry, "disc")
	if err := os.Mkdir(nested, 0o750); err != nil {
		t.Fatal(err)
	}
	select {
	case <-watched:
	case <-time.After(10 * time.Second):
		t.Fatal("new nested folder was not watched")
	}
	nested = filepath.Join(nested, "sub")
	if err := os.Mkdir(nested, 0o750); err != nil {
		t.Fatal(err)
	}
	select {
	case <-watched:
	case <-time.After(10 * time.Second):
		t.Fatal("nested child folder was not watched")
	}
	file := filepath.Join(nested, "movie.mkv")
	f, err := os.Create(file)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.Write([]byte("partial")); err != nil {
		t.Fatal(err)
	}
	for {
		select {
		case got := <-events:
			if got.Path == file && !got.Stable {
				goto firstWriteSeen
			}
		case <-time.After(10 * time.Second):
			t.Fatal("watcher did not report the open file")
		}
	}
firstWriteSeen:
	if _, err := f.Write([]byte("-more-data")); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	if info, err := os.Stat(file); err != nil || info.Size() != int64(len("partial-more-data")) {
		t.Fatalf("growing file size = %v, err = %v", info, err)
	}
	seenPending := false
	for {
		select {
		case got := <-events:
			if got.Area != "unassigned" || got.Group != "Slow Copy" {
				t.Fatalf("change = %+v", got)
			}
			if !got.Stable {
				seenPending = true
				continue
			}
			if !seenPending {
				t.Fatal("stable notification arrived without a copying notification")
			}
			goto settled
		case <-time.After(10 * time.Second):
			t.Fatal("watcher did not report the nested copy")
		}
	}
settled:

	cancel()
	if err := <-done; err != nil {
		t.Fatalf("watcher: %v", err)
	}
}

func TestWatcherCoalescesManyWritesIntoOneStableChange(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	settle := 100 * time.Millisecond
	changes := make(chan Change, 256)
	w := newWith([]Root{{Path: root, Area: "library"}}, settle, time.Now, func() (watchFS, error) {
		fsw, err := fsnotify.NewWatcher()
		if err != nil {
			return nil, err
		}
		return nativeFS{fsw}, nil
	})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- w.Run(ctx, func(change Change) { changes <- change }) }()

	file := filepath.Join(root, "Godzilla (2014)", "Godzilla.mkv")
	if err := os.Mkdir(filepath.Dir(file), 0o750); err != nil {
		t.Fatal(err)
	}
	f, err := os.Create(file)
	if err != nil {
		t.Fatal(err)
	}
	block := make([]byte, 64*1024)
	for range 64 {
		if _, err := f.Write(block); err != nil {
			t.Fatal(err)
		}
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}

	changeCount := 0
	deadline := time.After(5 * time.Second)
	for changeCount == 0 {
		select {
		case change := <-changes:
			changeCount++
			if !change.Stable {
				t.Fatalf("library change = %+v, want only a stable notification", change)
			}
			if change.Group != "Godzilla (2014)" {
				t.Fatalf("stable group = %q, want title folder", change.Group)
			}
		case <-deadline:
			t.Fatal("watcher did not report a stable change")
		}
	}

	quiet := time.NewTimer(2 * settle)
	defer quiet.Stop()
	for {
		select {
		case change := <-changes:
			changeCount++
			if !change.Stable {
				t.Fatalf("library change = %+v, want only stable notifications", change)
			}
		case <-quiet.C:
			if changeCount != 1 {
				t.Fatalf("library changes = %d, want one for the write burst", changeCount)
			}
			cancel()
			if err := <-done; err != nil {
				t.Fatalf("watcher: %v", err)
			}
			return
		}
	}
}
