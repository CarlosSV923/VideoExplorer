package application

import (
	"slices"

	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/catalog/domain"
)

// duplicatePreview is what a preview shows of the duplicates of one place (RF-24, RF-25): each video that holds the
// place with the label the rule proposes for it, the answers they accept, the answer the rule proposes first, and the
// label the rule proposes for the new video. Assignments and the edits build it the same way, so the preview lists the
// duplicates before the user answers, as it does after.
type duplicatePreview struct {
	duplicates        []PlannedDuplicate
	exactCopy         bool
	allowed           []domain.Decision
	proposal          domain.Decision
	versionProposal   string
	versionIncomplete bool
}

// previewOfDuplicates builds the preview of the duplicates of a place. dups is what the search found for the place
// and occ the same videos as occupants; labels are the labels the rule proposes for the occupants and then for the
// new video, or nil when the place takes several videos, which keep no label of their own (RF-24). folderChanged is
// true when the title moves to another folder: then Omitir is not offered, so it is not proposed either (RF-44).
// Without duplicates the preview is empty.
func previewOfDuplicates(dups domain.Duplicates, occ []domain.Occupant, labels []domain.VersionLabel, folderChanged bool) duplicatePreview {
	if len(dups.Files) == 0 {
		return duplicatePreview{}
	}
	out := duplicatePreview{exactCopy: dups.ExactCopy}
	for i, o := range occ {
		d := PlannedDuplicate{Rel: o.Video.Rel, Size: o.Video.Size, Version: o.Video.Version}
		if i < len(labels) {
			d.VersionProposal = labels[i].Text
			d.VersionNeeded = o.Video.Version == "" && labels[i].Incomplete
		}
		out.duplicates = append(out.duplicates, d)
	}
	if len(labels) > len(occ) {
		out.versionProposal = labels[len(occ)].Text
		out.versionIncomplete = labels[len(occ)].Incomplete
	}
	out.allowed = allowedFor(dups.Exclusive || unknownVersion(occ))
	if folderChanged {
		out.allowed = withoutSkip(out.allowed)
	}
	if dups.ExactCopy && slices.Contains(out.allowed, domain.DecisionSkip) {
		out.proposal = domain.DecisionSkip
	}
	return out
}

// withoutSkip returns the answers without Omitir.
func withoutSkip(in []domain.Decision) []domain.Decision {
	var out []domain.Decision
	for _, d := range in {
		if d != domain.DecisionSkip {
			out = append(out, d)
		}
	}
	return out
}
