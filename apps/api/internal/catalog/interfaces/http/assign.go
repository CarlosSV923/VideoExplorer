// Package http adapts the catalog use cases to the generated HTTP contract: the assignment of the
// entries of No asignados (RF-20 to RF-26) and the title search (RF-21).
package http

import (
	"context"
	"errors"
	"log/slog"
	"net/http"

	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/catalog/application"
	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/platform/httpapi"
)

// Handler implements the assignment operations of httpapi.StrictServerInterface.
type Handler struct {
	svc *application.Assignments
	log *slog.Logger
}

// NewHandler builds the handler over the assignment use cases. log may be nil.
func NewHandler(svc *application.Assignments, log *slog.Logger) *Handler {
	if log == nil {
		log = slog.New(slog.DiscardHandler)
	}
	return &Handler{svc: svc, log: log}
}

// TakeAssignLock implements httpapi.StrictServerInterface (RF-26): it opens the assignment of an entry.
func (h *Handler) TakeAssignLock(ctx context.Context, req httpapi.TakeAssignLockRequestObject) (httpapi.TakeAssignLockResponseObject, error) {
	lock, err := h.svc.Take(ctx, req.Id, tokenOf(req.Params.XAssignLock))
	switch {
	case errors.Is(err, application.ErrEntryNotFound):
		return httpapi.TakeAssignLock404ApplicationProblemPlusJSONResponse{
			NotFoundApplicationProblemPlusJSONResponse: httpapi.NotFoundApplicationProblemPlusJSONResponse(notFoundProblem()),
		}, nil
	case errors.Is(err, application.ErrAssigning):
		return httpapi.TakeAssignLock409ApplicationProblemPlusJSONResponse{
			AssignConflictApplicationProblemPlusJSONResponse: conflict(httpapi.AssignProblemCode("assigning"), "The entry is being assigned in another tab."),
		}, nil
	case errors.Is(err, application.ErrCopying):
		return httpapi.TakeAssignLock409ApplicationProblemPlusJSONResponse{
			AssignConflictApplicationProblemPlusJSONResponse: lockConflict(err),
		}, nil
	case err != nil:
		return nil, err
	}
	return httpapi.TakeAssignLock200JSONResponse{Token: lock.Token, ExpiresAt: lock.Until}, nil
}

// ReleaseAssignLock implements httpapi.StrictServerInterface (RF-26): it cancels the assignment.
func (h *Handler) ReleaseAssignLock(ctx context.Context, req httpapi.ReleaseAssignLockRequestObject) (httpapi.ReleaseAssignLockResponseObject, error) {
	err := h.svc.Release(ctx, req.Id, tokenOf(req.Params.XAssignLock))
	switch {
	case errors.Is(err, application.ErrEntryNotFound):
		return httpapi.ReleaseAssignLock404ApplicationProblemPlusJSONResponse{
			NotFoundApplicationProblemPlusJSONResponse: httpapi.NotFoundApplicationProblemPlusJSONResponse(notFoundProblem()),
		}, nil
	case err != nil:
		return nil, err
	}
	return httpapi.ReleaseAssignLock204Response{}, nil
}

// GetAssignSuggestions implements httpapi.StrictServerInterface (RF-20, RF-23).
func (h *Handler) GetAssignSuggestions(ctx context.Context, req httpapi.GetAssignSuggestionsRequestObject) (httpapi.GetAssignSuggestionsResponseObject, error) {
	s, err := h.svc.Suggest(ctx, req.Id, tokenOf(req.Params.XAssignLock))
	switch {
	case errors.Is(err, application.ErrEntryNotFound):
		return httpapi.GetAssignSuggestions404ApplicationProblemPlusJSONResponse{
			NotFoundApplicationProblemPlusJSONResponse: httpapi.NotFoundApplicationProblemPlusJSONResponse(notFoundProblem()),
		}, nil
	case isLockError(err):
		return httpapi.GetAssignSuggestions409ApplicationProblemPlusJSONResponse{
			AssignConflictApplicationProblemPlusJSONResponse: lockConflict(err),
		}, nil
	case err != nil:
		return nil, err
	}
	return httpapi.GetAssignSuggestions200JSONResponse(suggestionsToAPI(s)), nil
}

// PlanAssign implements httpapi.StrictServerInterface (RF-24, RF-25): the preview of the choices.
func (h *Handler) PlanAssign(ctx context.Context, req httpapi.PlanAssignRequestObject) (httpapi.PlanAssignResponseObject, error) {
	choices, ok := requestOf(req.Body)
	if !ok {
		return httpapi.PlanAssign400ApplicationProblemPlusJSONResponse{
			BadRequestApplicationProblemPlusJSONResponse: httpapi.BadRequestApplicationProblemPlusJSONResponse(badRequest()),
		}, nil
	}
	plan, err := h.svc.Plan(ctx, req.Id, tokenOf(req.Params.XAssignLock), choices)
	switch {
	case errors.Is(err, application.ErrEntryNotFound):
		return httpapi.PlanAssign404ApplicationProblemPlusJSONResponse{
			NotFoundApplicationProblemPlusJSONResponse: httpapi.NotFoundApplicationProblemPlusJSONResponse(notFoundProblem()),
		}, nil
	case isLockError(err):
		return httpapi.PlanAssign409ApplicationProblemPlusJSONResponse{
			AssignConflictApplicationProblemPlusJSONResponse: lockConflict(err),
		}, nil
	case err != nil:
		return nil, err
	}
	return httpapi.PlanAssign200JSONResponse(planToAPI(plan)), nil
}

