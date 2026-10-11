package application_test

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"testing"

	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/catalog/application"
	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/catalog/domain"
	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/catalog/infrastructure/libraryfs"
)

// memStore keeps the catalog records in memory. It is the Records a commit writes, and the
// repositories the use cases read. failRecords makes the next record fail.
type memStore struct {
	mu          sync.Mutex
	trash       []domain.TrashEntry
	entries     []domain.UnassignedEntry
	failRecords bool
}

func (m *memStore) Entries(context.Context) ([]domain.TrashEntry, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]domain.TrashEntry(nil), m.trash...), nil
}

func (m *memStore) InsertTrash(_ context.Context, e domain.TrashEntry) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.failRecords {
		m.failRecords = false
		return 0, errors.New("database is closed")
	}
	e.ID = int64(len(m.trash) + 1)
	m.trash = append(m.trash, e)
	return e.ID, nil
}

func (m *memStore) InsertUnassigned(_ context.Context, e domain.UnassignedEntry) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.failRecords {
		m.failRecords = false
		return 0, errors.New("database is closed")
	}
	e.ID = int64(len(m.entries) + 1)
	m.entries = append(m.entries, cloneEntry(e))
	return e.ID, nil
}

func (m *memStore) AddUnassignedFiles(_ context.Context, id int64, files []domain.UnassignedFile) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.entry(id).Files = append(m.entry(id).Files, files...)
	return nil
}

func (m *memStore) BySource(_ context.Context, source string) (domain.UnassignedEntry, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, e := range m.entries {
		if e.Source == source {
			return cloneEntry(e), true, nil
		}
	}
	return domain.UnassignedEntry{}, false, nil
}

func (m *memStore) Names(context.Context) ([]string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var names []string
	for _, e := range m.entries {
		names = append(names, e.Name)
	}
	return names, nil
}

// entry returns the stored entry with the id; the caller holds the lock.
func (m *memStore) entry(id int64) *domain.UnassignedEntry {
	return &m.entries[id-1]
}

// count returns how many entries of No asignados are recorded.
func (m *memStore) count() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.entries)
}

func cloneEntry(e domain.UnassignedEntry) domain.UnassignedEntry {
	e.Files = append([]domain.UnassignedFile(nil), e.Files...)
	return e
}

// memJournal keeps the operations in memory; its commits write to the store.
type memJournal struct {
	mu    sync.Mutex
	ops   []domain.Operation
	store *memStore
	// failCommit makes the next commit fail before it records anything.
	failCommit bool
}

func (j *memJournal) Save(_ context.Context, op domain.Operation) error {
	j.mu.Lock()
	defer j.mu.Unlock()
	j.ops = append(j.ops, op)
	return nil
}

func (j *memJournal) Advance(_ context.Context, id string, started int) error {
	j.mu.Lock()
	defer j.mu.Unlock()
	for i := range j.ops {
		if j.ops[i].ID == id {
			j.ops[i].Started = started
		}
	}
	return nil
}

func (j *memJournal) Pending(context.Context) ([]domain.Operation, error) {
	j.mu.Lock()
	defer j.mu.Unlock()
	return append([]domain.Operation(nil), j.ops...), nil
}

func (j *memJournal) Forget(_ context.Context, id string) error {
	j.mu.Lock()
	defer j.mu.Unlock()
	j.ops = slices.DeleteFunc(j.ops, func(op domain.Operation) bool { return op.ID == id })
	return nil
}

func (j *memJournal) Commit(ctx context.Context, id string, record func(application.Records) error) error {
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.failCommit {
		j.failCommit = false
		return errors.New("database is busy")
	}
	if err := record(j.store); err != nil {
		return err
	}
	j.ops = slices.DeleteFunc(j.ops, func(op domain.Operation) bool { return op.ID == id })
	return nil
}

// flakyFiles is the library's file system, where the rename of the file named failOn fails
// the first time it is asked for.
type flakyFiles struct {
	*libraryfs.FS
	failOn string
	moves  int
}

