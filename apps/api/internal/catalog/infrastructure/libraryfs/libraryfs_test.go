package libraryfs

import (
	"errors"
	iofs "io/fs"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/catalog/application"
)

// fixture returns a library root, with the app's folder inside it.
func fixture(t *testing.T) (*FS, string) {
	t.Helper()
	root := t.TempDir()
	return New(root), root
}

// exdev makes the first rename of a move fail as a move across file systems does. The rename
// of the finished copy into place is a rename on the same file system, and works.
func exdev(f *FS) {
	f.rename = func(from, to string) error {
		if strings.HasSuffix(from, partialSuffix) {
			return os.Rename(from, to)
		}
		return &os.LinkError{Op: "rename", Old: from, New: to, Err: syscall.EXDEV}
	}
}

func TestMoveRenamesInsideTheLibraryAndNeverReplaces(t *testing.T) {
	t.Parallel()

	fs, root := fixture(t)
	from := filepath.Join(root, "a.zip")
	to := filepath.Join(root, "b.zip")
	if err := os.WriteFile(from, []byte("one"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := fs.Move(from, to); err != nil {
		t.Fatalf("Move: %v", err)
	}
	if data, err := os.ReadFile(to); err != nil || string(data) != "one" {
		t.Fatalf("moved file = %q, %v", data, err)
	}
	if _, err := os.Stat(from); !os.IsNotExist(err) {
		t.Fatalf("source still there: %v", err)
	}

	if err := os.WriteFile(from, []byte("two"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := fs.Move(from, to); !errors.Is(err, iofs.ErrExist) {
		t.Fatalf("Move over an existing file = %v, want ErrExist", err)
	}
	if data, _ := os.ReadFile(to); string(data) != "one" {
		t.Fatalf("destination was overwritten: %q", data)
	}
}

func TestPathsOutsideTheLibraryAreRefused(t *testing.T) {
	t.Parallel()

	fs, root := fixture(t)
	outside := filepath.Join(filepath.Dir(root), "outside.zip")
	inside := filepath.Join(root, "inside.zip")
	if err := os.WriteFile(inside, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := fs.Move(inside, outside); err == nil {
		t.Error("Move wrote outside the library")
	}
	if err := fs.Move(outside, inside); err == nil {
		t.Error("Move read from outside the library")
	}
	if err := fs.RemoveAll(filepath.Dir(root)); err == nil {
		t.Error("RemoveAll removed a folder outside the library")
	}
	if err := fs.RemoveAll(root); err == nil {
		t.Error("RemoveAll removed the library root")
	}
	if err := fs.Mkdir(filepath.Join(filepath.Dir(root), "elsewhere")); err == nil {
		t.Error("Mkdir created a folder outside the library")
	}
	if _, err := fs.Exists(outside); err == nil {
		t.Error("Exists looked outside the library")
	}
	if err := fs.Mkdir(root); err == nil {
		t.Error("Mkdir accepted the library root")
	}
}

func TestMkdirCreatesOneFolderAndRefusesAnExistingOne(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		dir     func(root string) string
		wantErr error
	}{
		{name: "a new folder in the library", dir: func(root string) string { return filepath.Join(root, "Movies") }},
		{name: "a folder that exists", dir: func(root string) string {
			if err := os.Mkdir(filepath.Join(root, "Movies"), 0o750); err != nil {
				panic(err)
			}
			return filepath.Join(root, "Movies")
		}, wantErr: iofs.ErrExist},
		{name: "a folder whose parent is missing", dir: func(root string) string { return filepath.Join(root, "Missing", "Movie") }, wantErr: iofs.ErrNotExist},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			fs, root := fixture(t)
			dir := tt.dir(root)
			err := fs.Mkdir(dir)
			if tt.wantErr == nil && err != nil {
				t.Fatalf("Mkdir: %v", err)
			}
			if tt.wantErr != nil && !errors.Is(err, tt.wantErr) {
				t.Fatalf("Mkdir = %v, want %v", err, tt.wantErr)
			}
		})
	}
}

func TestRemoveDirOnlyRemovesEmptyFolders(t *testing.T) {
	t.Parallel()

	fs, root := fixture(t)
	empty := filepath.Join(root, "empty")
	full := filepath.Join(root, "full")
	if err := os.MkdirAll(empty, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(full, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(full, "kept.mkv"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := fs.RemoveDir(full); !errors.Is(err, application.ErrNotEmpty) {
		t.Errorf("RemoveDir of a folder with a file = %v, want ErrNotEmpty", err)
	}
	if _, err := os.Stat(filepath.Join(full, "kept.mkv")); err != nil {
		t.Errorf("the file in the folder was touched: %v", err)
	}
	if err := fs.RemoveDir(empty); err != nil {
		t.Errorf("RemoveDir of an empty folder: %v", err)
	}
	if _, err := os.Stat(empty); !os.IsNotExist(err) {
		t.Errorf("the empty folder is still there: %v", err)
	}
}

func TestMoveAcrossFileSystemsCopiesThenDeletes(t *testing.T) {
	t.Parallel()

	fs, root := fixture(t)
	exdev(fs)
	from := filepath.Join(root, "staged.mkv")
	to := filepath.Join(root, "Movies", "Movie.mkv")
	if err := os.WriteFile(from, []byte("video bytes"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Dir(to), 0o750); err != nil {
		t.Fatal(err)
	}

	if err := fs.Move(from, to); err != nil {
		t.Fatalf("Move across file systems: %v", err)
	}
	if data, err := os.ReadFile(to); err != nil || string(data) != "video bytes" {
		t.Fatalf("copied file = %q, %v", data, err)
	}
	if _, err := os.Stat(from); !os.IsNotExist(err) {
		t.Errorf("the source is still there after the copy: %v", err)
	}
	if _, err := os.Stat(to + partialSuffix); !os.IsNotExist(err) {
		t.Errorf("the partial copy is left behind: %v", err)
	}
}

func TestAFailedCopyAcrossFileSystemsLeavesTheSourceAndNoPartialCopy(t *testing.T) {
	t.Parallel()

	fs, root := fixture(t)
	exdev(fs)
	from := filepath.Join(root, "staged")
	to := filepath.Join(root, "Movie")
	if err := os.Mkdir(from, 0o750); err != nil { // a folder cannot be copied as a file: the copy fails
		t.Fatal(err)
	}

	if err := fs.Move(from, to); err == nil {
		t.Fatal("Move of a folder across file systems succeeded")
	}
	if _, err := os.Stat(from); err != nil {
		t.Errorf("the source was touched: %v", err)
	}
	if _, err := os.Stat(to + partialSuffix); !os.IsNotExist(err) {
		t.Errorf("the partial copy is left behind: %v", err)
	}
}

func TestRemoveCopyDeletesOnlyThePartialCopyOfAPath(t *testing.T) {
	t.Parallel()

	fs, root := fixture(t)
	target := filepath.Join(root, "Movie.mkv")
	if err := os.WriteFile(target+partialSuffix, []byte("half"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "other.mkv"), []byte("other"), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := fs.RemoveCopy(target); err != nil {
		t.Fatalf("RemoveCopy: %v", err)
	}
	if _, err := os.Stat(target + partialSuffix); !os.IsNotExist(err) {
		t.Errorf("the partial copy is still there: %v", err)
	}
	if err := fs.RemoveCopy(target); err != nil {
		t.Errorf("RemoveCopy with no partial copy: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "other.mkv")); err != nil {
		t.Errorf("another file was touched: %v", err)
	}
}

// TestSymbolicLinksAreRefused: a link inside the library is never followed, and a link is
// never moved in place of the file it points to.
func TestSymbolicLinksAreRefused(t *testing.T) {
	t.Parallel()

	fs, root := fixture(t)
	realDir := filepath.Join(root, "Movies", "Real")
	if err := os.MkdirAll(realDir, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(realDir, "movie.mkv"), []byte("video"), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "Movies", "Link")
	if err := os.Symlink(realDir, link); err != nil {
		t.Fatal(err)
	}
	fileLink := filepath.Join(root, "Movies", "file.mkv")
	if err := os.Symlink(filepath.Join(realDir, "movie.mkv"), fileLink); err != nil {
		t.Fatal(err)
	}

	if _, err := fs.Exists(filepath.Join(link, "movie.mkv")); err == nil {
		t.Error("Exists followed a folder link")
	}
	if err := fs.Mkdir(filepath.Join(link, "New")); err == nil {
		t.Error("Mkdir created a folder through a link")
	}
	if err := fs.Move(filepath.Join(link, "movie.mkv"), filepath.Join(root, "x.mkv")); err == nil {
		t.Error("Move took a file through a folder link")
	}
	if err := fs.Move(fileLink, filepath.Join(root, "moved.mkv")); err == nil {
		t.Error("Move moved a symbolic link")
	}
	if _, err := os.Lstat(fileLink); err != nil {
		t.Errorf("the link was touched: %v", err)
	}
	if _, err := os.Stat(filepath.Join(realDir, "movie.mkv")); err != nil {
		t.Errorf("the file behind the link was touched: %v", err)
	}
}

func TestCopyFileCopiesOnceAndOnlyRegularFiles(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	from := filepath.Join(dir, "from.bin")
	to := filepath.Join(dir, "to.bin")
	if err := os.WriteFile(from, []byte("payload"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := copyFile(from, to); err != nil {
		t.Fatalf("copyFile: %v", err)
	}
	if data, _ := os.ReadFile(to); string(data) != "payload" {
		t.Fatalf("copy = %q", data)
	}
	if err := copyFile(from, to); err == nil {
		t.Error("copyFile overwrote its destination")
	}
	if err := copyFile(dir, filepath.Join(dir, "dir-copy")); err == nil {
		t.Error("copyFile copied a folder")
	}
}

func TestSizeReadsOnlyRegularFilesOfTheLibrary(t *testing.T) {
	t.Parallel()

	fs, root := fixture(t)
	outside := t.TempDir()
	file := filepath.Join(root, "Movies", "a.mkv")
	if err := os.MkdirAll(filepath.Dir(file), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file, []byte("12345"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(outside, "secret.mkv"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	linked := filepath.Join(root, "Movies", "link.mkv")
	if err := os.Symlink(filepath.Join(outside, "secret.mkv"), linked); err != nil {
		t.Skipf("symbolic links are not available here: %v", err)
	}
	linkedFolder := filepath.Join(root, "Linked")
	if err := os.Symlink(outside, linkedFolder); err != nil {
		t.Skipf("symbolic links are not available here: %v", err)
	}

	tests := []struct {
		name    string
		path    string
		want    int64
		wantErr bool
	}{
		{name: "a regular file of the library", path: file, want: 5},
		{name: "a folder", path: filepath.Dir(file), wantErr: true},
		{name: "a symbolic link to a file", path: linked, wantErr: true},
		{name: "a file below a symbolic link", path: filepath.Join(linkedFolder, "secret.mkv"), wantErr: true},
		{name: "a file that does not exist", path: filepath.Join(root, "Movies", "none.mkv"), wantErr: true},
		{name: "a file outside the library", path: filepath.Join(outside, "secret.mkv"), wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := fs.Size(tt.path)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("Size = %d; want an error", got)
				}
				return
			}
			if err != nil || got != tt.want {
				t.Fatalf("Size = %d, %v; want %d", got, err, tt.want)
			}
		})
	}
}

func TestLibraryPathIsInsideTheRoot(t *testing.T) {
	t.Parallel()

	fs, root := fixture(t)
	if got, want := fs.LibraryPath("Movies", "a.mkv"), filepath.Join(root, "Movies", "a.mkv"); got != want {
		t.Errorf("LibraryPath = %q, want %q", got, want)
	}
}
