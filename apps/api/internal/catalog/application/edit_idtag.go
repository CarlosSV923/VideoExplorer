package application

import (
	"context"
	"fmt"
	"strings"

	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/catalog/domain"
)

// The ID tag of a title (spec RF-45): putting the tag [tmdbid-ID] on a linked title or taking it off. The action is an
// edit that keeps the choices of every file (place, version, language and flags) and changes only the tag, so it goes
// through the same plan and the same commit as an edit (RF-44, RF-25), with the same checks and the same undo. What
// differs is the names: a movie and its files take the names of spec §5 with or without the tag, while a series moves
// only its folder, so its chapters and subtitles keep their names (RF-45, RF-29).

// IDTagRequest is what the ID tag action sends (RF-45): the revision of the plan it answers (RF-25), the tag the title
// takes (true puts it on, false takes it off), the answers to the duplicates of its videos (RF-24) and the labels of
// the old versions that need one. A video that is not in Videos keeps its choices.
type IDTagRequest struct {
	Revision string
	IDTag    bool
	Videos   []IDTagVideo
	Labels   []LabelChoice
}

// IDTagVideo is the answer of the form to one video of the title: the label it takes as a version, and the answer
// to the duplicate of its place (RF-24). The place of a video is not chosen by the action.
type IDTagVideo struct {
	FileID    int64
	Version   string
	Duplicate domain.Decision
}

// PlanIDTag is the preview of the ID tag action of a title (RF-45). It changes nothing. A free title has no tag: its
// plan is not valid (free_title).
func (e *Edits) PlanIDTag(ctx context.Context, titleID int64, req IDTagRequest) (EditPlan, error) {
	st, err := e.load(ctx, titleID)
	if err != nil {
		return EditPlan{}, err
	}
	ev, err := e.assessIDTag(ctx, st, req)
	if err != nil {
		return EditPlan{}, err
	}
	return ev.plan, nil
}

// CommitIDTag stores the ID tag action of a title as one change of the library (RF-45, RNF-04). The revision must be
// the one of the plan (plan_changed, RF-25), and an invalid plan is refused as an edit is.
func (e *Edits) CommitIDTag(ctx context.Context, titleID int64, req IDTagRequest) (EditResult, error) {
	return e.commitWith(ctx, titleID, req.Revision, "edit.id_tag", func(st editState) (*editEval, error) {
		return e.assessIDTag(ctx, st, req)
	})
}

// assessIDTag checks the ID tag action: the title keeps the choices of its files and takes the tag req.IDTag.
func (e *Edits) assessIDTag(ctx context.Context, st editState, req IDTagRequest) (*editEval, error) {
	return e.assess(ctx, st, idTagChoices(st, req), &req.IDTag)
}

// idTagChoices is the edit the ID tag action performs: every live file keeps its choices, and the answers of the
// request give the duplicate answer of the videos they name and their label when they give one. An answer for a file that is not a video
// of the title, or a second answer for one file, is passed on as a choice the title does not have, so that the plan
// reports it (RF-44).
func idTagChoices(st editState, in IDTagRequest) EditRequest {
	req := keepChoices(st)
	index := map[int64]int{}
	for i, v := range req.Videos {
		index[v.FileID] = i
	}
	answered := map[int64]bool{}
	for _, c := range in.Videos {
		i, known := index[c.FileID]
		if !known || answered[c.FileID] {
			req.Videos = append(req.Videos, EditVideo{FileID: c.FileID, Version: c.Version, Duplicate: c.Duplicate})
			continue
		}
		answered[c.FileID] = true
		if c.Version != "" {
			req.Videos[i].Version = c.Version
		}
		req.Videos[i].Duplicate = c.Duplicate
	}
	req.Labels = in.Labels
	return req
}

// keepChoices is the request that keeps every live file of the title as it is, with the title it has now (RF-45).
func keepChoices(st editState) EditRequest {
	req := EditRequest{Revision: st.revision, Title: TitleChoice{Source: "library", LibraryID: st.title.ID}}
	for _, f := range st.files {
		if !f.Live() {
			continue
		}
		switch f.Kind {
		case domain.FileVideo:
			v := EditVideo{FileID: f.ID, Part: f.Place.Part, Version: f.Version}
			if f.Place.Episode {
				season, first, last := f.Place.Season, f.Place.First, f.Place.Last
				v.Season, v.Chapter = &season, &first
				if last > first {
					v.LastChapter = &last
				}
			}
			req.Videos = append(req.Videos, v)
		case domain.FileSubtitle:
			req.Subtitles = append(req.Subtitles, EditSubtitle{FileID: f.ID, Language: f.Language, Forced: f.Forced, SDH: f.SDH})
		}
	}
	return req
}

// keepsNames reports whether the edit keeps the names of the files and moves only their folders: the ID tag of a
// series (RF-45). Every other edit names its files by spec §5.
func (ev *editEval) keepsNames() bool {
	return ev.idTag != nil && ev.id.kind == domain.KindSeries
}

// folderMoves returns the moves of a chapter that keeps its name (RF-45): the video and each of its subtitles go from
// the title's old folder to its new one, with the same names inside it.
func (ev *editEval) folderMoves(v *editVideo) ([]domain.Move, error) {
	from, to := ev.st.title.Folder, ev.id.folder
	moved, err := moveUnder(v.file.Rel, from, to)
	if err != nil {
		return nil, err
	}
	moves := []domain.Move{{From: v.file.Rel, To: moved}}
	for _, s := range v.subs {
		moved, err := moveUnder(s.file.Rel, from, to)
		if err != nil {
			return nil, err
		}
		moves = append(moves, domain.Move{From: s.file.Rel, To: moved})
	}
	return moves, nil
}

// moveUnder returns rel with the folder from replaced by the folder to, keeping the rest of the path. It fails when rel
// is not inside from, which the registry does not allow for the files of a title.
func moveUnder(rel, from, to string) (string, error) {
	prefix := from + "/"
	if len(rel) <= len(prefix) || !strings.EqualFold(rel[:len(prefix)], prefix) {
		return "", fmt.Errorf("catalog: %q is not inside its title folder %q", rel, from)
	}
	return to + "/" + rel[len(prefix):], nil
}
