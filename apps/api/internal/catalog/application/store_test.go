package application_test

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

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
	titles      []domain.LibraryTitle
	files       []domain.LibraryFile
	removed     map[int64]bool
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
	for i, e := range m.entries {
		if e.Source == source && !m.removed[int64(i+1)] {
			return cloneEntry(e), true, nil
		}
	}
	return domain.UnassignedEntry{}, false, nil
}

func (m *memStore) Names(context.Context) ([]string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var names []string
	for i, e := range m.entries {
		if !m.removed[int64(i+1)] {
			names = append(names, e.Name)
		}
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
	return len(m.entries) - len(m.removed)
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

// InsertLibraryTitle implements Records: the title gets the next id.
func (m *memStore) InsertLibraryTitle(_ context.Context, t domain.LibraryTitle) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	t.ID = int64(len(m.titles) + 1)
	m.titles = append(m.titles, t)
	return t.ID, nil
}

// InsertLibraryFiles implements Records.
func (m *memStore) InsertLibraryFiles(_ context.Context, titleID int64, files []domain.LibraryFile) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, f := range files {
		f.ID = int64(len(m.files) + 1)
		f.TitleID = titleID
		m.files = append(m.files, f)
	}
	return nil
}

// MoveLibraryFile implements Records.
func (m *memStore) MoveLibraryFile(_ context.Context, id int64, rel, version string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i := range m.files {
		if m.files[i].ID == id {
			m.files[i].Rel = rel
			m.files[i].Version = version
		}
	}
	return nil
}

// ByID implements UnassignedRepository.
func (m *memStore) ByID(_ context.Context, id int64) (domain.UnassignedEntry, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if e := m.live(id); e != nil {
		return cloneEntry(*e), true, nil
	}
	return domain.UnassignedEntry{}, false, nil
}

// Lock implements UnassignedRepository: the lock is taken when it is free, expired or held by token.
func (m *memStore) Lock(_ context.Context, id int64, token string, now, until time.Time) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	e := m.live(id)
	if e == nil {
		return false, nil
	}
	if e.Lock.Token != "" && e.Lock.Held(now) && e.Lock.Token != token {
		return false, nil
	}
	e.Lock = domain.AssignLock{Token: token, Until: until}
	return true, nil
}

// Unlock implements UnassignedRepository: only the token that holds the lock frees it.
func (m *memStore) Unlock(_ context.Context, id int64, token string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if e := m.live(id); e != nil && e.Lock.Token == token {
		e.Lock = domain.AssignLock{}
	}
	return nil
}

// TitleByFolder implements LibraryRepository.
func (m *memStore) TitleByFolder(_ context.Context, folder string) (domain.LibraryTitle, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, t := range m.titles {
		if strings.EqualFold(t.Folder, folder) {
			return t, true, nil
		}
	}
	return domain.LibraryTitle{}, false, nil
}

// Files implements LibraryRepository: every file of the title, trashed ones included.
func (m *memStore) Files(_ context.Context, titleID int64) ([]domain.LibraryFile, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []domain.LibraryFile
	for _, f := range m.files {
		if f.TitleID == titleID {
			out = append(out, f)
		}
	}
	return out, nil
}

// Titles implements LibraryRepository.
func (m *memStore) Titles(context.Context) ([]domain.LibraryTitle, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]domain.LibraryTitle(nil), m.titles...), nil
}

// RemoveUnassignedFiles implements Records: the files that left the entry are forgotten.
func (m *memStore) RemoveUnassignedFiles(_ context.Context, entryID int64, sources []string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	e := m.entry(entryID)
	var kept []domain.UnassignedFile
	for _, f := range e.Files {
		if !slices.Contains(sources, f.Source) {
			kept = append(kept, f)
		}
	}
	e.Files = kept
	return nil
}

// DeleteUnassignedEntry implements Records. The slot of the entry stays, so the ids keep their index.
func (m *memStore) DeleteUnassignedEntry(_ context.Context, entryID int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.removed == nil {
		m.removed = map[int64]bool{}
	}
	m.removed[entryID] = true
	return nil
}

// ReleaseAssignLock implements Records.
func (m *memStore) ReleaseAssignLock(_ context.Context, entryID int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.entry(entryID).Lock = domain.AssignLock{}
	return nil
}

// live returns the stored entry with the id, or nil when there is none or it was deleted. The
// caller holds the lock.
func (m *memStore) live(id int64) *domain.UnassignedEntry {
	if id < 1 || id > int64(len(m.entries)) || m.removed[id] {
		return nil
	}
	return &m.entries[id-1]
}

// TrashLibraryFiles implements Records.
func (m *memStore) TrashLibraryFiles(_ context.Context, ids []int64, entryID int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i := range m.files {
		for _, id := range ids {
			if m.files[i].ID == id {
				m.files[i].TrashEntryID = entryID
			}
		}
	}
	return nil
}

// List implements the section's repository: the entries that were not deleted, oldest first.
func (m *memStore) List(context.Context) ([]domain.UnassignedEntry, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []domain.UnassignedEntry
	for i, e := range m.entries {
		if !m.removed[int64(i+1)] {
			out = append(out, cloneEntry(e))
		}
	}
	return out, nil
}

// Apply implements the section's repository: fn writes its records straight into the store.
func (m *memStore) Apply(_ context.Context, fn func(application.Records) error) error {
	return fn(m)
}

// UpdateUnassignedFile implements Records: the place, size and reading of a file of the entry.
func (m *memStore) UpdateUnassignedFile(_ context.Context, entryID int64, f domain.UnassignedFile) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	e := m.entry(entryID)
	for i := range e.Files {
		if e.Files[i].Source == f.Source {
			e.Files[i] = f
			return nil
		}
	}
	return errors.New("no such file in the entry")
}
