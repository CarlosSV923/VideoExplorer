package domain_test

import (
	"errors"
	"testing"

	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/catalog/domain"
)

// unlabelled is a version of the place that has no label of its own, and whose media cannot be read.
func unlabelled(rel string) domain.Occupant {
	return domain.Occupant{Video: domain.Existing{Rel: rel, Size: 10, VersionKnown: true}}
}

// newCut is the new video of the place, a 4K copy.
func newCut(version string) domain.Incoming {
	return domain.Incoming{
		Source: "_unassigned/Iron Man 4K.mkv", Kind: domain.KindMovie, Root: "Movies",
		Title: domain.Title{Name: "Iron Man", Year: 2008, TMDBID: 1726, IDTag: true}, Extension: ".mkv", Version: version,
	}
}

const ironOld = "Movies/Iron Man (2008) [tmdbid-1726]/Iron Man (2008) [tmdbid-1726].mkv"

func TestAnUnlabelledVersionTakesTheLabelTheUserWrote(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		label     string
		newLabel  string
		wantErr   error
		wantOld   string
		wantVer   string
		wantNewTo string
	}{
		{
			name: "the user's label renames the old version", label: "Director's cut", newLabel: "2160p",
			wantOld: "Movies/Iron Man (2008) [tmdbid-1726]/Iron Man (2008) [tmdbid-1726] - Director's cut.mkv", wantVer: "Director's cut",
			wantNewTo: "Movies/Iron Man (2008) [tmdbid-1726]/Iron Man (2008) [tmdbid-1726] - 2160p.mkv",
		},
		{
			name: "a label that the new version has gets a suffix", label: "2160p", newLabel: "2160p",
			wantOld: "Movies/Iron Man (2008) [tmdbid-1726]/Iron Man (2008) [tmdbid-1726] - 2160p.mkv", wantVer: "2160p",
			wantNewTo: "Movies/Iron Man (2008) [tmdbid-1726]/Iron Man (2008) [tmdbid-1726] - 2160p (2).mkv",
		},
		{
			name: "a label is cleaned like any name", label: "  Director:  cut  ", newLabel: "2160p",
			wantOld: "Movies/Iron Man (2008) [tmdbid-1726]/Iron Man (2008) [tmdbid-1726] - Director - cut.mkv", wantVer: "Director - cut",
			wantNewTo: "Movies/Iron Man (2008) [tmdbid-1726]/Iron Man (2008) [tmdbid-1726] - 2160p.mkv",
		},
		{name: "a label with nothing usable is refused", label: "  . ", newLabel: "2160p", wantErr: domain.ErrInvalidVersion},
		{name: "no label leaves the proposal incomplete", label: "", newLabel: "2160p", wantErr: domain.ErrIncompleteLabel},
		{name: "a new label with nothing usable is refused", label: "Director", newLabel: " . ", wantErr: domain.ErrInvalidVersion},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			o := unlabelled(ironOld)
			o.Label = tc.label
			plan, err := domain.PlanDecision(domain.DecisionKeepBoth, []domain.Occupant{o}, false, []domain.Incoming{newCut(tc.newLabel)})
			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("err = %v, want %v", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("PlanDecision: %v", err)
			}
			if len(plan.Renames) != 1 || plan.Renames[0].To != tc.wantOld || plan.Renames[0].Version != tc.wantVer {
				t.Errorf("renames = %+v, want %s labelled %q", plan.Renames, tc.wantOld, tc.wantVer)
			}
			if len(plan.Place) != 1 || plan.Place[0].To != tc.wantNewTo {
				t.Errorf("place = %+v, want %s", plan.Place, tc.wantNewTo)
			}
		})
	}
}

func TestAnUnlabelledVersionWithAReadableLabelNeedsNoUserLabel(t *testing.T) {
	t.Parallel()
	// A complete proposal names the old version without the user's help.
	old := domain.Occupant{
		Video: domain.Existing{Rel: ironOld, Size: 10, VersionKnown: true},
		Media: domain.VersionMedia{Width: 1920, Height: 1080, Codec: "h264", Audio: []domain.AudioMedia{{Language: "eng"}}},
	}
	in := newCut("")
	in.Media = domain.VersionMedia{Width: 3840, Height: 2160, Codec: "hevc", Audio: []domain.AudioMedia{{Language: "eng"}}}
	plan, err := domain.PlanDecision(domain.DecisionKeepBoth, []domain.Occupant{old}, false, []domain.Incoming{in})
	if err != nil {
		t.Fatalf("PlanDecision: %v", err)
	}
	if len(plan.Renames) != 1 || plan.Renames[0].Version != "1080p" || plan.Version != "2160p" {
		t.Errorf("plan = %+v, want the old version labelled 1080p and the new 2160p", plan)
	}
}

func TestReplaceIgnoresTheLabelsOfTheOldVersions(t *testing.T) {
	t.Parallel()
	o := unlabelled(ironOld)
	o.Label = "Director's cut"
	plan, err := domain.PlanDecision(domain.DecisionReplace, []domain.Occupant{o}, false, []domain.Incoming{newCut("")})
	if err != nil {
		t.Fatalf("PlanDecision: %v", err)
	}
	if len(plan.Renames) != 0 || len(plan.Trash) != 1 {
		t.Errorf("replace renames nothing and trashes the old version: %+v", plan)
	}
}
