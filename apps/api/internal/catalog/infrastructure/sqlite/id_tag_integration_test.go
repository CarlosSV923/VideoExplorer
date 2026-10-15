package sqlite_test

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/catalog/application"
	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/catalog/domain"
)

// The ID tag action (RF-45) on a library in a temporary folder, with the real registry, journal and file system.
// A linked Iron Man (2008) (TMDB 1726) and a linked Breaking Bad (2008) (TMDB 1396) are the titles it works on.
const (
	taggedIron     = "Movies/Iron Man (2008) [tmdbid-1726]"
	taggedBreaking = "TV Shows/Breaking Bad (2008) [tmdbid-1396]"
	plainBreaking  = "TV Shows/Breaking Bad (2008)"
)

// tagPlan returns the preview of the ID tag action that puts the tag on (idTag) or takes it off (RF-45).
func (e *editEnv) tagPlan(titleID int64, req application.IDTagRequest) application.EditPlan {
	e.t.Helper()
	p, err := e.edits.PlanIDTag(e.ctx, titleID, req)
	if err != nil {
		e.t.Fatalf("PlanIDTag: %v", err)
	}
	return p
}

// setTag plans the ID tag action, checks that the plan is valid and commits it with the revision of that plan.
func (e *editEnv) setTag(titleID int64, idTag bool) application.EditResult {
	e.t.Helper()
	p := e.tagPlan(titleID, application.IDTagRequest{IDTag: idTag})
	if !p.Valid {
		e.t.Fatalf("the tag plan is not valid: %+v", p.Errors)
	}
	res, err := e.edits.CommitIDTag(e.ctx, titleID, application.IDTagRequest{Revision: p.Revision, IDTag: idTag})
	if err != nil {
		e.t.Fatalf("CommitIDTag: %v", err)
	}
	return res
}

// read returns the content of a library file.
func (e *editEnv) read(rel string) string {
	e.t.Helper()
	b, err := os.ReadFile(filepath.Join(e.root, filepath.FromSlash(rel))) //nolint:gosec // a file the test wrote
	if err != nil {
		e.t.Fatalf("read %s: %v", rel, err)
	}
	return string(b)
}

// inTrash reports whether a file with the content is in the trash folder of the library (RF-30).
func (e *editEnv) inTrash(content string) bool {
	e.t.Helper()
	found := false
	err := filepath.WalkDir(filepath.Join(e.root, ".videoexplorer", "trash"), func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		if b, readErr := os.ReadFile(p); readErr == nil && string(b) == content { //nolint:gosec // a file the test wrote
			found = true
		}
		return nil
	})
	if err != nil {
		e.t.Fatalf("walk the trash: %v", err)
	}
	return found
}

// hasEditIssue reports whether the issues hold one with the field and the code.
func hasEditIssue(issues []application.EditIssue, field, code string) bool {
	for _, is := range issues {
		if is.Field == field && is.Code == code {
			return true
		}
	}
	return false
}

// TestTakingTheTagOffAMovieRenamesItsFilesAndSubtitles takes the tag off a linked movie with a version and a
// subtitle: the folder and every file take the names of spec §5 without the tag (RF-45).
func TestTakingTheTagOffAMovieRenamesItsFilesAndSubtitles(t *testing.T) {
	t.Parallel()
	e := newEditEnv(t, nil)
	whole := e.write(taggedIron+"/Iron Man (2008) [tmdbid-1726].mkv", "whole")
	version := e.write(taggedIron+"/Iron Man (2008) [tmdbid-1726] - 1080p.mkv", "version")
	sub := e.write(taggedIron+"/Iron Man (2008) [tmdbid-1726].es.srt", "sub")
	titleID := e.seedMovie(taggedIron, 1726, true,
		e.video(domain.KindMovie, whole, domain.Place{}, ""),
		e.video(domain.KindMovie, version, domain.Place{}, "1080p"),
		mustSubtitle(t, domain.SubtitleSpec{Rel: sub, VideoRel: whole, Language: "es", Size: 3}))

	p := e.tagPlan(titleID, application.IDTagRequest{IDTag: false})
	if !p.Valid || p.Folder != ironFolder || p.MergeInto != 0 {
		t.Fatalf("plan: valid=%v folder=%q merge=%d errors=%+v", p.Valid, p.Folder, p.MergeInto, p.Errors)
	}
	res := e.setTag(titleID, false)
	if !res.Changed || res.Folder != ironFolder || res.TitleID != titleID {
		t.Errorf("result = %+v", res)
	}
	for _, rel := range []string{ironFolder + "/Iron Man (2008).mkv", ironFolder + "/Iron Man (2008) - 1080p.mkv", ironFolder + "/Iron Man (2008).es.srt"} {
		if !e.exists(rel) {
			t.Errorf("%s is at its name without the tag", rel)
		}
	}
	if e.exists(taggedIron) {
		t.Error("the tagged folder is gone")
	}
	title, ok := e.titleOf(ironFolder)
	if !ok || title.IDTag || title.TMDBID != 1726 {
		t.Errorf("the registry title = %+v, found %v: linked, without the tag", title, ok)
	}
	rows := e.rows(titleID)
	if got := rows[ironFolder+"/Iron Man (2008) - 1080p.mkv"]; got.ID == 0 || got.Version != "1080p" {
		t.Errorf("the version keeps its label in the registry: %+v", got)
	}
	e.assertNothingJournaled()
}

