package application_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/catalog/application"
	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/catalog/domain"
	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/catalog/infrastructure/libraryfs"
)

// memUnassigned keeps the entries of the section in memory. failMarks makes the next
// MarkMoved calls fail, to simulate a restart between a rename and its record.
type memUnassigned struct {
	mu        sync.Mutex
	entries   []domain.UnassignedEntry
	failMarks int
}

func (m *memUnassigned) BySource(_ context.Context, source string) (domain.UnassignedEntry, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, e := range m.entries {
		if e.Source == source {
			return cloneEntry(e), true, nil
		}
	}
	return domain.UnassignedEntry{}, false, nil
}

func (m *memUnassigned) Names(context.Context) ([]string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var names []string
	for _, e := range m.entries {
		names = append(names, e.Name)
	}
	return names, nil
}

func (m *memUnassigned) Record(_ context.Context, e domain.UnassignedEntry) (domain.UnassignedEntry, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	e.ID = int64(len(m.entries) + 1)
	m.entries = append(m.entries, cloneEntry(e))
	return cloneEntry(e), nil
}

func (m *memUnassigned) AddFiles(_ context.Context, id int64, files []domain.UnassignedFile) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.entry(id).Files = append(m.entry(id).Files, files...)
	return nil
}

func (m *memUnassigned) MarkMoved(_ context.Context, id int64, source string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.failMarks > 0 {
		m.failMarks--
		return errors.New("database is busy")
	}
	files := m.entry(id).Files
	for i := range files {
		if files[i].Source == source {
			files[i].Moved = true
		}
	}
	return nil
}

// entry returns the stored entry with the id, which the caller must hold the lock for.
func (m *memUnassigned) entry(id int64) *domain.UnassignedEntry {
	return &m.entries[id-1]
}

func (m *memUnassigned) count() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.entries)
}

func cloneEntry(e domain.UnassignedEntry) domain.UnassignedEntry {
	e.Files = append([]domain.UnassignedFile(nil), e.Files...)
	return e
}

// flakyFiles is the library's file system, where the rename of the file named failOn
// fails the first time it is asked for.
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

// section is a library with a staging area and an unassigned section.
type section struct {
	root    string
	staging string
	repo    *memUnassigned
	files   *flakyFiles
	put     *application.Unassigned
}

func newSection(t *testing.T) *section {
	t.Helper()
	root := t.TempDir()
	s := &section{root: root, staging: filepath.Join(root, ".videoexplorer", "staging"), repo: &memUnassigned{}}
	if err := os.MkdirAll(s.staging, 0o750); err != nil {
		t.Fatal(err)
	}
	s.files = &flakyFiles{FS: libraryfs.New(root, filepath.Join(root, ".videoexplorer", "trash"))}
	now := time.Date(2026, time.October, 11, 9, 0, 0, 0, time.UTC)
	s.put = application.NewUnassigned(s.repo, s.files, func() time.Time { return now })
	return s
}