// CommitAssign implements httpapi.StrictServerInterface (RF-25, RF-26): it stores the assignment.
func (h *Handler) CommitAssign(ctx context.Context, req httpapi.CommitAssignRequestObject) (httpapi.CommitAssignResponseObject, error) {
	choices, ok := requestOf(req.Body)
	if !ok {
		return httpapi.CommitAssign400ApplicationProblemPlusJSONResponse{
			BadRequestApplicationProblemPlusJSONResponse: httpapi.BadRequestApplicationProblemPlusJSONResponse(badRequest()),
		}, nil
	}
	res, err := h.svc.Commit(ctx, req.Id, tokenOf(req.Params.XAssignLock), choices)
	var invalid *application.PlanInvalidError
	switch {
	case errors.Is(err, application.ErrEntryNotFound):
		return httpapi.CommitAssign404ApplicationProblemPlusJSONResponse{
			NotFoundApplicationProblemPlusJSONResponse: httpapi.NotFoundApplicationProblemPlusJSONResponse(notFoundProblem()),
		}, nil
	case isLockError(err):
		return httpapi.CommitAssign409ApplicationProblemPlusJSONResponse{
			AssignConflictApplicationProblemPlusJSONResponse: lockConflict(err),
		}, nil
	case errors.Is(err, application.ErrTargetTaken):
		h.log.WarnContext(ctx, "assignment refused: a target was taken", "entry", req.Id, "error", err)
		return httpapi.CommitAssign409ApplicationProblemPlusJSONResponse{
			AssignConflictApplicationProblemPlusJSONResponse: conflict(httpapi.AssignProblemCodePlanChanged, "A file took a place that the plan needed. Nothing was changed; check the plan again."),
		}, nil
	case errors.As(err, &invalid):
		return httpapi.CommitAssign422JSONResponse{AssignInvalidJSONResponse: httpapi.AssignInvalidJSONResponse(planToAPI(invalid.Plan))}, nil
	case err != nil:
		return nil, err
	}
	return httpapi.CommitAssign200JSONResponse(resultToAPI(res)), nil
}

// SearchTitles implements httpapi.StrictServerInterface (RF-21): the titles to assign to.
func (h *Handler) SearchTitles(ctx context.Context, req httpapi.SearchTitlesRequestObject) (httpapi.SearchTitlesResponseObject, error) {
	year := 0
	if req.Params.Year != nil {
		year = *req.Params.Year
	}
	res, err := h.svc.Search(ctx, req.Params.Query, year)
	switch {
	case errors.Is(err, application.ErrInvalidQuery):
		return httpapi.SearchTitles400ApplicationProblemPlusJSONResponse{
			BadRequestApplicationProblemPlusJSONResponse: httpapi.BadRequestApplicationProblemPlusJSONResponse(badRequest()),
		}, nil
	case err != nil:
		return nil, err
	}
	return httpapi.SearchTitles200JSONResponse(httpapi.TitleSearch{
		TmdbAvailable: res.TMDBAvailable,
		Library:       hitsToAPI(res.Library),
		Tmdb:          hitsToAPI(res.TMDB),
	}), nil
}

// tokenOf returns the lock token a call sent, or "" when it sent none.
func tokenOf(token *httpapi.AssignLockToken) string {
	if token == nil {
		return ""
	}
	return *token
}

// isLockError reports whether err says that the assignment lock does not allow the call.
func isLockError(err error) bool {
	return errors.Is(err, application.ErrAssigning) || errors.Is(err, application.ErrLockLost) || errors.Is(err, application.ErrCopying)
}

// lockConflict is the 409 of a call that the lock does not allow, or that a file still being copied
// does not allow (RF-27).
func lockConflict(err error) httpapi.AssignConflictApplicationProblemPlusJSONResponse {
	if errors.Is(err, application.ErrAssigning) {
		return conflict(httpapi.AssignProblemCode("assigning"), "The entry is being assigned in another tab.")
	}
	if errors.Is(err, application.ErrCopying) {
		return conflict(httpapi.AssignProblemCode("copying"), "A file of the entry is still being copied. Nothing was changed.")
	}
	return conflict(httpapi.AssignProblemCode("lock_lost"), "This tab does not hold the assignment lock any more. Open the assignment again.")
}

// conflict builds the 409 of an assignment with its stable code.
func conflict(code httpapi.AssignProblemCode, detail string) httpapi.AssignConflictApplicationProblemPlusJSONResponse {
	return httpapi.AssignConflictApplicationProblemPlusJSONResponse(httpapi.AssignProblem{
		Code: code, Status: http.StatusConflict, Title: "Conflict", Detail: &detail,
	})
}

// notFoundProblem is the RFC 9457 problem of an entry that is not in No asignados.
func notFoundProblem() httpapi.Problem {
	return httpapi.Problem{Status: http.StatusNotFound, Title: "Not Found"}
}

// badRequest is the 400 of a request whose body or query does not name a valid assignment.
func badRequest() httpapi.Problem {
	detail := "The request does not name a valid assignment."
	return httpapi.Problem{Status: http.StatusBadRequest, Title: "Bad Request", Detail: &detail}
}