// TestPuttingTheTagOnAnImportedMovieRenamesItsFiles puts the tag on a movie that was imported without it: its folder
// and its files take the names of spec §5 with the tag (RF-29, RF-45).
func TestPuttingTheTagOnAnImportedMovieRenamesItsFiles(t *testing.T) {
	t.Parallel()
	e := newEditEnv(t, nil)
	video := e.write(ironFolder+"/Iron.Man.2008.1080p.mkv", "imported")
	sub := e.write(ironFolder+"/Iron.Man.2008.1080p.es.srt", "sub")
	titleID := e.seedMovie(ironFolder, 1726, false,
		e.video(domain.KindMovie, video, domain.Place{}, ""),
		mustSubtitle(t, domain.SubtitleSpec{Rel: sub, VideoRel: video, Language: "es", Size: 3}))

	res := e.setTag(titleID, true)
	if !res.Changed || res.Folder != taggedIron {
		t.Fatalf("result = %+v", res)
	}
	if !e.exists(taggedIron+"/Iron Man (2008) [tmdbid-1726].mkv") || !e.exists(taggedIron+"/Iron Man (2008) [tmdbid-1726].es.srt") {
		t.Error("the video and its subtitle take the names of spec §5 with the tag")
	}
	if e.exists(ironFolder) || e.exists(video) {
		t.Error("the imported names are gone")
	}
	title, ok := e.titleOf(taggedIron)
	if !ok || !title.IDTag || title.TMDBID != 1726 {
		t.Errorf("the registry title = %+v, found %v: linked, with the tag", title, ok)
	}
	if got := e.rows(titleID); len(got) != 2 || got[taggedIron+"/Iron Man (2008) [tmdbid-1726].mkv"].ID == 0 {
		t.Errorf("the registry files follow their new paths: %+v", got)
	}
	e.assertNothingJournaled()
}

// TestAFailureInTheMiddleUndoesTheTagChange makes the move of a subtitle fail after the video has moved: the tag change
// is undone as a whole, on disk and in the registry (RNF-04).
func TestAFailureInTheMiddleUndoesTheTagChange(t *testing.T) {
	t.Parallel()
	failed := false
	e := newEditEnv(t, func(f application.Files) application.Files { return failingFiles{Files: f, failed: &failed} })
	video := e.write(ironFolder+"/Iron Man (2008).mkv", "iron")
	sub := e.write(ironFolder+"/Iron Man (2008).es.srt", "sub")
	titleID := e.seedMovie(ironFolder, 1726, false,
		e.video(domain.KindMovie, video, domain.Place{}, ""),
		mustSubtitle(t, domain.SubtitleSpec{Rel: sub, VideoRel: video, Language: "es", Size: 3}))
	p := e.tagPlan(titleID, application.IDTagRequest{IDTag: true})
	if !p.Valid {
		t.Fatalf("plan: %+v", p.Errors)
	}
	if _, err := e.edits.CommitIDTag(e.ctx, titleID, application.IDTagRequest{Revision: p.Revision, IDTag: true}); err == nil {
		t.Fatal("the commit fails at the injected step")
	}
	if !e.exists(video) || !e.exists(sub) || e.exists(taggedIron) {
		t.Error("everything is back where it was")
	}
	if title, ok := e.titleOf(ironFolder); !ok || title.IDTag {
		t.Errorf("the registry title is unchanged: %+v, %v", title, ok)
	}
	if got := e.rows(titleID); got[video].ID == 0 || got[sub].Language != "es" {
		t.Errorf("the registry rows are unchanged: %+v", got)
	}
	e.assertNothingJournaled()
}