// stage writes a file in staging, as an upload or an extraction leaves it.
func (s *section) stage(t *testing.T, name, content string) string {
	t.Helper()
	p := filepath.Join(s.staging, name)
	if err := os.MkdirAll(filepath.Dir(p), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

// folder makes a folder inside the section, as an earlier entry or a Samba copy leaves it.
func (s *section) folder(t *testing.T, name string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(s.root, domain.UnassignedDir, name), 0o750); err != nil {
		t.Fatal(err)
	}
}

func (s *section) unassigned(parts ...string) string {
	return filepath.Join(append([]string{s.root, domain.UnassignedDir}, parts...)...)
}

func folderRequest(name string, files ...application.AsideFile) application.AsideRequest {
	return application.AsideRequest{Source: "batch-1", Name: name, BatchID: "b1", Relation: domain.RelationParts, Files: files}
}

func TestPutMovesAFolderEntryIntoUnassigned(t *testing.T) {
	t.Parallel()

	s := newSection(t)
	video := s.stage(t, "j1/Movie.mkv", "video")
	subtitle := s.stage(t, "j1/sub/Movie.es.srt", "subtitle")

	places, err := s.put.Put(t.Context(), folderRequest("Mission: Impossible",
		application.AsideFile{Source: video, Rel: "Mission/Movie.mkv", Size: 5},
		application.AsideFile{Source: subtitle, Rel: "Mission/sub/Movie.es.srt", Size: 8},
	))
	if err != nil {
		t.Fatalf("Put: %v", err)
	}

	wantVideo := s.unassigned("Mission - Impossible", "Mission", "Movie.mkv")
	if places[video] != wantVideo {
		t.Errorf("place of the video = %q, want %q", places[video], wantVideo)
	}
	for _, p := range []string{wantVideo, s.unassigned("Mission - Impossible", "Mission", "sub", "Movie.es.srt")} {
		if _, err := os.Stat(p); err != nil {
			t.Errorf("%s is not in place: %v", p, err)
		}
	}
	if _, err := os.Stat(video); !os.IsNotExist(err) {
		t.Errorf("the video is still in staging: %v", err)
	}
	if s.repo.count() != 1 {
		t.Fatalf("entries = %d, want 1", s.repo.count())
	}
	e := s.repo.entries[0]
	if e.Name != "Mission - Impossible" || e.Loose || e.BatchID != "b1" || e.Relation != domain.RelationParts || e.Origin != domain.OriginUpload {
		t.Errorf("entry = %+v; want a folder named after the sanitized name, with its batch, relation and origin", e)
	}
	if !e.Complete() {
		t.Errorf("entry has files not moved: %+v", e.Files)
	}
}

func TestPutNamesTheEntryAndSuffixesItWhenTaken(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		raw   string
		setup func(*testing.T, *section)
		want  string
	}{
		{name: "a name is sanitized", raw: "Mission: Impossible", want: "Mission - Impossible"},
		{name: "an empty name gets the fallback", raw: "?*", want: domain.FallbackName},
		{name: "a folder with the name takes (2)", raw: "Movie", setup: func(t *testing.T, s *section) { s.folder(t, "Movie") }, want: "Movie (2)"},
		{name: "a folder taken in another letter case takes (2)", raw: "Movie", setup: func(t *testing.T, s *section) { s.folder(t, "MOVIE") }, want: "Movie (2)"},
		{name: "the next free suffix is used", raw: "Movie", setup: func(t *testing.T, s *section) {
			s.folder(t, "Movie")
			s.folder(t, "Movie (2)")
		}, want: "Movie (3)"},
		{name: "a name already recorded takes (2) even when its folder is gone", raw: "Movie", setup: func(t *testing.T, s *section) {
			s.repo.entries = append(s.repo.entries, domain.UnassignedEntry{ID: 1, Source: "older", Name: "Movie"})
		}, want: "Movie (2)"},
		{name: "a name longer than 255 bytes is cut and still fits", raw: strings.Repeat("é", 200) + "!", want: strings.Repeat("é", 127)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			s := newSection(t)
			if tt.setup != nil {
				tt.setup(t, s)
			}
			video := s.stage(t, "j1/a.mkv", "video")
			if _, err := s.put.Put(t.Context(), folderRequest(tt.raw, application.AsideFile{Source: video, Rel: "a.mkv"})); err != nil {
				t.Fatalf("Put: %v", err)
			}
			if got := s.repo.entries[len(s.repo.entries)-1].Name; got != tt.want {
				t.Errorf("entry name = %q, want %q", got, tt.want)
			}
			if _, err := os.Stat(s.unassigned(tt.want, "a.mkv")); err != nil {
				t.Errorf("the file is not in the entry folder: %v", err)
			}
		})
	}
}

