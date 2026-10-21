package http

import (
	"context"
	"errors"
	"time"

	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/catalog/application"
	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/catalog/domain"
	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/platform/httpapi"
)

// TrashHandler exposes listing, restoring and permanently deleting trash entries (RF-30, RF-31, RF-31a).
type TrashHandler struct {
	svc       *application.Trash
	retention time.Duration
}

// NewTrashHandler builds the protected trash endpoints. retentionDays is TRASH_RETENTION_DAYS.
func NewTrashHandler(svc *application.Trash, retentionDays int) *TrashHandler {
	return &TrashHandler{svc: svc, retention: time.Duration(retentionDays) * 24 * time.Hour}
}

// ListTrash implements the newest-first listing of trash entries (RF-31a).
func (h *TrashHandler) ListTrash(ctx context.Context, _ httpapi.ListTrashRequestObject) (httpapi.ListTrashResponseObject, error) {
	entries, err := h.svc.Entries(ctx)
	if err != nil {
		return nil, err
	}
	out := httpapi.TrashList{Entries: make([]httpapi.TrashEntry, 0, len(entries))}
	for _, e := range entries {
		files := make([]httpapi.TrashFile, 0, len(e.Files))
		for _, f := range e.Files {
			files = append(files, httpapi.TrashFile{Name: f.Name, Size: f.Size})
		}
		batch, rel := e.BatchID, e.RelativePath
		row := httpapi.TrashEntry{
			Id: e.ID, Origin: httpapi.TrashEntryOrigin(e.Origin), Reason: httpapi.TrashEntryReason(e.Reason),
			TrashedAt: e.TrashedAt, PurgeAt: e.TrashedAt.Add(h.retention), Files: files,
		}
		if batch != "" {
			row.BatchId = &batch
		}
		if rel != "" {
			row.RelativePath = &rel
		}
		out.Entries = append(out.Entries, row)
	}
	return httpapi.ListTrash200JSONResponse(out), nil
}

// EmptyTrash permanently removes all entries through the engine (RF-30, RF-31a).
func (h *TrashHandler) EmptyTrash(ctx context.Context, _ httpapi.EmptyTrashRequestObject) (httpapi.EmptyTrashResponseObject, error) {
	if err := h.svc.Empty(ctx); err != nil {
		return nil, err
	}
	return httpapi.EmptyTrash204Response{}, nil
}

// DeleteTrashEntry permanently removes one entry through the engine (RF-30, RF-31a).
func (h *TrashHandler) DeleteTrashEntry(ctx context.Context, req httpapi.DeleteTrashEntryRequestObject) (httpapi.DeleteTrashEntryResponseObject, error) {
	if err := h.svc.Delete(ctx, req.Id); err != nil {
		if errors.Is(err, application.ErrTrashEntryNotFound) {
			return httpapi.DeleteTrashEntry404ApplicationProblemPlusJSONResponse{NotFoundApplicationProblemPlusJSONResponse: httpapi.NotFoundApplicationProblemPlusJSONResponse(notFoundProblem())}, nil
		}
		return nil, err
	}
	return httpapi.DeleteTrashEntry204Response{}, nil
}

// PlanTrashRestore previews destinations and conflicts without changing the library (RF-31, RF-31a).
func (h *TrashHandler) PlanTrashRestore(ctx context.Context, req httpapi.PlanTrashRestoreRequestObject) (httpapi.PlanTrashRestoreResponseObject, error) {
	p, err := h.svc.PlanRestore(ctx, req.Id)
	if err != nil {
		if errors.Is(err, application.ErrTrashEntryNotFound) {
			return httpapi.PlanTrashRestore404ApplicationProblemPlusJSONResponse{NotFoundApplicationProblemPlusJSONResponse: httpapi.NotFoundApplicationProblemPlusJSONResponse(notFoundProblem())}, nil
		}
		return nil, err
	}
	return httpapi.PlanTrashRestore200JSONResponse(trashRestorePlanToAPI(p)), nil
}

func trashRestorePlanToAPI(p application.TrashRestorePlan) httpapi.TrashRestorePlan {
	out := httpapi.TrashRestorePlan{EntryId: p.EntryID, Files: make([]httpapi.TrashRestoreFilePlan, 0, len(p.Files))}
	for _, f := range p.Files {
		v := httpapi.TrashRestoreFilePlan{File: f.File, Destinations: append([]string{}, f.Destinations...), Duplicates: make([]httpapi.PlanDuplicate, 0, len(f.Duplicates)), Allowed: make([]httpapi.TrashRestoreDecision, 0, len(f.Allowed))}
		if f.VersionProposal != "" {
			proposal := f.VersionProposal
			v.VersionProposal = &proposal
		}
		v.VersionIncomplete = f.VersionIncomplete
		for _, d := range f.Duplicates {
			v.Duplicates = append(v.Duplicates, duplicateToAPI(d))
		}
		for _, d := range f.Allowed {
			v.Allowed = append(v.Allowed, httpapi.TrashRestoreDecision(d))
		}
		if f.Proposal != "" {
			proposal := httpapi.TrashRestoreDecision(f.Proposal)
			v.Proposal = &proposal
		}
		out.Files = append(out.Files, v)
	}
	return out
}

// RestoreTrashEntry commits the preview through the catalog engine (RF-31, RF-31a).
func (h *TrashHandler) RestoreTrashEntry(ctx context.Context, req httpapi.RestoreTrashEntryRequestObject) (httpapi.RestoreTrashEntryResponseObject, error) {
	var choices []application.TrashRestoreDecision
	if req.Body != nil {
		for _, d := range req.Body.Decisions {
			version := ""
			if d.Version != nil {
				version = *d.Version
			}
			choices = append(choices, application.TrashRestoreDecision{File: d.File, Decision: domain.Decision(d.Decision), Version: version})
		}
	}
	paths, err := h.svc.Restore(ctx, req.Id, choices)
	if err != nil {
		if errors.Is(err, application.ErrTrashEntryNotFound) {
			return httpapi.RestoreTrashEntry404ApplicationProblemPlusJSONResponse{NotFoundApplicationProblemPlusJSONResponse: httpapi.NotFoundApplicationProblemPlusJSONResponse(notFoundProblem())}, nil
		}
		if errors.Is(err, application.ErrTrashRestoreCancelled) {
			return httpapi.RestoreTrashEntry200JSONResponse(httpapi.TrashRestoreResult{Restored: []string{}}), nil
		}
		if errors.Is(err, application.ErrTrashRestoreDecisionRequired) || errors.Is(err, application.ErrTargetTaken) {
			return httpapi.RestoreTrashEntry409ApplicationProblemPlusJSONResponse{EditConflictApplicationProblemPlusJSONResponse: editConflict()}, nil
		}
		return nil, err
	}
	return httpapi.RestoreTrashEntry200JSONResponse(httpapi.TrashRestoreResult{Restored: append([]string{}, paths...)}), nil
}