// TestTakingTheTagOffASeriesRenamesOnlyItsFolder takes the tag off a linked series: only the folder changes, the
// chapter and its subtitle keep their names, even though TMDB would name the chapter differently (RF-45).
func TestTakingTheTagOffASeriesRenamesOnlyItsFolder(t *testing.T) {
	t.Parallel()
	e := newEditEnv(t, nil)
	chapter := e.write(taggedBreaking+"/Season 01/Breaking Bad (2008) - S01E01 - Pilot.mkv", "pilot")
	sub := e.write(taggedBreaking+"/Season 01/Breaking Bad (2008) - S01E01 - Pilot.es.srt", "sub")
	titleID := recordTitle(e.t, e.conn, mustTitle(e.t, domain.KindSeries, "Breaking Bad", taggedBreaking, 1396, true),
		e.video(domain.KindSeries, chapter, domain.Place{Episode: true, Season: 1, First: 1, Last: 1}, ""),
		mustSubtitle(t, domain.SubtitleSpec{Rel: sub, VideoRel: chapter, Language: "es", Size: 3}))

	res := e.setTag(titleID, false)
	if !res.Changed || res.Folder != plainBreaking {
		t.Fatalf("result = %+v", res)
	}
	newChapter := plainBreaking + "/Season 01/Breaking Bad (2008) - S01E01 - Pilot.mkv"
	if !e.exists(newChapter) || !e.exists(plainBreaking+"/Season 01/Breaking Bad (2008) - S01E01 - Pilot.es.srt") {
		t.Error("the chapter and its subtitle keep their names in the new folder")
	}
	if e.exists(taggedBreaking) {
		t.Error("the tagged folder is gone")
	}
	title, ok := e.titleOf(plainBreaking)
	if !ok || title.IDTag || title.TMDBID != 1396 {
		t.Errorf("the registry title = %+v, found %v", title, ok)
	}
	if got := e.rows(titleID)[newChapter]; got.ID == 0 || got.Place.Season != 1 || got.Place.First != 1 {
		t.Errorf("the registry chapter follows its new path: %+v", got)
	}
	e.assertNothingJournaled()
}

// TestPuttingTheTagOnASeriesKeepsTheNamesOfItsChapters puts the tag on a series imported without it, whose chapter has
// a name that spec §5 does not write: the folder takes the tag and the chapter keeps its name (RF-29, RF-45).
func TestPuttingTheTagOnASeriesKeepsTheNamesOfItsChapters(t *testing.T) {
	t.Parallel()
	e := newEditEnv(t, nil)
	chapter := e.write(plainBreaking+"/Season 01/bb.S01E02.mkv", "imported")
	sub := e.write(plainBreaking+"/Season 01/bb.S01E02.es.srt", "sub")
	titleID := recordTitle(e.t, e.conn, mustTitle(e.t, domain.KindSeries, "Breaking Bad", plainBreaking, 1396, false),
		e.video(domain.KindSeries, chapter, domain.Place{Episode: true, Season: 1, First: 2, Last: 2}, ""),
		mustSubtitle(t, domain.SubtitleSpec{Rel: sub, VideoRel: chapter, Language: "es", Size: 3}))

	res := e.setTag(titleID, true)
	if !res.Changed || res.Folder != taggedBreaking {
		t.Fatalf("result = %+v", res)
	}
	if !e.exists(taggedBreaking+"/Season 01/bb.S01E02.mkv") || !e.exists(taggedBreaking+"/Season 01/bb.S01E02.es.srt") {
		t.Error("the chapter and its subtitle keep their names in the tagged folder")
	}
	if e.exists(plainBreaking) {
		t.Error("the untagged folder is gone")
	}
	if title, ok := e.titleOf(taggedBreaking); !ok || !title.IDTag {
		t.Errorf("the registry title = %+v, found %v: with the tag", title, ok)
	}
	e.assertNothingJournaled()
}

// TestTheTagOfAFreeTitleIsRefused checks that a free title has no tag to put or take: its plan is not valid, and its
// commit is refused, whatever the tag asked for (RF-45).
func TestTheTagOfAFreeTitleIsRefused(t *testing.T) {
	t.Parallel()
	for _, want := range []bool{true, false} {
		e := newEditEnv(t, nil)
		video := e.write(ironFolder+"/Iron Man (2008).mkv", "iron")
		titleID := e.seedMovie(ironFolder, 0, false, e.video(domain.KindMovie, video, domain.Place{}, ""))

		p := e.tagPlan(titleID, application.IDTagRequest{IDTag: want})
		if p.Valid || !hasEditIssue(p.Errors, "title", "free_title") {
			t.Errorf("tag %t: the plan = valid %v, errors %+v; want free_title", want, p.Valid, p.Errors)
		}
		_, err := e.edits.CommitIDTag(e.ctx, titleID, application.IDTagRequest{Revision: p.Revision, IDTag: want})
		var invalid *application.EditInvalidError
		if !errors.As(err, &invalid) {
			t.Errorf("tag %t: the commit err = %v, want an invalid edit", want, err)
		}
		if !e.exists(video) || e.exists(taggedIron) {
			t.Errorf("tag %t: the free title did not move", want)
		}
		e.assertNothingJournaled()
	}
}