func TestALooseFileKeepsItsExtensionAndItsPlace(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		raw   string
		setup func(*testing.T, *section)
		want  string
	}{
		{name: "a video keeps its name", raw: "Movie.mkv", want: "Movie.mkv"},
		{name: "the extension keeps its case and the stem is sanitized", raw: "Movie: Film.MKV", want: "Movie - Film.MKV"},
		{name: "a file already there takes (2) before the extension", raw: "Movie.mkv", setup: func(t *testing.T, s *section) {
			if err := os.MkdirAll(s.unassigned(), 0o750); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(s.unassigned("Movie.mkv"), []byte("other"), 0o600); err != nil {
				t.Fatal(err)
			}
		}, want: "Movie (2).mkv"},
		{name: "an empty stem gets the fallback before the extension", raw: "?.mkv", want: domain.FallbackName + ".mkv"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			s := newSection(t)
			if tt.setup != nil {
				tt.setup(t, s)
			}
			video := s.stage(t, "j1", "video")
			places, err := s.put.Put(t.Context(), application.AsideRequest{
				Source: "upload-1", Name: tt.raw, Loose: true, BatchID: "b1", Relation: domain.RelationNone,
				Files: []application.AsideFile{{Source: video, Size: 5}},
			})
			if err != nil {
				t.Fatalf("Put: %v", err)
			}
			want := s.unassigned(tt.want)
			if places[video] != want {
				t.Errorf("place = %q, want %q", places[video], want)
			}
			if _, err := os.Stat(want); err != nil {
				t.Errorf("the file is not at its place: %v", err)
			}
			if e := s.repo.entries[0]; !e.Loose || e.Name != tt.want {
				t.Errorf("entry = %+v; want a loose entry named %q", e, tt.want)
			}
			if tt.setup != nil {
				got, err := os.ReadFile(s.unassigned("Movie.mkv"))
				if err != nil || string(got) != "other" {
					t.Errorf("the file already there was overwritten: %q, %v", got, err)
				}
			}
		})
	}
}

func TestFilesThatCollideInTheEntryGetASuffix(t *testing.T) {
	t.Parallel()

	s := newSection(t)
	first := s.stage(t, "j1/a.mkv", "one")
	second := s.stage(t, "j2/a.mkv", "two")
	places, err := s.put.Put(t.Context(), folderRequest("Series",
		application.AsideFile{Source: first, Rel: "Season 01/a.mkv"},
		application.AsideFile{Source: second, Rel: "Season 01/a.mkv"},
	))
	if err != nil {
		t.Fatalf("Put: %v", err)
	}
	if got, want := places[first], s.unassigned("Series", "Season 01", "a.mkv"); got != want {
		t.Errorf("first place = %q, want %q", got, want)
	}
	if got, want := places[second], s.unassigned("Series", "Season 01", "a (2).mkv"); got != want {
		t.Errorf("second place = %q, want %q", got, want)
	}
}

func TestAFailedMoveGoesOnWithoutMovingAgain(t *testing.T) {
	t.Parallel()

	s := newSection(t)
	first := s.stage(t, "j1/a.mkv", "one")
	second := s.stage(t, "j1/b.mkv", "two")
	s.files.failOn = "b.mkv"
	req := folderRequest("Series",
		application.AsideFile{Source: first, Rel: "a.mkv"},
		application.AsideFile{Source: second, Rel: "b.mkv"},
	)

	if _, err := s.put.Put(t.Context(), req); err == nil {
		t.Fatal("Put succeeded with a failing rename")
	}
	if _, err := os.Stat(s.unassigned("Series", "a.mkv")); err != nil {
		t.Fatalf("the first file is not in place after the failure: %v", err)
	}

	places, err := s.put.Put(t.Context(), req)
	if err != nil {
		t.Fatalf("second Put: %v", err)
	}
	if s.repo.count() != 1 || s.repo.entries[0].Name != "Series" {
		t.Errorf("entries = %+v; want one entry named Series, nothing duplicated", s.repo.entries)
	}
	if got, want := places[second], s.unassigned("Series", "b.mkv"); got != want {
		t.Errorf("second place = %q, want %q", got, want)
	}
	if s.files.moves != 3 {
		t.Errorf("renames = %d, want 3 (the first file is not renamed again)", s.files.moves)
	}
	if _, err := os.Stat(s.unassigned("Series", "a (2).mkv")); !os.IsNotExist(err) {
		t.Errorf("the first file was moved twice: %v", err)
	}
}

