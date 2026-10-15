package sqlite_test

import (
	"slices"
	"testing"

	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/catalog/application"
	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/catalog/domain"
)

// The preview of an edit lists the duplicates of each video it moves, with the answers they accept and the one the rule
// proposes, before the user answers (RF-24, RF-25, RF-44). These tests check it on a library in a temporary folder.

// previewVideo returns the video of the plan whose path before the edit is rel.
func previewVideo(t *testing.T, p application.EditPlan, rel string) application.EditedVideo {
	t.Helper()
	for _, v := range p.Videos {
		if v.File == rel {
			return v
		}
	}
	t.Fatalf("the plan has no video %s: %+v", rel, p.Videos)
	return application.EditedVideo{}
}

// dupPaths returns the paths of the duplicates a video of the plan lists.
func dupPaths(v application.EditedVideo) []string {
	var out []string
	for _, d := range v.Duplicates {
		out = append(out, d.Rel)
	}
	return out
}

// decide gives the answer to the duplicates of the video at rel in an edit request.
func decide(req application.EditRequest, rows map[string]domain.LibraryFile, rel string, d domain.Decision) application.EditRequest {
	for i := range req.Videos {
		if rows[rel].ID == req.Videos[i].FileID {
			req.Videos[i].Duplicate = d
		}
	}
	return req
}

// TestAChapterCopyIsListedAndSkipIsProposedBeforeTheAnswer checks a chapter that clashes with an exact copy of
// another chapter: without an answer the plan lists the copy and proposes Omitir, and the answer Omitir keeps the chapter
// where it is (RF-24).
func TestAChapterCopyIsListedAndSkipIsProposedBeforeTheAnswer(t *testing.T) {
	t.Parallel()
	e := newEditEnv(t, nil)
	pilot := e.write(lizzieFolder+"/Season 01/Lizzie (2008) - S01E01 - Pilot.mkv", "same")
	trial := e.write(lizzieFolder+"/Season 01/Lizzie (2008) - S01E02 - Trial.mkv", "same")
	titleID := recordTitle(e.t, e.conn, mustTitle(e.t, domain.KindSeries, "Lizzie", lizzieFolder, 0, false),
		e.video(domain.KindSeries, pilot, domain.Place{Episode: true, Season: 1, First: 1, Last: 1}, ""),
		e.video(domain.KindSeries, trial, domain.Place{Episode: true, Season: 1, First: 2, Last: 2}, ""))
	rows := e.rows(titleID)
	req := setChapter(chapterChoices(rows, titleID), rows, trial, 1)

	p := e.plan(titleID, req)
	if p.Valid || !hasEditIssue(p.Errors, "duplicate", "required") {
		t.Fatalf("the clash waits for an answer: valid=%v errors=%+v", p.Valid, p.Errors)
	}
	v := previewVideo(t, p, trial)
	if got := dupPaths(v); !slices.Equal(got, []string{pilot}) {
		t.Errorf("the duplicates = %v, want the pilot", got)
	}
	if !v.ExactCopy || v.Proposal != domain.DecisionSkip {
		t.Errorf("an exact copy proposes Omitir: exact=%t proposal=%q", v.ExactCopy, v.Proposal)
	}
	if want := []domain.Decision{domain.DecisionReplace, domain.DecisionKeepBoth, domain.DecisionSkip}; !slices.Equal(v.Allowed, want) {
		t.Errorf("the answers = %v, want %v", v.Allowed, want)
	}
	if e.exists(lizzieFolder+"/Season 01/Lizzie (2008) - S01E01 - Trial.mkv") || !e.exists(trial) {
		t.Error("nothing moved in the preview")
	}
	e.assertNothingJournaled()

	req = decide(withRevision(req, p.Revision), rows, trial, domain.DecisionSkip)
	p = e.plan(titleID, req)
	if !p.Valid {
		t.Fatalf("with Omitir: %+v", p.Errors)
	}
	e.commit(titleID, withRevision(req, p.Revision))
	if !e.exists(trial) {
		t.Error("the skipped chapter stays where it is")
	}
}