func (f *flakyFiles) Move(from, to string) error {
	f.moves++
	if filepath.Base(from) == f.failOn {
		f.failOn = ""
		return errors.New("input/output error")
	}
	return f.FS.Move(from, to)
}

// errDead is what every call returns once the simulated process is gone.
var errDead = errors.New("the process stopped")

// dying simulates a process that stops: after allow more changes (a journal write or a file
// change), it is gone, and every later call fails. Nothing that runs after the stop reaches
// the disk or the journal. A negative allow never stops.
type dying struct {
	mu    sync.Mutex
	allow int
	dead  bool
}

// change counts one change: it fails when the process is gone or stops now.
func (d *dying) change() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.dead {
		return errDead
	}
	if d.allow == 0 {
		d.dead = true
		return errDead
	}
	if d.allow > 0 {
		d.allow--
	}
	return nil
}

// read fails when the process is gone.
func (d *dying) read() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.dead {
		return errDead
	}
	return nil
}

// dyingFiles is the library's files, as a process that may stop sees them.
type dyingFiles struct {
	application.Files
	d *dying
}

func (f dyingFiles) Mkdir(dir string) error {
	if err := f.d.change(); err != nil {
		return err
	}
	return f.Files.Mkdir(dir)
}

func (f dyingFiles) RemoveDir(dir string) error {
	if err := f.d.change(); err != nil {
		return err
	}
	return f.Files.RemoveDir(dir)
}

func (f dyingFiles) Move(from, to string) error {
	if err := f.d.change(); err != nil {
		return err
	}
	return f.Files.Move(from, to)
}

func (f dyingFiles) RemoveCopy(p string) error {
	if err := f.d.change(); err != nil {
		return err
	}
	return f.Files.RemoveCopy(p)
}

func (f dyingFiles) RemoveAll(p string) error {
	if err := f.d.change(); err != nil {
		return err
	}
	return f.Files.RemoveAll(p)
}

func (f dyingFiles) Exists(p string) (bool, error) {
	if err := f.d.read(); err != nil {
		return false, err
	}
	return f.Files.Exists(p)
}

func (f dyingFiles) Names(dir string) ([]string, error) {
	if err := f.d.read(); err != nil {
		return nil, err
	}
	return f.Files.Names(dir)
}

// dyingJournal is the journal, as a process that may stop sees it.
type dyingJournal struct {
	application.Journal
	d *dying
}

func (j dyingJournal) Save(ctx context.Context, op domain.Operation) error {
	if err := j.d.change(); err != nil {
		return err
	}
	return j.Journal.Save(ctx, op)
}

func (j dyingJournal) Advance(ctx context.Context, id string, started int) error {
	if err := j.d.change(); err != nil {
		return err
	}
	return j.Journal.Advance(ctx, id, started)
}

func (j dyingJournal) Forget(ctx context.Context, id string) error {
	if err := j.d.change(); err != nil {
		return err
	}
	return j.Journal.Forget(ctx, id)
}

func (j dyingJournal) Commit(ctx context.Context, id string, record func(application.Records) error) error {
	if err := j.d.change(); err != nil {
		return err
	}
	return j.Journal.Commit(ctx, id, record)
}

func (j dyingJournal) Pending(ctx context.Context) ([]domain.Operation, error) {
	if err := j.d.read(); err != nil {
		return nil, err
	}
	return j.Journal.Pending(ctx)
}

// libraryFiles lists the regular files under root, relative to it, with "/" separators.
func libraryFiles(t *testing.T, root string) []string {
	t.Helper()
	var out []string
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.Type().IsRegular() {
			rel, _ := filepath.Rel(root, p)
			out = append(out, filepath.ToSlash(rel))
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", root, err)
	}
	slices.Sort(out)
	return out
}

// writeFile writes a file, creating its folders, and returns its path.
func writeFile(t *testing.T, path, content string) string {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

// readFile returns the content of a file, failing the test when it cannot be read.
func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(data)
}