func TestARestartFindsTheFilesAlreadyInPlace(t *testing.T) {
	t.Parallel()

	s := newSection(t)
	video := s.stage(t, "j1/Movie.mkv", "video")
	req := application.AsideRequest{
		Source: "upload-1", Name: "Movie.mkv", Loose: true, BatchID: "b1", Relation: domain.RelationNone,
		Files: []application.AsideFile{{Source: video, Size: 5}},
	}
	// The rename happens, but the record of the move is lost with the process.
	s.repo.failMarks = 1
	if _, err := s.put.Put(t.Context(), req); err == nil {
		t.Fatal("Put succeeded although the record of the move failed")
	}
	if _, err := os.Stat(s.unassigned("Movie.mkv")); err != nil {
		t.Fatalf("the file is not in place: %v", err)
	}

	if _, err := s.put.Put(t.Context(), req); err != nil {
		t.Fatalf("Put after the restart: %v", err)
	}
	if s.repo.count() != 1 || s.repo.entries[0].Name != "Movie.mkv" || !s.repo.entries[0].Complete() {
		t.Errorf("entries = %+v; want one complete entry named Movie.mkv", s.repo.entries)
	}
	if s.files.moves != 1 {
		t.Errorf("renames = %d, want 1", s.files.moves)
	}
}

func TestASecondPutAddsItsFilesToTheSameEntry(t *testing.T) {
	t.Parallel()

	s := newSection(t)
	first := s.stage(t, "j1/Movie.mkv", "one")
	if _, err := s.put.Put(t.Context(), folderRequest("Movie", application.AsideFile{Source: first, Rel: "Movie.mkv"})); err != nil {
		t.Fatalf("first Put: %v", err)
	}
	second := s.stage(t, "j2/Movie.srt", "two")
	places, err := s.put.Put(t.Context(), folderRequest("Other name",
		application.AsideFile{Source: first, Rel: "Movie.mkv"},
		application.AsideFile{Source: second, Rel: "Movie.srt"},
	))
	if err != nil {
		t.Fatalf("second Put: %v", err)
	}
	if s.repo.count() != 1 || s.repo.entries[0].Name != "Movie" {
		t.Errorf("entries = %+v; want the same entry, named as it was first", s.repo.entries)
	}
	if got, want := places[second], s.unassigned("Movie", "Movie.srt"); got != want {
		t.Errorf("added file place = %q, want %q", got, want)
	}
	if len(s.repo.entries[0].Files) != 2 {
		t.Errorf("files = %d, want 2 (the first one is not recorded twice)", len(s.repo.entries[0].Files))
	}
}

func TestPutRefusesWhatCannotBeMoved(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		req  func(s *section) application.AsideRequest
	}{
		{name: "no files", req: func(*section) application.AsideRequest {
			return folderRequest("Movie")
		}},
		{name: "a loose entry with two files", req: func(s *section) application.AsideRequest {
			return application.AsideRequest{
				Source: "u", Name: "a.mkv", Loose: true, BatchID: "b1", Relation: domain.RelationNone,
				Files: []application.AsideFile{{Source: s.stage(t, "x/a", "1")}, {Source: s.stage(t, "x/b", "2")}},
			}
		}},
		{name: "a file that is neither in staging nor in place", req: func(s *section) application.AsideRequest {
			return folderRequest("Movie", application.AsideFile{Source: filepath.Join(s.staging, "gone"), Rel: "a.mkv"})
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			s := newSection(t)
			if _, err := s.put.Put(t.Context(), tt.req(s)); err == nil {
				t.Fatal("Put accepted it")
			}
			if s.repo.count() != 0 {
				t.Errorf("entries = %d, want none recorded", s.repo.count())
			}
			if _, err := os.Stat(s.unassigned()); !os.IsNotExist(err) {
				t.Errorf("the section folder was created: %v", err)
			}
		})
	}
}

func TestInnerNamesAreSanitizedAndACollisionAfterSanitizingTakesTwo(t *testing.T) {
	t.Parallel()

	s := newSection(t)
	first := s.stage(t, "j1/a.mkv", "one")
	second := s.stage(t, "j2/a.mkv", "two")
	places, err := s.put.Put(t.Context(), folderRequest("Series",
		application.AsideFile{Source: first, Rel: "Season?/a:b.mkv"},
		application.AsideFile{Source: second, Rel: "Season/a-b.mkv"},
	))
	if err != nil {
		t.Fatalf("Put: %v", err)
	}
	if got, want := places[first], s.unassigned("Series", "Season", "a-b.mkv"); got != want {
		t.Errorf("sanitized place = %q, want %q", got, want)
	}
	if got, want := places[second], s.unassigned("Series", "Season", "a-b (2).mkv"); got != want {
		t.Errorf("collision after sanitizing = %q, want %q", got, want)
	}
}
