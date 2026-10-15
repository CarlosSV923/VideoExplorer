package application

import (
	"context"
	"strings"

	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/catalog/domain"
)

// decideGroups answers the duplicates of every group that moves one of its files (RF-24): a group whose place
// is free takes its planned moves, and one whose place is taken takes the answer the form gave to it.
func (e *Edits) decideGroups(ctx context.Context, ev *editEval) error {
	for _, g := range ev.groups {
		if !g.hasMover() {
			continue
		}
		if err := e.decideGroup(ctx, ev, g); err != nil {
			return err
		}
	}
	return nil
}

// decideGroup decides the placement of one group of videos (RF-24). The videos of the group, and the videos
// that move, do not hold the place: they leave the duplicates' search. What is left is the duplicates.
func (e *Edits) decideGroup(ctx context.Context, ev *editEval, g *editGroup) error {
	members := g.members
	first := members[0]
	dec := first.choice.Duplicate
	for _, m := range members[1:] {
		if m.choice.Duplicate != dec {
			ev.addMembers(members, "duplicate", "mismatch", "")
			return nil
		}
	}
	skip := ev.movers()
	for _, m := range members {
		skip[strings.ToLower(m.file.Rel)] = true
	}
	dups, err := e.d.Finder.FindExcept(ctx, ev.id.folder, editTarget(ev.id.kind, first.place), first.file.Rel, skip)
	if err != nil {
		return err
	}
	if ev.id.kind == domain.KindMovie && !ev.folderChanged() {
		dups = ownCollisions(dups, members)
	}
	if len(dups.Files) == 0 {
		return nil
	}
	for _, m := range members {
		m.dups, m.hasDups, m.exclusive = dups, true, dups.Exclusive
	}
	if dec == "" {
		ev.addMembers(members, "duplicate", "required", "")
		return nil
	}
	if dec == domain.DecisionSkip {
		if ev.folderChanged() {
			ev.addMembers(members, "duplicate", "not_allowed", "")
			return nil
		}
		for _, m := range members {
			m.skipped, m.moves, m.decision = true, nil, dec
		}
		return nil
	}
	if dec == domain.DecisionKeepBoth && (len(members) > 1 || dups.Exclusive) {
		ev.addMembers(members, "duplicate", "not_allowed", "")
		return nil
	}
	if dec == domain.DecisionReplace {
		for _, m := range members {
			if m.version != "" {
				ev.addMembers(members, "version", "not_allowed", "")
				return nil
			}
		}
	}
	occ, err := e.d.Finder.Occupants(ctx, ev.id.folder, dups, e.d.SubtitleExts, e.d.Media)
	if err != nil {
		return err
	}
	incs := make([]domain.Incoming, 0, len(members))
	for _, m := range members {
		in := m.in
		if len(members) == 1 {
			in.Media, err = e.mediaOf(ctx, m.file.Rel)
			if err != nil {
				return err
			}
		}
		if dec == domain.DecisionKeepBoth {
			in.Version = m.version
		}
		incs = append(incs, in)
	}
	for _, m := range members {
		m.decision, m.occ = dec, occ
	}
	if len(members) == 1 {
		proposed := domain.ProposeVersions(occ, incs[0])
		first.occLabels = proposed
		first.labelProp, first.labelBad = proposed[len(occ)].Text, proposed[len(occ)].Incomplete
		if dec == domain.DecisionKeepBoth && !dups.Exclusive {
			if !ev.applyLabels(first, occ, proposed) {
				return nil
			}
		} else {
			ev.markLabels(occ)
		}
	} else {
		ev.markLabels(occ)
	}
	plan, err := domain.PlanDecision(dec, occ, dups.Exclusive, incs)
	if err != nil {
		field, code, ok := decisionIssue(err)
		if !ok {
			return err
		}
		ev.addMembers(members, field, code, "")
		return nil
	}
	for _, m := range members {
		m.moves = movesOfNames(plan.Place, namesOf(m))
		m.version = ""
		if dec == domain.DecisionKeepBoth {
			m.version = plan.Version
		}
	}
	ev.trash = append(ev.trash, plan.Trash...)
	ev.renames = append(ev.renames, plan.Renames...)
	return nil
}

// ownCollisions keeps, of the duplicates of a movie whose title keeps its folder, the files the registry does not
// know, and the files of the title only when they hold a path a member takes. The versions of one movie are not
// duplicates of each other: a version that changes its label clashes only with a file that has the same name
// (RF-24). A series keeps every duplicate of its chapter.
func ownCollisions(dups domain.Duplicates, members []*editVideo) domain.Duplicates {
	var kept []domain.Existing
	for _, existing := range dups.Files {
		if existing.ID == 0 {
			kept = append(kept, existing)
			continue
		}
		for _, m := range members {
			if strings.EqualFold(existing.Rel, m.finalOfVideo()) {
				kept = append(kept, existing)
				break
			}
		}
	}
	if len(kept) < len(dups.Files) {
		dups.ExactCopy = false
	}
	dups.Files = kept
	return dups
}

// namesOf returns the lower-case paths of a video and its subtitles: the files that move with it.
func namesOf(v *editVideo) map[string]bool {
	out := map[string]bool{strings.ToLower(v.file.Rel): true}
	for _, s := range v.subs {
		out[strings.ToLower(s.file.Rel)] = true
	}
	return out
}

// applyLabels gives each old version that needs one the label the user wrote for it (RF-24). An old version with
// a label of its own, or with a complete proposal, needs none: a label written for it is an issue, and so is a
// version that needs one and has none. It reports whether every label is usable.
func (ev *editEval) applyLabels(m *editVideo, occ []domain.Occupant, proposed []domain.VersionLabel) bool {
	ok := true
	for i := range occ {
		rel := occ[i].Video.Rel
		key := strings.ToLower(rel)
		given, has := ev.labels[key]
		if has {
			ev.labelUsed[key] = true
		}
		needs := occ[i].Video.Version == "" && proposed[i].Incomplete
		switch {
		case has && !needs:
			ev.addMembers([]*editVideo{m}, "version", "not_needed", rel)
			ok = false
		case has:
			if _, err := domain.CleanVersion(given); err != nil {
				ev.addMembers([]*editVideo{m}, "version", "invalid", rel)
				ok = false
				continue
			}
			occ[i].Label = given
		case needs:
			ev.addMembers([]*editVideo{m}, "version", "incomplete", rel)
			ok = false
		}
	}
	return ok
}

// markLabels marks the labels that address these versions as used, without applying them.
func (ev *editEval) markLabels(occ []domain.Occupant) {
	for _, o := range occ {
		key := strings.ToLower(o.Video.Rel)
		if _, ok := ev.labels[key]; ok {
			ev.labelUsed[key] = true
		}
	}
}