// TestAPartClashListsTheWholeFileAndOffersReplaceOrSkip checks RF-22 and RF-24 in an edit: a video that becomes a part
// clashes with the whole movie of the folder, so the plan lists it and offers only Replace or Omitir. Omitir is allowed
// because the folder does not change.
func TestAPartClashListsTheWholeFileAndOffersReplaceOrSkip(t *testing.T) {
	t.Parallel()
	e := newEditEnv(t, nil)
	version := e.write(taggedIron+"/Iron Man (2008) [tmdbid-1726] - 1080p.mkv", "version")
	whole := e.write(taggedIron+"/Iron Man (2008) [tmdbid-1726].mkv", "whole")
	titleID := e.seedMovie(taggedIron, 1726, true, e.video(domain.KindMovie, version, domain.Place{}, "1080p"))
	rows := e.rows(titleID)
	req := movieChoices(rows, application.TitleChoice{Source: "library", LibraryID: titleID})
	for i := range req.Videos {
		if req.Videos[i].FileID == rows[version].ID {
			req.Videos[i].Part = 1
			req.Videos[i].Version = ""
		}
	}

	p := e.plan(titleID, req)
	if p.Valid || !hasEditIssue(p.Errors, "duplicate", "required") {
		t.Fatalf("the part clashes with the whole file: valid=%v errors=%+v", p.Valid, p.Errors)
	}
	v := previewVideo(t, p, version)
	if got := dupPaths(v); !slices.Equal(got, []string{whole}) {
		t.Errorf("the duplicates = %v, want the whole file", got)
	}
	if want := []domain.Decision{domain.DecisionReplace, domain.DecisionSkip}; !slices.Equal(v.Allowed, want) {
		t.Errorf("the answers = %v, want %v (parts take no version)", v.Allowed, want)
	}
	if v.ExactCopy || v.Proposal != "" {
		t.Errorf("the contents differ: exact=%t proposal=%q", v.ExactCopy, v.Proposal)
	}

	req = decide(withRevision(req, p.Revision), rows, version, domain.DecisionSkip)
	p = e.plan(titleID, req)
	if !p.Valid {
		t.Fatalf("with Omitir: %+v", p.Errors)
	}
	e.commit(titleID, withRevision(req, p.Revision))
	if !e.exists(version) || e.read(whole) != "whole" {
		t.Error("Omitir keeps the video and the whole file where they are")
	}
}

// TestAnExactCopyWhoseFolderChangesOffersNoSkip checks RF-44: a title that links to TMDB moves to another folder, so
// the copy that is there is listed, and neither Omitir is offered nor proposed.
func TestAnExactCopyWhoseFolderChangesOffersNoSkip(t *testing.T) {
	t.Parallel()
	e := newEditEnv(t, nil)
	video := e.write(ironFolder+"/Iron Man (2008).mkv", "iron")
	titleID := e.seedMovie(ironFolder, 0, false, e.video(domain.KindMovie, video, domain.Place{}, ""))
	copyRel := e.write("Movies/Iron Man (2008) [tmdbid-1726]/Iron Man (2008) [tmdbid-1726].mkv", "iron")
	rows := e.rows(titleID)
	req := movieChoices(rows, application.TitleChoice{Source: "tmdb", Kind: domain.KindMovie, TMDBID: 1726})

	p := e.plan(titleID, req)
	if p.Valid || !hasEditIssue(p.Errors, "duplicate", "required") {
		t.Fatalf("the copy waits for an answer: valid=%v errors=%+v", p.Valid, p.Errors)
	}
	v := previewVideo(t, p, video)
	if got := dupPaths(v); !slices.Equal(got, []string{copyRel}) {
		t.Errorf("the duplicates = %v, want the copy", got)
	}
	if !v.ExactCopy {
		t.Error("the copy is an exact copy")
	}
	if want := []domain.Decision{domain.DecisionReplace, domain.DecisionKeepBoth}; !slices.Equal(v.Allowed, want) {
		t.Errorf("the answers = %v, want %v (Omitir is not offered when the folder changes)", v.Allowed, want)
	}
	if v.Proposal != "" {
		t.Errorf("no answer is proposed when Omitir is not offered: %q", v.Proposal)
	}
	e.assertNothingJournaled()
}
