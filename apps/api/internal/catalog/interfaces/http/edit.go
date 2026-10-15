package http

import (
	"context"
	"errors"
	"net/http"

	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/catalog/application"
	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/catalog/domain"
	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/platform/httpapi"
)

// PlanTitleEdit implements httpapi.StrictServerInterface (RF-44): the preview of an edit of a title.
func (h *LibraryHandler) PlanTitleEdit(ctx context.Context, req httpapi.PlanTitleEditRequestObject) (httpapi.PlanTitleEditResponseObject, error) {
	choices, ok := editRequestOf(req.Body)
	if !ok {
		return httpapi.PlanTitleEdit400ApplicationProblemPlusJSONResponse{
			BadRequestApplicationProblemPlusJSONResponse: httpapi.BadRequestApplicationProblemPlusJSONResponse(badRequest()),
		}, nil
	}
	plan, err := h.edits.Plan(ctx, req.Id, choices)
	switch {
	case errors.Is(err, application.ErrTitleNotFound):
		return httpapi.PlanTitleEdit404ApplicationProblemPlusJSONResponse{
			NotFoundApplicationProblemPlusJSONResponse: httpapi.NotFoundApplicationProblemPlusJSONResponse(notFoundProblem()),
		}, nil
	case err != nil:
		return nil, err
	}
	return httpapi.PlanTitleEdit200JSONResponse(editPlanToAPI(plan)), nil
}

// CommitTitleEdit implements httpapi.StrictServerInterface (RF-44): it stores the edit of a title as one change.
func (h *LibraryHandler) CommitTitleEdit(ctx context.Context, req httpapi.CommitTitleEditRequestObject) (httpapi.CommitTitleEditResponseObject, error) {
	choices, ok := editRequestOf(req.Body)
	if !ok {
		return httpapi.CommitTitleEdit400ApplicationProblemPlusJSONResponse{
			BadRequestApplicationProblemPlusJSONResponse: httpapi.BadRequestApplicationProblemPlusJSONResponse(badRequest()),
		}, nil
	}
	res, err := h.edits.Commit(ctx, req.Id, choices)
	var invalid *application.EditInvalidError
	switch {
	case errors.Is(err, application.ErrTitleNotFound):
		return httpapi.CommitTitleEdit404ApplicationProblemPlusJSONResponse{
			NotFoundApplicationProblemPlusJSONResponse: httpapi.NotFoundApplicationProblemPlusJSONResponse(notFoundProblem()),
		}, nil
	case errors.Is(err, application.ErrPlanChanged), errors.Is(err, application.ErrTargetTaken):
		if errors.Is(err, application.ErrTargetTaken) {
			h.log.WarnContext(ctx, "edit refused: a target was taken", "title", req.Id, "error", err)
		}
		return httpapi.CommitTitleEdit409ApplicationProblemPlusJSONResponse{
			EditConflictApplicationProblemPlusJSONResponse: editConflict(),
		}, nil
	case errors.As(err, &invalid):
		return httpapi.CommitTitleEdit422JSONResponse{EditInvalidJSONResponse: httpapi.EditInvalidJSONResponse(editPlanToAPI(invalid.Plan))}, nil
	case err != nil:
		return nil, err
	}
	return httpapi.CommitTitleEdit200JSONResponse(httpapi.EditResult{
		TitleId: res.TitleID, Folder: res.Folder, Merged: res.Merged, Changed: res.Changed,
	}), nil
}

// editConflict is the 409 of an edit whose title changed since its plan (RF-25).
func editConflict() httpapi.EditConflictApplicationProblemPlusJSONResponse {
	detail := "The title changed since the plan was made. Nothing was changed; check the plan again."
	return httpapi.EditConflictApplicationProblemPlusJSONResponse(httpapi.EditProblem{
		Code: httpapi.EditProblemCodePlanChanged, Status: http.StatusConflict, Title: "Conflict", Detail: &detail,
	})
}

