package sqlite_test

import (
	"slices"
	"testing"

	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/catalog/application"
	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/catalog/domain"
)

// The preview of the ID tag action lists the duplicates of each file it moves, as an edit does (RF-24, RF-25, RF-45).
// Taking the tag off renames the title's folder, so Omitir is never offered (RF-44).

// TestTheTagPlanListsAnExactCopyAndProposesNoSkip checks that taking the tag off lists the copy that the new folder
// already holds, with Replace and Keep both as the answers, and that no answer is proposed.
func TestTheTagPlanListsAnExactCopyAndProposesNoSkip(t *testing.T) {
	t.Parallel()
	e := newEditEnv(t, nil)
	whole := e.write(taggedIron+"/Iron Man (2008) [tmdbid-1726].mkv", "iron")
	copyRel := e.write(ironFolder+"/Iron Man (2008).mkv", "iron")
	titleID := e.seedMovie(taggedIron, 1726, true, e.video(domain.KindMovie, whole, domain.Place{}, ""))

	p := e.tagPlan(titleID, application.IDTagRequest{IDTag: false})
	if p.Valid || !hasEditIssue(p.Errors, "duplicate", "required") {
		t.Fatalf("the copy waits for an answer: valid=%v errors=%+v", p.Valid, p.Errors)
	}
	v := previewVideo(t, p, whole)
	if got := dupPaths(v); !slices.Equal(got, []string{copyRel}) {
		t.Errorf("the duplicates = %v, want the copy", got)
	}
	if !v.ExactCopy {
		t.Error("the copy is an exact copy")
	}
	if want := []domain.Decision{domain.DecisionReplace, domain.DecisionKeepBoth}; !slices.Equal(v.Allowed, want) {
		t.Errorf("the answers = %v, want %v", v.Allowed, want)
	}
	if v.Proposal != "" {
		t.Errorf("no answer is proposed when Omitir is not offered: %q", v.Proposal)
	}
	e.assertNothingJournaled()
}

// TestTheTagPlanListsAChapterClashOfTheNewFolder checks that taking the tag off a series lists the chapter that the
// untagged folder holds, where the chapter goes (its name stays, RF-45), and does not offer Omitir.
func TestTheTagPlanListsAChapterClashOfTheNewFolder(t *testing.T) {
	t.Parallel()
	e := newEditEnv(t, nil)
	chapter := e.write(taggedBreaking+"/Season 01/Breaking Bad (2008) - S01E01 - Pilot.mkv", "pilot")
	occupant := e.write(plainBreaking+"/Season 01/Breaking Bad (2008) - S01E01 - Pilot.mkv", "old copy")
	titleID := recordTitle(e.t, e.conn, mustTitle(e.t, domain.KindSeries, "Breaking Bad", taggedBreaking, 1396, true),
		e.video(domain.KindSeries, chapter, domain.Place{Episode: true, Season: 1, First: 1, Last: 1}, ""))

	p := e.tagPlan(titleID, application.IDTagRequest{IDTag: false})
	if p.Valid || !hasEditIssue(p.Errors, "duplicate", "required") {
		t.Fatalf("the chapter clashes with the untagged folder: valid=%v errors=%+v", p.Valid, p.Errors)
	}
	v := previewVideo(t, p, chapter)
	if got := dupPaths(v); !slices.Equal(got, []string{occupant}) {
		t.Errorf("the duplicates = %v, want the chapter of the untagged folder", got)
	}
	if v.ExactCopy {
		t.Error("the contents differ")
	}
	// The untagged chapter has a title in its name, so its version cannot be told apart from it: Keep both waits for
	// the import (RF-24), and Omitir is not offered either (RF-44).
	if want := []domain.Decision{domain.DecisionReplace}; !slices.Equal(v.Allowed, want) {
		t.Errorf("the answers = %v, want %v", v.Allowed, want)
	}
	if v.Proposal != "" {
		t.Errorf("no answer is proposed: %q", v.Proposal)
	}
	e.assertNothingJournaled()
}
