package libraryfs

import (
	"os"
	"path/filepath"
	"testing"
)

// fixture returns a library root with its trash folder inside it.
func fixture(t *testing.T) (*FS, string) {
	t.Helper()
	root := t.TempDir()
	return New(root, filepath.Join(root, ".videoexplorer", "trash")), root
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
	if err := fs.Move(from, to); err == nil {
		t.Fatal("Move replaced an existing file")
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
	if err := fs.MkdirAll(filepath.Join(filepath.Dir(root), "elsewhere")); err == nil {
		t.Error("MkdirAll created a folder outside the library")
	}
}

func TestMkdirAllCreatesTheTrashFolders(t *testing.T) {
	t.Parallel()

	fs, _ := fixture(t)
	dir := fs.TrashPath("a1b2")
	if err := fs.MkdirAll(dir); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if info, err := os.Stat(dir); err != nil || !info.IsDir() {
		t.Fatalf("folder not created: %v", err)
	}
	if err := fs.MkdirAll(dir); err != nil {
		t.Fatalf("MkdirAll of an existing folder: %v", err)
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