// editRequestOf turns the body of an edit into the application's request. It answers false when the body is missing.
func editRequestOf(body *httpapi.EditRequest) (application.EditRequest, bool) {
	if body == nil {
		return application.EditRequest{}, false
	}
	out := application.EditRequest{Revision: body.Revision, Title: titleOf(body.Title)}
	for _, v := range body.Videos {
		c := application.EditVideo{
			FileID:       v.FileId,
			Part:         intOf(v.Part),
			Season:       v.Season,
			Chapter:      v.Chapter,
			LastChapter:  v.LastChapter,
			ChapterTitle: stringOf(v.ChapterTitle),
			Version:      stringOf(v.Version),
		}
		if v.Duplicate != nil {
			c.Duplicate = domain.Decision(*v.Duplicate)
		}
		out.Videos = append(out.Videos, c)
	}
	for _, s := range body.Subtitles {
		out.Subtitles = append(out.Subtitles, application.EditSubtitle{
			FileID: s.FileId, Language: s.Language, Forced: boolOf(s.Forced), SDH: boolOf(s.Sdh),
		})
	}
	if body.Labels != nil {
		for _, l := range *body.Labels {
			out.Labels = append(out.Labels, application.LabelChoice{Rel: l.Rel, Version: l.Version})
		}
	}
	return out, true
}

// editPlanToAPI turns the preview of an edit into the contract.
func editPlanToAPI(p application.EditPlan) httpapi.EditPlan {
	out := httpapi.EditPlan{
		TitleId:   p.TitleID,
		Revision:  p.Revision,
		Valid:     p.Valid,
		Kind:      httpapi.TitleKind(p.Kind),
		Title:     p.Title,
		Folder:    p.Folder,
		Videos:    []httpapi.EditPlanVideo{},
		Subtitles: []httpapi.EditPlanSubtitle{},
		Errors:    editIssuesToAPI(p.Errors),
		Warnings:  editIssuesToAPI(p.Warnings),
	}
	if p.MergeInto != 0 {
		merged := p.MergeInto
		out.MergeInto = &merged
	}
	for _, v := range p.Videos {
		item := httpapi.EditPlanVideo{
			FileId:     v.FileID,
			File:       v.File,
			Changed:    v.Changed,
			Duplicates: []httpapi.PlanDuplicate{},
			ExactCopy:  v.ExactCopy,
			Allowed:    []httpapi.DuplicateDecision{},
		}
		if v.FinalPath != "" {
			finalPath := v.FinalPath
			item.FinalPath = &finalPath
		}
		for _, d := range v.Duplicates {
			item.Duplicates = append(item.Duplicates, duplicateToAPI(d))
		}
		for _, a := range v.Allowed {
			item.Allowed = append(item.Allowed, httpapi.DuplicateDecision(a))
		}
		if v.Proposal != "" {
			proposal := httpapi.DuplicateDecision(v.Proposal)
			item.Proposal = &proposal
		}
		if v.Decision != "" {
			decision := httpapi.DuplicateDecision(v.Decision)
			item.Decision = &decision
		}
		if v.Version != "" {
			item.Version = &v.Version
		}
		if v.VersionProposal != "" {
			item.VersionProposal = &v.VersionProposal
		}
		if v.VersionIncomplete {
			incomplete := true
			item.VersionIncomplete = &incomplete
		}
		out.Videos = append(out.Videos, item)
	}
	for _, s := range p.Subtitles {
		sub := httpapi.EditPlanSubtitle{FileId: s.FileID, File: s.File, Changed: s.Changed}
		if s.FinalPath != "" {
			finalPath := s.FinalPath
			sub.FinalPath = &finalPath
		}
		out.Subtitles = append(out.Subtitles, sub)
	}
	return out
}

// duplicateToAPI turns a video that already holds a place into the contract (RF-24).
func duplicateToAPI(d application.PlannedDuplicate) httpapi.PlanDuplicate {
	dup := httpapi.PlanDuplicate{Rel: d.Rel, Size: d.Size}
	if d.Version != "" {
		dup.Version = &d.Version
	}
	if d.VersionProposal != "" {
		proposal := d.VersionProposal
		dup.VersionProposal = &proposal
	}
	if d.VersionNeeded {
		needed := true
		dup.VersionNeeded = &needed
	}
	return dup
}

// editIssuesToAPI turns the issues of an edit into the contract's plan issues: the file they are about, when any.
func editIssuesToAPI(issues []application.EditIssue) []httpapi.PlanIssue {
	out := make([]httpapi.PlanIssue, 0, len(issues))
	for _, i := range issues {
		item := httpapi.PlanIssue{Field: i.Field, Code: i.Code}
		if i.File != "" {
			file := i.File
			item.File = &file
		}
		if i.Target != "" {
			target := i.Target
			item.Target = &target
		}
		out = append(out, item)
	}
	return out
}
