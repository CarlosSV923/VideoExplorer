package http

import (
	"context"
	"errors"
	"log/slog"

	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/catalog/application"
	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/catalog/domain"
	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/platform/httpapi"
)

// LibraryHandler implements the reads of the title pages (RF-40, RF-43) of httpapi.StrictServerInterface.
type LibraryHandler struct {
	views *application.LibraryViews
	log   *slog.Logger
}

// NewLibraryHandler builds the handler over the library reads. log may be nil.
func NewLibraryHandler(views *application.LibraryViews, log *slog.Logger) *LibraryHandler {
	if log == nil {
		log = slog.New(slog.DiscardHandler)
	}
	return &LibraryHandler{views: views, log: log}
}

// ListLibraryTitles implements httpapi.StrictServerInterface (RF-40): the titles of the library.
func (h *LibraryHandler) ListLibraryTitles(ctx context.Context, _ httpapi.ListLibraryTitlesRequestObject) (httpapi.ListLibraryTitlesResponseObject, error) {
	titles, err := h.views.Titles(ctx)
	if err != nil {
		return nil, err
	}
	out := httpapi.LibraryTitles{Titles: make([]httpapi.LibraryTitleSummary, 0, len(titles))}
	for _, t := range titles {
		out.Titles = append(out.Titles, summaryToAPI(t))
	}
	return httpapi.ListLibraryTitles200JSONResponse(out), nil
}

// GetLibraryTitle implements httpapi.StrictServerInterface (RF-43): a title with its files.
func (h *LibraryHandler) GetLibraryTitle(ctx context.Context, req httpapi.GetLibraryTitleRequestObject) (httpapi.GetLibraryTitleResponseObject, error) {
	t, err := h.views.Title(ctx, req.Id)
	switch {
	case errors.Is(err, application.ErrTitleNotFound):
		return httpapi.GetLibraryTitle404ApplicationProblemPlusJSONResponse{
			NotFoundApplicationProblemPlusJSONResponse: httpapi.NotFoundApplicationProblemPlusJSONResponse(notFoundProblem()),
		}, nil
	case err != nil:
		return nil, err
	}
	return httpapi.GetLibraryTitle200JSONResponse(detailToAPI(t)), nil
}

// summaryToAPI returns the contract's view of a title in the list.
func summaryToAPI(t application.TitleSummary) httpapi.LibraryTitleSummary {
	return httpapi.LibraryTitleSummary{
		Id: t.ID, Kind: kindToAPI(t.Kind), Title: t.Title, Year: t.Year, TmdbId: tmdbIDToAPI(t.TMDBID), IdTag: t.IDTag,
	}
}

// detailToAPI returns the contract's view of a title with its videos.
func detailToAPI(t application.TitleDetail) httpapi.LibraryTitle {
	out := httpapi.LibraryTitle{
		Id: t.ID, Kind: kindToAPI(t.Kind), Title: t.Title, DiscTitle: t.DiscTitle, Year: t.Year,
		TmdbId: tmdbIDToAPI(t.TMDBID), IdTag: t.IDTag, Videos: make([]httpapi.LibraryVideo, 0, len(t.Videos)),
	}
	if t.Poster != "" {
		poster := t.Poster
		out.Poster = &poster
	}
	for _, v := range t.Videos {
		out.Videos = append(out.Videos, videoToAPI(v, t.Kind))
	}
	return out
}

// videoToAPI returns the contract's view of a video and its subtitles.
func videoToAPI(v application.VideoView, kind domain.TitleKind) httpapi.LibraryVideo {
	out := httpapi.LibraryVideo{
		Id: v.ID, Path: v.Rel, Size: v.Size, Version: v.Version, Part: v.Place.Part,
		Subtitles: make([]httpapi.LibrarySubtitle, 0, len(v.Subtitles)),
	}
	if kind == domain.KindSeries && v.Place.Episode {
		season, chapter, last := v.Place.Season, v.Place.First, v.Place.Last
		out.Season, out.Chapter, out.LastChapter = &season, &chapter, &last
		if v.ChapterTitle != "" {
			title := v.ChapterTitle
			out.ChapterTitle = &title
		}
	}
	for _, s := range v.Subtitles {
		out.Subtitles = append(out.Subtitles, httpapi.LibrarySubtitle{
			Id: s.ID, Path: s.Rel, Language: s.Language, Forced: s.Forced, Sdh: s.SDH,
		})
	}
	return out
}

// kindToAPI returns the contract's kind of a title.
func kindToAPI(k domain.TitleKind) httpapi.TitleKind {
	return httpapi.TitleKind(k)
}

// tmdbIDToAPI returns the TMDB id of a linked title, or nil for a free title.
func tmdbIDToAPI(id int64) *int64 {
	if id == 0 {
		return nil
	}
	return &id
}
