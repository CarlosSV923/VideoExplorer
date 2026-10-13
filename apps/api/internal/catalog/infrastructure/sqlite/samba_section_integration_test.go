package sqlite_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/catalog/application"
	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/catalog/domain"
)

// packEntryID returns the id of the entry Pack of the section, which must be a Samba copy.
func packEntryID(t *testing.T, e *sectionEnv) int64 {
	t.Helper()
	for _, entry := range e.list() {
		if entry.Name == "Pack" {
			if entry.Origin != domain.OriginSamba {
				t.Fatalf("entry Pack has origin %s, want samba", entry.Origin)
			}
			return entry.ID
		}
	}
	t.Fatal("no entry Pack in the section")
	return 0
}

func TestTakeSambaMovesTheCopyToStagingAndEmptiesItsFolder(t *testing.T) {
	t.Parallel()

	e := newSectionEnv(t)
	e.settled("Pack/movie.zip", "archive bytes")
	id := packEntryID(t, e)
	dst := filepath.Join(e.root, ".videoexplorer", "staging", "taken-job")

	files, err := e.section.SambaFiles(t.Context())
	if err != nil || len(files) != 1 || files[0].Rel != "movie.zip" || files[0].Name != "movie.zip" || files[0].Copying {
		t.Fatalf("SambaFiles = %+v, %v; want the settled archive of Pack", files, err)
	}
	if err := e.section.TakeSamba(t.Context(), id, []application.SambaMove{{Rel: "movie.zip", Dst: dst}}); err != nil {
		t.Fatalf("TakeSamba: %v", err)
	}
	if got, err := os.ReadFile(dst); err != nil || string(got) != "archive bytes" {
		t.Fatalf("staging holds %q, %v; want the archive", got, err)
	}
	if e.exists("Pack") {
		t.Fatal("the folder left empty by the take is still in the section")
	}
	for _, entry := range e.list() {
		if entry.ID == id {
			t.Fatalf("the entry of the taken copy is still listed: %+v", entry)
		}
	}
}

func TestTakeSambaKeepsTheFolderThatHoldsAnotherFile(t *testing.T) {
	t.Parallel()

	e := newSectionEnv(t)
	e.settled("Pack/movie.zip", "archive bytes")
	e.settled("Pack/readme.txt", "notes")
	id := packEntryID(t, e)
	dst := filepath.Join(e.root, ".videoexplorer", "staging", "taken-job-2")

	if err := e.section.TakeSamba(t.Context(), id, []application.SambaMove{{Rel: "movie.zip", Dst: dst}}); err != nil {
		t.Fatalf("TakeSamba: %v", err)
	}
	if !e.exists("Pack/readme.txt") {
		t.Fatal("the file that stayed was removed with the copy")
	}
}

func TestTakeSambaRefusesACopyStillBeingCopied(t *testing.T) {
	t.Parallel()

	e := newSectionEnv(t)
	e.settled("Pack/movie.zip", "archive bytes")
	id := packEntryID(t, e)
	e.put("Pack/movie.zip", "more bytes", e.now)                         // modified just now: copying again
	dst := filepath.Join(e.root, ".videoexplorer", "staging", "taken-3") // never written

	err := e.section.TakeSamba(t.Context(), id, []application.SambaMove{{Rel: "movie.zip", Dst: dst}})
	if !errors.Is(err, application.ErrCopying) {
		t.Fatalf("TakeSamba of a copy in progress = %v, want ErrCopying", err)
	}
	if !e.exists("Pack/movie.zip") {
		t.Fatal("the copy in progress was moved")
	}
}

func TestTakeSambaRefusesAnUploadEntry(t *testing.T) {
	t.Parallel()

	e := newSectionEnv(t)
	staged := e.stagedUpload("upload-taken", "video")
	if _, err := e.unassign.Put(t.Context(), application.AsideRequest{
		Source: "upload-x", Name: "Uploaded", BatchID: "b-x", Relation: "none",
		Files: []application.AsideFile{{Source: staged, Rel: "Uploaded.mkv", Size: 5}},
	}); err != nil {
		t.Fatalf("Put: %v", err)
	}
	var uploadID int64
	for _, entry := range e.list() {
		if entry.Name == "Uploaded" {
			uploadID = entry.ID
		}
	}
	err := e.section.TakeSamba(t.Context(), uploadID, []application.SambaMove{{Rel: "Uploaded.mkv", Dst: filepath.Join(e.root, "x")}})
	if !errors.Is(err, application.ErrNotSamba) {
		t.Fatalf("TakeSamba of an upload = %v, want ErrNotSamba", err)
	}
}

func TestTakeSambaRefusesAFileThatIsNotInTheEntry(t *testing.T) {
	t.Parallel()

	e := newSectionEnv(t)
	e.settled("Pack/movie.zip", "archive bytes")
	id := packEntryID(t, e)

	err := e.section.TakeSamba(t.Context(), id, []application.SambaMove{{Rel: "gone.zip", Dst: filepath.Join(e.root, "x")}})
	if !errors.Is(err, application.ErrFileNotFound) {
		t.Fatalf("TakeSamba of a file that is not there = %v, want ErrFileNotFound", err)
	}
	if !e.exists("Pack/movie.zip") {
		t.Fatal("a refused take moved the copy")
	}
}
