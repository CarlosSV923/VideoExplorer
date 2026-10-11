// Package http adapts the ingestion use cases to the generated HTTP contract.
package http

import (
	"context"
	"errors"
	"log/slog"
	"net/http"

	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/ingestion/application"
	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/ingestion/domain"
	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/platform/httpapi"
)

// Handler implements the batch operations of httpapi.StrictServerInterface.
// The uploads themselves go through the tus endpoint, not through here.
type Handler struct {
	svc *application.Service
	log *slog.Logger
}

// NewHandler builds the handler over the ingestion service. log may be nil.
func NewHandler(svc *application.Service, log *slog.Logger) *Handler {
	if log == nil {
		log = slog.New(slog.DiscardHandler)
	}
	return &Handler{svc: svc, log: log}
}

// CreateBatch implements httpapi.StrictServerInterface. It answers 400 for a
// relation outside RF-03 or a name that cannot be a folder name.
func (h *Handler) CreateBatch(ctx context.Context, req httpapi.CreateBatchRequestObject) (httpapi.CreateBatchResponseObject, error) {
	if req.Body == nil {
		return httpapi.CreateBatch400ApplicationProblemPlusJSONResponse{
			BadRequestApplicationProblemPlusJSONResponse: httpapi.BadRequestApplicationProblemPlusJSONResponse(
				problem(http.StatusBadRequest, "Bad Request", "The request needs a JSON body with a relation.")),
		}, nil
	}
	var name string
	if req.Body.Name != nil {
		name = *req.Body.Name
	}
	batch, err := h.svc.CreateBatch(ctx, name, domain.Relation(req.Body.Relation))
	if errors.Is(err, domain.ErrInvalidBatch) {
		return httpapi.CreateBatch400ApplicationProblemPlusJSONResponse{
			BadRequestApplicationProblemPlusJSONResponse: httpapi.BadRequestApplicationProblemPlusJSONResponse(
				problem(http.StatusBadRequest, "Bad Request", "The relation must be parts, episodes or none, and the name must be usable as a folder name.")),
		}, nil
	}
	if err != nil {
		return nil, err
	}
	h.log.DebugContext(ctx, "batch created", "batch", batch.ID, "relation", batch.Relation)
	return httpapi.CreateBatch201JSONResponse(toAPI(batch)), nil
}

func toAPI(b *domain.Batch) httpapi.Batch {
	return httpapi.Batch{
		Id:        string(b.ID),
		Name:      b.Name,
		Relation:  httpapi.BatchRelation(b.Relation),
		CreatedAt: b.CreatedAt,
	}
}

func problem(status int, title, detail string) httpapi.Problem {
	return httpapi.Problem{Status: status, Title: title, Detail: &detail}
}
