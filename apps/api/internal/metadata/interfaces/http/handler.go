// Package http adapts the metadata use cases to the generated HTTP contract.
// The operations here answer images: the bytes go out through http.ServeContent,
// so Range and conditional requests work and nothing is buffered in memory.
package http

import (
	"context"
	"errors"
	"log/slog"
	"net/http"

	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/metadata/application"
	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/metadata/domain"
	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/platform/httpapi"
	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/platform/httpx"
)

// Handler implements the image operation of httpapi.StrictServerInterface.
type Handler struct {
	images application.ImageStore
	log    *slog.Logger
}

// NewHandler builds the handler over the image store. log may be nil.
func NewHandler(images application.ImageStore, log *slog.Logger) *Handler {
	if log == nil {
		log = slog.New(slog.DiscardHandler)
	}
	return &Handler{images: images, log: log}
}

// GetImage implements httpapi.StrictServerInterface. It answers 400 for a name
// that is not an image of the app, 404 when the image is not cached and TMDB
// is not configured or does not have it, and 502 when TMDB fails.
func (h *Handler) GetImage(ctx context.Context, req httpapi.GetImageRequestObject) (httpapi.GetImageResponseObject, error) {
	ref, err := domain.NewImageRef(string(req.Kind), req.File)
	if err != nil {
		return httpapi.GetImage400ApplicationProblemPlusJSONResponse{
			BadRequestApplicationProblemPlusJSONResponse: httpapi.BadRequestApplicationProblemPlusJSONResponse(
				problem(http.StatusBadRequest, "Bad Request", "The image name is not valid.")),
		}, nil
	}

	img, err := h.images.Open(ctx, ref)
	switch {
	case errors.Is(err, domain.ErrNotConfigured):
		return httpapi.GetImage404ApplicationProblemPlusJSONResponse{
			NotFoundApplicationProblemPlusJSONResponse: httpapi.NotFoundApplicationProblemPlusJSONResponse(
				problem(http.StatusNotFound, "Not Found", "The image is not cached and TMDB is not configured.")),
		}, nil
	case errors.Is(err, domain.ErrNotFound):
		return httpapi.GetImage404ApplicationProblemPlusJSONResponse{
			NotFoundApplicationProblemPlusJSONResponse: httpapi.NotFoundApplicationProblemPlusJSONResponse(
				problem(http.StatusNotFound, "Not Found", "TMDB has no such image.")),
		}, nil
	case errors.Is(err, domain.ErrUpstream):
		h.log.WarnContext(ctx, "image unavailable", "kind", ref.Kind, "error", err)
		return httpapi.GetImage502ApplicationProblemPlusJSONResponse{
			BadGatewayApplicationProblemPlusJSONResponse: httpapi.BadGatewayApplicationProblemPlusJSONResponse(
				problem(http.StatusBadGateway, "Bad Gateway", "The image is not cached and TMDB could not provide it.")),
		}, nil
	case err != nil:
		return nil, err
	}

	r, ok := httpx.Request(ctx)
	if !ok {
		_ = img.File.Close()
		return nil, errors.New("image: request missing from context (httpx.WithRequest)")
	}
	return imageResponse{img: img, req: r, contentType: ref.ContentType()}, nil
}

// imageCacheControl is the exception of RNF-02 for /api/images/*: the URL of an
// image comes from a TMDB path that never changes, so the browser may keep it
// for 30 days. private keeps shared caches out. Only the 200 answers carry it:
// errors keep the no-store of the rest of /api/*.
const imageCacheControl = "private, max-age=2592000, immutable"

// imageResponse sends a cached image. It replaces the generated 200 types,
// which cannot seek: http.ServeContent needs the request and a seekable file.
type imageResponse struct {
	img         application.StoredImage
	req         *http.Request
	contentType string
}

// VisitGetImageResponse writes the image and closes the file. The cache header
// is set here, after the security middleware has set no-store, so it wins.
func (r imageResponse) VisitGetImageResponse(w http.ResponseWriter) error {
	defer r.img.File.Close()
	w.Header().Set("Content-Type", r.contentType)
	w.Header().Set("Cache-Control", imageCacheControl)
	http.ServeContent(w, r.req, "", r.img.ModTime, r.img.File)
	return nil
}

func problem(status int, title, detail string) httpapi.Problem {
	return httpapi.Problem{Status: status, Title: title, Detail: &detail}
}
