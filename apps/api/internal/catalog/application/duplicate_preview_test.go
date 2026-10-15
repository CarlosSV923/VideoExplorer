package application

import (
	"slices"
	"testing"

	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/catalog/domain"
)

// previewOccupant is a video that holds the place, with its version as the registry or the name gives it.
func previewOccupant(rel, version string, known bool) domain.Occupant {
	return domain.Occupant{Video: domain.Existing{Rel: rel, Size: 10, Version: version, VersionKnown: known}}
}

// previewDups is what the search found: the videos, and whether they are parts or an exact copy.
func previewDups(exclusive, exact bool, occ ...domain.Occupant) domain.Duplicates {
	d := domain.Duplicates{Exclusive: exclusive, ExactCopy: exact}
	for _, o := range occ {
		d.Files = append(d.Files, o.Video)
	}
	return d
}

func TestPreviewOfDuplicates(t *testing.T) {
	t.Parallel()

	all := []domain.Decision{domain.DecisionReplace, domain.DecisionKeepBoth, domain.DecisionSkip}
	old := previewOccupant("Movies/Iron Man (2008)/Iron Man (2008).mkv", "", true)
	labeled := previewOccupant("Movies/Iron Man (2008)/Iron Man (2008) - 2160p.mkv", "2160p", true)
	unnamed := previewOccupant("Movies/Iron Man (2008)/Iron Man (2008).mkv", "", false)

	tests := []struct {
		name          string
		dups          domain.Duplicates
		occ           []domain.Occupant
		labels        []domain.VersionLabel
		folderChanged bool
		wantLen       int
		wantAllowed   []domain.Decision
		wantProposal  domain.Decision
		wantExact     bool
		wantLabel     string
		wantIncompl   bool
		wantNeeded    bool // of the first duplicate
	}{
		{
			name:    "no duplicate: the preview is empty",
			dups:    domain.Duplicates{},
			wantLen: 0,
		},
		{
			name:        "a clash offers every answer and proposes none",
			dups:        previewDups(false, false, old),
			occ:         []domain.Occupant{old},
			wantLen:     1,
			wantAllowed: all,
		},
		{
			name:         "an exact copy proposes Omitir",
			dups:         previewDups(false, true, old),
			occ:          []domain.Occupant{old},
			wantLen:      1,
			wantAllowed:  all,
			wantProposal: domain.DecisionSkip,
			wantExact:    true,
		},
		{
			name:          "an exact copy of a title whose folder changes proposes nothing: Omitir is not offered",
			dups:          previewDups(false, true, old),
			occ:           []domain.Occupant{old},
			folderChanged: true,
			wantLen:       1,
			wantAllowed:   []domain.Decision{domain.DecisionReplace, domain.DecisionKeepBoth},
			wantExact:     true,
		},
		{
			name:        "parts or a place with parts takes no version: Replace or Omitir",
			dups:        previewDups(true, false, old),
			occ:         []domain.Occupant{old},
			wantLen:     1,
			wantAllowed: []domain.Decision{domain.DecisionReplace, domain.DecisionSkip},
		},
		{
			name:          "parts whose folder changes take only Replace",
			dups:          previewDups(true, false, old),
			occ:           []domain.Occupant{old},
			folderChanged: true,
			wantLen:       1,
			wantAllowed:   []domain.Decision{domain.DecisionReplace},
		},
		{
			name:         "an exact copy of a part proposes Omitir, which it still allows",
			dups:         previewDups(true, true, old),
			occ:          []domain.Occupant{old},
			wantLen:      1,
			wantAllowed:  []domain.Decision{domain.DecisionReplace, domain.DecisionSkip},
			wantProposal: domain.DecisionSkip,
			wantExact:    true,
		},
		{
			name:        "a version the name does not tell takes no Keep both until it is imported",
			dups:        previewDups(false, false, unnamed),
			occ:         []domain.Occupant{unnamed},
			wantLen:     1,
			wantAllowed: []domain.Decision{domain.DecisionReplace, domain.DecisionSkip},
		},
		{
			name:        "the label the rule proposes for the old version and for the new one",
			dups:        previewDups(false, false, old),
			occ:         []domain.Occupant{old},
			labels:      []domain.VersionLabel{{Text: "720p", Incomplete: true}, {Text: "1080p", Incomplete: false}},
			wantLen:     1,
			wantAllowed: all,
			wantLabel:   "1080p",
			wantNeeded:  true,
		},
		{
			name:        "an old version with a label of its own needs none",
			dups:        previewDups(false, false, labeled),
			occ:         []domain.Occupant{labeled},
			labels:      []domain.VersionLabel{{Text: "2160p", Incomplete: true}, {Text: "1080p", Incomplete: false}},
			wantLen:     1,
			wantAllowed: all,
			wantLabel:   "1080p",
			wantNeeded:  false,
		},
		{
			name:        "an incomplete label of the new video is marked as such",
			dups:        previewDups(false, false, labeled),
			occ:         []domain.Occupant{labeled},
			labels:      []domain.VersionLabel{{Text: "2160p", Incomplete: false}, {Text: "1080p und", Incomplete: true}},
			wantLen:     1,
			wantAllowed: all,
			wantLabel:   "1080p und",
			wantIncompl: true,
		},
		{
			name:        "parts take no label of their own, even with a duplicate",
			dups:        previewDups(true, false, old),
			occ:         []domain.Occupant{old},
			wantLen:     1,
			wantAllowed: []domain.Decision{domain.DecisionReplace, domain.DecisionSkip},
			wantNeeded:  false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := previewOfDuplicates(tt.dups, tt.occ, tt.labels, tt.folderChanged)
			if len(got.duplicates) != tt.wantLen {
				t.Fatalf("duplicates = %+v, want %d", got.duplicates, tt.wantLen)
			}
			if !slices.Equal(got.allowed, tt.wantAllowed) {
				t.Errorf("allowed = %v, want %v", got.allowed, tt.wantAllowed)
			}
			if got.proposal != tt.wantProposal {
				t.Errorf("proposal = %q, want %q", got.proposal, tt.wantProposal)
			}
			if got.exactCopy != tt.wantExact {
				t.Errorf("exact copy = %t, want %t", got.exactCopy, tt.wantExact)
			}
			if got.versionProposal != tt.wantLabel {
				t.Errorf("version proposal = %q, want %q", got.versionProposal, tt.wantLabel)
			}
			if got.versionIncomplete != tt.wantIncompl {
				t.Errorf("version incomplete = %t, want %t", got.versionIncomplete, tt.wantIncompl)
			}
			if tt.wantLen > 0 && got.duplicates[0].VersionNeeded != tt.wantNeeded {
				t.Errorf("the old version needs a label = %t, want %t", got.duplicates[0].VersionNeeded, tt.wantNeeded)
			}
		})
	}
}
