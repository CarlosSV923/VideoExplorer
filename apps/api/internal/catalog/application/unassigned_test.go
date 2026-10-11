package application_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/catalog/application"
	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/catalog/domain"
	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/catalog/infrastructure/libraryfs"
)

// section is a library with a staging area and an unassigned section.
type section struct {
	root    string
	staging string
	store   *memStore
	journal *memJournal
	files   *flakyFiles
	put     *application.Unassigned
}

func newSection(t *testing.T) *section {
	t.Helper()
	root := t.TempDir()
	s := &section{root: root, staging: filepath.Join(root, ".videoexplorer", "staging"), store: &memStore{}}
	s.journal = &memJournal{store: s.store}
	if err := os.MkdirAll(s.staging, 0o750); err != nil {
		t.Fatal(err)
	}
	s.files = &flakyFiles{FS: libraryfs.New(root)}
	s.put = application.NewUnassigned(s.store, s.files, engine(s.files, s.journal), nil)
	return s
}

// stage writes a file in staging, as an upload or an extraction leaves it.
func (s *section) stage(t *testing.T, name, content string) string {
	t.Helper()
	return writeFile(t, filepath.Join(s.staging, name), content)
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
	if s.store.count() != 1 {
		t.Fatalf("entries = %d, want 1", s.store.count())
	}
	e := s.store.entries[0]
	if e.Name != "Mission - Impossible" || e.Loose || e.BatchID != "b1" || e.Relation != domain.RelationParts || e.Origin != domain.OriginUpload {
		t.Errorf("entry = %+v; want a folder named after the sanitized name, with its batch, relation and origin", e)
	}
	if len(e.Files) != 2 {
		t.Errorf("files = %d, want both recorded once they are in place", len(e.Files))
	}
	if pending, _ := s.journal.Pending(t.Context()); len(pending) != 0 {
		t.Errorf("journal keeps %d operations after the commit", len(pending))
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
			s.store.entries = append(s.store.entries, domain.UnassignedEntry{ID: 1, Source: "older", Name: "Movie"})
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
			if got := s.store.entries[len(s.store.entries)-1].Name; got != tt.want {
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
			writeFile(t, s.unassigned("Movie.mkv"), "other")
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
			if e := s.store.entries[0]; !e.Loose || e.Name != tt.want {
				t.Errorf("entry = %+v; want a loose entry named %q", e, tt.want)
			}
			if tt.setup != nil {
				if got := readFile(t, s.unassigned("Movie.mkv")); got != "other" {
					t.Errorf("the file already there was overwritten: %q", got)
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

// TestAFailedMoveIsUndoneAndTheRetryMovesEverything: the library holds nothing of a failed
// call, so the retry plans the whole entry again, and nothing is moved twice.
func TestAFailedMoveIsUndoneAndTheRetryMovesEverything(t *testing.T) {
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
	if readFile(t, first) != "one" {
		t.Fatalf("the first file did not go back to staging")
	}
	if _, err := os.Stat(s.unassigned()); !os.IsNotExist(err) {
		t.Errorf("the section folder is left behind after the undo: %v", err)
	}
	if s.store.count() != 0 {
		t.Fatalf("entries = %d after a failed call, want none", s.store.count())
	}

	places, err := s.put.Put(t.Context(), req)
	if err != nil {
		t.Fatalf("second Put: %v", err)
	}
	if s.store.count() != 1 || s.store.entries[0].Name != "Series" {
		t.Errorf("entries = %+v; want one entry named Series", s.store.entries)
	}
	if got, want := places[second], s.unassigned("Series", "b.mkv"); got != want {
		t.Errorf("second place = %q, want %q", got, want)
	}
	if s.files.moves != 5 {
		t.Errorf("renames = %d, want 5 (the failed call's two and the undo of the first, then the retry's two)", s.files.moves)
	}
	if _, err := os.Stat(s.unassigned("Series", "a (2).mkv")); !os.IsNotExist(err) {
		t.Errorf("the first file was moved twice: %v", err)
	}
}

// TestAFailedRecordUndoesTheMovesOfTheEntry: the moves happen, the commit that records the
// entry fails, and the files go back: nothing is left in No asignados without its record.
func TestAFailedRecordUndoesTheMovesOfTheEntry(t *testing.T) {
	t.Parallel()

	s := newSection(t)
	video := s.stage(t, "j1/Movie.mkv", "video")
	req := application.AsideRequest{
		Source: "upload-1", Name: "Movie.mkv", Loose: true, BatchID: "b1", Relation: domain.RelationNone,
		Files: []application.AsideFile{{Source: video, Size: 5}},
	}
	s.store.failRecords = true
	if _, err := s.put.Put(t.Context(), req); err == nil {
		t.Fatal("Put succeeded although the record failed")
	}
	if readFile(t, video) != "video" {
		t.Fatalf("the file did not go back to staging")
	}
	if _, err := os.Stat(s.unassigned("Movie.mkv")); !os.IsNotExist(err) {
		t.Fatalf("the file is still in the section: %v", err)
	}

	if _, err := s.put.Put(t.Context(), req); err != nil {
		t.Fatalf("Put after the failure: %v", err)
	}
	if s.store.count() != 1 || s.store.entries[0].Name != "Movie.mkv" {
		t.Errorf("entries = %+v; want one entry named Movie.mkv", s.store.entries)
	}
	if s.files.moves != 3 {
		t.Errorf("renames = %d, want 3 (the move and its undo in the failed call, then the retry's move)", s.files.moves)
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
	if s.store.count() != 1 || s.store.entries[0].Name != "Movie" {
		t.Errorf("entries = %+v; want the same entry, named as it was first", s.store.entries)
	}
	if got, want := places[second], s.unassigned("Movie", "Movie.srt"); got != want {
		t.Errorf("added file place = %q, want %q", got, want)
	}
	if places[first] != s.unassigned("Movie", "Movie.mkv") {
		t.Errorf("the file already in the entry moved place: %q", places[first])
	}
	if len(s.store.entries[0].Files) != 2 {
		t.Errorf("files = %d, want 2 (the first one is not recorded twice)", len(s.store.entries[0].Files))
	}
	if s.files.moves != 2 {
		t.Errorf("renames = %d, want 2 (the file already in the entry is not moved again)", s.files.moves)
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
		{name: "a file that is not in staging", req: func(s *section) application.AsideRequest {
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
			if s.store.count() != 0 {
				t.Errorf("entries = %d, want none recorded", s.store.count())
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
