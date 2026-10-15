package http

import (
	"context"
	"errors"

	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/catalog/application"
	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/platform/httpapi"
)

// MoveTitleToUnassigned implements httpapi.StrictServerInterface (RF-44): a title, a season or a file goes to No asignados.
func (h *LibraryHandler) MoveTitleToUnassigned(ctx context.Context, req httpapi.MoveTitleToUnassignedRequestObject) (httpapi.MoveTitleToUnassignedResponseObject, error) {
	s, ok := scopeOf(req.Body)
	if !ok {
		return httpapi.MoveTitleToUnassigned400ApplicationProblemPlusJSONResponse{
			BadRequestApplicationProblemPlusJSONResponse: httpapi.BadRequestApplicationProblemPlusJSONResponse(badRequest()),
		}, nil
	}
	err := h.edits.MoveToUnassigned(ctx, req.Id, s)
	if notFound(err) {
		return httpapi.MoveTitleToUnassigned404ApplicationProblemPlusJSONResponse{
			NotFoundApplicationProblemPlusJSONResponse: httpapi.NotFoundApplicationProblemPlusJSONResponse(notFoundProblem()),
		}, nil
	}
	if err != nil {
		return nil, err
	}
	return httpapi.MoveTitleToUnassigned204Response{}, nil
}

// TrashTitleScope implements httpapi.StrictServerInterface (RF-44, RF-30): a title, a season or a file goes to the trash.
func (h *LibraryHandler) TrashTitleScope(ctx context.Context, req httpapi.TrashTitleScopeRequestObject) (httpapi.TrashTitleScopeResponseObject, error) {
	s, ok := scopeOf(req.Body)
	if !ok {
		return httpapi.TrashTitleScope400ApplicationProblemPlusJSONResponse{
			BadRequestApplicationProblemPlusJSONResponse: httpapi.BadRequestApplicationProblemPlusJSONResponse(badRequest()),
		}, nil
	}
	err := h.edits.Trash(ctx, req.Id, s)
	if notFound(err) {
		return httpapi.TrashTitleScope404ApplicationProblemPlusJSONResponse{
			NotFoundApplicationProblemPlusJSONResponse: httpapi.NotFoundApplicationProblemPlusJSONResponse(notFoundProblem()),
		}, nil
	}
	if err != nil {
		return nil, err
	}
	return httpapi.TrashTitleScope204Response{}, nil
}

// notFound reports whether err says that the title, or what the scope names of it, is not in the library.
func notFound(err error) bool {
	return errors.Is(err, application.ErrTitleNotFound) || errors.Is(err, application.ErrScopeNotFound)
}

// scopeOf turns the body of a move or a trash into the application's scope. It answers false when the body is
// missing or names a season or a file without its number or id.
func scopeOf(body *httpapi.LibraryScope) (application.Scope, bool) {
	if body == nil {
		return application.Scope{}, false
	}
	switch body.Kind {
	case httpapi.LibraryScopeKind("title"):
		return application.Scope{Kind: application.ScopeTitle}, true
	case httpapi.LibraryScopeKind("season"):
		if body.Season == nil {
			return application.Scope{}, false
		}
		return application.Scope{Kind: application.ScopeSeason, Season: *body.Season}, true
	case httpapi.LibraryScopeKind("file"):
		if body.FileId == nil {
			return application.Scope{}, false
		}
		return application.Scope{Kind: application.ScopeFile, FileID: *body.FileId}, true
	default:
		return application.Scope{}, false
	}
}