// TestTheTagActionRefusesAStaleRevision checks that a plan made before another change of the title is not stored
// (RF-25: plan_changed).
func TestTheTagActionRefusesAStaleRevision(t *testing.T) {
	t.Parallel()
	e := newEditEnv(t, nil)
	video := e.write(ironFolder+"/Iron Man (2008).mkv", "iron")
	titleID := e.seedMovie(ironFolder, 1726, false, e.video(domain.KindMovie, video, domain.Place{}, ""))
	stale := e.tagPlan(titleID, application.IDTagRequest{IDTag: true})

	e.setTag(titleID, true)
	_, err := e.edits.CommitIDTag(e.ctx, titleID, application.IDTagRequest{Revision: stale.Revision, IDTag: true})
	if !errors.Is(err, application.ErrPlanChanged) {
		t.Errorf("a commit with the old revision: err = %v, want ErrPlanChanged", err)
	}
}

// TestAFileAlreadyAtTheNewPathIsADecision checks RF-24 in the tag action: a file that the disk holds at the new name
// (copied by Samba, not registered) is a duplicate that the plan asks about, and Replace sends it to the trash.
func TestAFileAlreadyAtTheNewPathIsADecision(t *testing.T) {
	t.Parallel()
	e := newEditEnv(t, nil)
	whole := e.write(taggedIron+"/Iron Man (2008) [tmdbid-1726].mkv", "iron")
	occupant := e.write(ironFolder+"/Iron Man (2008).mkv", "old copy")
	titleID := e.seedMovie(taggedIron, 1726, true, e.video(domain.KindMovie, whole, domain.Place{}, ""))
	rows := e.rows(titleID)

	p := e.tagPlan(titleID, application.IDTagRequest{IDTag: false})
	if p.Valid || !hasEditIssue(p.Errors, "duplicate", "required") {
		t.Fatalf("the plan asks for a decision: valid=%v errors=%+v", p.Valid, p.Errors)
	}
	if len(p.Videos) != 1 {
		t.Fatalf("the videos of the plan: %+v", p.Videos)
	}
	allowed := p.Videos[0].Allowed
	if len(allowed) != 2 || allowed[0] != domain.DecisionReplace || allowed[1] != domain.DecisionKeepBoth {
		t.Errorf("the answers the plan accepts = %v, want Replace and Keep both (Omitir is not offered when the folder changes)", allowed)
	}

	req := application.IDTagRequest{Revision: p.Revision, IDTag: false, Videos: []application.IDTagVideo{
		{FileID: rows[whole].ID, Duplicate: domain.DecisionReplace},
	}}
	p = e.tagPlan(titleID, req)
	if !p.Valid {
		t.Fatalf("with Replace: %+v", p.Errors)
	}
	req.Revision = p.Revision
	if _, err := e.edits.CommitIDTag(e.ctx, titleID, req); err != nil {
		t.Fatalf("CommitIDTag: %v", err)
	}
	if got := e.read(occupant); got != "iron" {
		t.Errorf("the video took the place: %q", got)
	}
	if !e.inTrash("old copy") {
		t.Error("the copy that was there is in the trash")
	}
	e.assertNothingJournaled()
}

// TestChangingTheMatchKeepsTheTagState checks RF-44: choosing a TMDB match again, or another one, keeps the state of
// the tag: a title without it stays without it, and one with it keeps it.
func TestChangingTheMatchKeepsTheTagState(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name   string
		folder string
		tagged bool
		want   string
	}{
		{name: "without the tag", folder: ironFolder, tagged: false, want: ironFolder},
		{name: "with the tag", folder: taggedIron, tagged: true, want: taggedIron},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			e := newEditEnv(t, nil)
			video := e.write(tt.folder+"/Iron Man (2008).mkv", "iron")
			titleID := e.seedMovie(tt.folder, 1726, tt.tagged, e.video(domain.KindMovie, video, domain.Place{}, ""))
			req := movieChoices(e.rows(titleID), application.TitleChoice{Source: "tmdb", Kind: domain.KindMovie, TMDBID: 1726})

			p := e.plan(titleID, req)
			if !p.Valid || p.Folder != tt.want {
				t.Fatalf("plan: valid=%v folder=%q errors=%+v, want folder %q", p.Valid, p.Folder, p.Errors, tt.want)
			}
			e.commit(titleID, withRevision(req, p.Revision))
			if title, ok := e.titleOf(tt.want); !ok || title.IDTag != tt.tagged {
				t.Errorf("the registry title = %+v, found %v: IDTag %t", title, ok, tt.tagged)
			}
		})
	}
}
