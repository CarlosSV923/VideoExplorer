package http

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"unicode"

	openapi_types "github.com/oapi-codegen/runtime/types"
	"golang.org/x/text/unicode/norm"

	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/catalog/application"
	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/catalog/domain"
	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/platform/httpapi"
	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/platform/httpx"
	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/platform/zipstream"
)

// LibraryHandler implements the reads of the title pages (RF-40, RF-43) of httpapi.StrictServerInterface.
type LibraryHandler struct {
	views     *application.LibraryViews
	edits     *application.Edits
	technical *application.LibraryTechnical
	downloads *application.LibraryDownloads
	log       *slog.Logger
}

// NewLibraryHandler builds the handler over the library reads and the edits of a title (RF-44). log may be nil.
func NewLibraryHandler(views *application.LibraryViews, edits *application.Edits, technical *application.LibraryTechnical, log *slog.Logger, downloads ...*application.LibraryDownloads) *LibraryHandler {
	if log == nil {
		log = slog.New(slog.DiscardHandler)
	}
	var d *application.LibraryDownloads
	if len(downloads) > 0 {
		d = downloads[0]
	}
	return &LibraryHandler{views: views, edits: edits, technical: technical, downloads: d, log: log}
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

// GetLibraryFileTechnical implements the cache-backed technical read of one
// live video in a title (RF-44).
func (h *LibraryHandler) GetLibraryFileTechnical(ctx context.Context, req httpapi.GetLibraryFileTechnicalRequestObject) (httpapi.GetLibraryFileTechnicalResponseObject, error) {
	t, err := h.technical.File(ctx, req.Id, req.FileId)
	switch {
	case errors.Is(err, application.ErrTitleNotFound), errors.Is(err, application.ErrLibraryFileNotFound):
		return httpapi.GetLibraryFileTechnical404ApplicationProblemPlusJSONResponse{
			NotFoundApplicationProblemPlusJSONResponse: httpapi.NotFoundApplicationProblemPlusJSONResponse(notFoundProblem()),
		}, nil
	case err != nil:
		return nil, err
	}
	return httpapi.GetLibraryFileTechnical200JSONResponse(technicalToAPI(t)), nil
}

// DownloadLibraryTitle implements the streamed complete-title download (RF-44).
func (h *LibraryHandler) DownloadLibraryTitle(ctx context.Context, req httpapi.DownloadLibraryTitleRequestObject) (httpapi.DownloadLibraryTitleResponseObject, error) {
	if h.downloads == nil {
		return nil, errors.New("library downloads are not configured")
	}
	d, err := h.downloads.Title(ctx, req.Id)
	if errors.Is(err, application.ErrTitleNotFound) || errors.Is(err, application.ErrDownloadUnavailable) {
		return httpapi.DownloadLibraryTitle404ApplicationProblemPlusJSONResponse{NotFoundApplicationProblemPlusJSONResponse: httpapi.NotFoundApplicationProblemPlusJSONResponse(notFoundProblem())}, nil
	}
	if err != nil {
		return nil, err
	}
	return h.download(ctx, d), nil
}

// DownloadLibrarySeason implements the streamed season download (RF-44).
func (h *LibraryHandler) DownloadLibrarySeason(ctx context.Context, req httpapi.DownloadLibrarySeasonRequestObject) (httpapi.DownloadLibrarySeasonResponseObject, error) {
	if h.downloads == nil {
		return nil, errors.New("library downloads are not configured")
	}
	d, err := h.downloads.Season(ctx, req.Id, req.Season)
	if errors.Is(err, application.ErrTitleNotFound) || errors.Is(err, application.ErrLibraryFileNotFound) || errors.Is(err, application.ErrDownloadUnavailable) {
		return httpapi.DownloadLibrarySeason404ApplicationProblemPlusJSONResponse{NotFoundApplicationProblemPlusJSONResponse: httpapi.NotFoundApplicationProblemPlusJSONResponse(notFoundProblem())}, nil
	}
	if err != nil {
		return nil, err
	}
	return h.download(ctx, d), nil
}

// DownloadLibraryFile implements a Range-capable single library file download (RF-44).
func (h *LibraryHandler) DownloadLibraryFile(ctx context.Context, req httpapi.DownloadLibraryFileRequestObject) (httpapi.DownloadLibraryFileResponseObject, error) {
	if h.downloads == nil {
		return nil, errors.New("library downloads are not configured")
	}
	d, err := h.downloads.File(ctx, req.Id, req.FileId)
	if errors.Is(err, application.ErrTitleNotFound) || errors.Is(err, application.ErrLibraryFileNotFound) || errors.Is(err, application.ErrDownloadUnavailable) {
		return httpapi.DownloadLibraryFile404ApplicationProblemPlusJSONResponse{NotFoundApplicationProblemPlusJSONResponse: httpapi.NotFoundApplicationProblemPlusJSONResponse(notFoundProblem())}, nil
	}
	if err != nil {
		return nil, err
	}
	return h.download(ctx, d), nil
}

// downloadResponse is the streaming response for all library downloads.
type downloadResponse struct {
	d     application.Download
	r     *http.Request
	files application.DownloadFiles
	log   *slog.Logger
}

func (h *LibraryHandler) download(ctx context.Context, d application.Download) downloadResponse {
	r, _ := httpx.Request(ctx)
	var files application.DownloadFiles
	if h.downloads != nil {
		files = h.downloads.Files()
	}
	return downloadResponse{d: d, r: r, files: files, log: h.log}
}

func (d downloadResponse) VisitDownloadLibraryTitleResponse(w http.ResponseWriter) error {
	return d.visit(w)
}

func (d downloadResponse) VisitDownloadLibrarySeasonResponse(w http.ResponseWriter) error {
	return d.visit(w)
}

func (d downloadResponse) VisitDownloadLibraryFileResponse(w http.ResponseWriter) error {
	return d.visit(w)
}

func (d downloadResponse) VisitDownloadUnassignedEntryResponse(w http.ResponseWriter) error {
	return d.visit(w)
}

func (d downloadResponse) VisitDownloadUnassignedFileResponse(w http.ResponseWriter) error {
	return d.visit(w)
}

func (d downloadResponse) visit(w http.ResponseWriter) error {
	if d.files == nil {
		return errors.New("download: files are not configured")
	}
	w.Header().Set("Content-Disposition", contentDisposition(d.d.Name))
	w.Header().Set("Cache-Control", "no-store")
	if d.d.File != nil {
		return d.serveFile(w)
	}
	entries := make([]zipstream.Entry, 0, len(d.d.Entries))
	for _, e := range d.d.Entries {
		e := e
		entries = append(entries, zipstream.Entry{Name: e.Name, Size: e.Size, Modified: e.Modified, Open: func() (io.ReadCloser, error) {
			return d.files.Open(e.Path)
		}})
	}
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Length", strconv.FormatInt(zipstream.Size(entries), 10))
	w.WriteHeader(http.StatusOK)
	if err := zipstream.Write(w, entries); err != nil {
		if d.log != nil {
			d.log.Warn("zip download interrupted", "name", d.d.Name, "error", err)
		}
	}
	return nil
}

func (d downloadResponse) serveFile(w http.ResponseWriter) error {
	if d.r == nil {
		return errors.New("download: request missing from context (httpx.WithRequest)")
	}
	if d.files == nil {
		return errors.New("download: files are not configured")
	}
	f, err := d.files.Open(d.d.File.Path)
	if err != nil {
		return fmt.Errorf("open %s: %w", d.d.Name, err)
	}
	defer f.Close()
	w.Header().Set("Content-Type", "application/octet-stream")
	http.ServeContent(w, d.r, "", d.d.File.Modified, f)
	return nil
}

// contentDisposition uses an ASCII fallback plus RFC 6266 filename* for UTF-8 names.
func contentDisposition(name string) string {
	ascii := asciiName(name)
	if ascii == name {
		return mime.FormatMediaType("attachment", map[string]string{"filename": name})
	}
	return mime.FormatMediaType("attachment", map[string]string{"filename": ascii}) + "; filename*=UTF-8''" + url.PathEscape(name)
}

func asciiName(name string) string {
	var b strings.Builder
	for _, r := range norm.NFD.String(name) {
		switch {
		case unicode.Is(unicode.Mn, r):
		case r < 0x20 || r == '"' || r == '\\' || r == 0x7f:
		case r < 0x80:
			b.WriteRune(r)
		default:
			b.WriteRune('_')
		}
	}
	return b.String()
}

// summaryToAPI returns the contract's view of a title in the list.
func summaryToAPI(t application.TitleSummary) httpapi.LibraryTitleSummary {
	out := httpapi.LibraryTitleSummary{Id: t.ID, Kind: kindToAPI(t.Kind), Title: t.Title, Year: t.Year, TmdbId: tmdbIDToAPI(t.TMDBID), IdTag: t.IDTag}
	if t.MissingEpisodes != nil {
		out.MissingEpisodes = t.MissingEpisodes
	}
	return out
}

// detailToAPI returns the contract's view of a title with its videos.
func detailToAPI(t application.TitleDetail) httpapi.LibraryTitle {
	out := httpapi.LibraryTitle{
		Id: t.ID, Kind: kindToAPI(t.Kind), Title: t.Title, DiscTitle: t.DiscTitle, Year: t.Year,
		TmdbId: tmdbIDToAPI(t.TMDBID), IdTag: t.IDTag, OriginalTitle: t.OriginalTitle,
		Overview: t.Overview, Runtime: t.Runtime, Genres: append([]string(nil), t.Genres...), Seasons: make([]httpapi.LibrarySeason, 0, len(t.Seasons)), Videos: make([]httpapi.LibraryVideo, 0, len(t.Videos)),
	}
	if t.MissingEpisodes != nil {
		out.MissingEpisodes = t.MissingEpisodes
	}
	if t.Poster != "" {
		poster := imageURL("poster", t.Poster)
		out.Poster = &poster
	}
	if t.Backdrop != "" {
		backdrop := imageURL("backdrop", t.Backdrop)
		out.Backdrop = &backdrop
	}
	if t.Logo != "" {
		logo := imageURL("logo", t.Logo)
		out.Logo = &logo
	}
	if t.Collection != nil {
		out.Collection = collectionToAPI(*t.Collection)
	}
	for _, s := range t.Seasons {
		season := httpapi.LibrarySeason{Number: s.Number, Episodes: make([]httpapi.LibraryEpisode, 0, len(s.Episodes))}
		for _, e := range s.Episodes {
			episode := httpapi.LibraryEpisode{Id: e.ID, Number: e.Number, Title: e.Title, Overview: e.Overview, Status: httpapi.LibraryEpisodeStatus(e.Status), Files: make([]httpapi.LibraryVideo, 0, len(e.Files))}
			if e.Still != "" {
				still := imageURL("still", e.Still)
				episode.Thumbnail = &still
			}
			if !e.AirDate.IsZero() {
				date := openapi_types.Date{Time: e.AirDate}
				episode.AirDate = &date
			}
			for _, file := range e.Files {
				episode.Files = append(episode.Files, videoToAPI(file, t.Kind))
			}
			season.Episodes = append(season.Episodes, episode)
		}
		out.Seasons = append(out.Seasons, season)
	}
	for _, v := range t.Videos {
		out.Videos = append(out.Videos, videoToAPI(v, t.Kind))
	}
	return out
}

func collectionToAPI(c application.CollectionView) *httpapi.LibraryCollection {
	out := &httpapi.LibraryCollection{Id: c.ID, Name: c.Name, Overview: c.Overview, Movies: make([]httpapi.LibraryCollectionMovie, 0, len(c.Movies))}
	if c.Poster != "" {
		poster := imageURL("poster", c.Poster)
		out.Poster = &poster
	}
	if c.Backdrop != "" {
		backdrop := imageURL("backdrop", c.Backdrop)
		out.Backdrop = &backdrop
	}
	for _, movie := range c.Movies {
		item := httpapi.LibraryCollectionMovie{Id: movie.ID, Title: movie.Title, OriginalTitle: movie.OriginalTitle, Year: movie.Year, InLibrary: movie.LibraryTitleID != 0}
		if movie.Poster != "" {
			poster := imageURL("poster", movie.Poster)
			item.Poster = &poster
		}
		if movie.LibraryTitleID != 0 {
			id := movie.LibraryTitleID
			item.LibraryTitleId = &id
		}
		out.Movies = append(out.Movies, item)
	}
	return out
}

func imageURL(kind, file string) string { return "/api/images/" + kind + "/" + file }

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

func technicalToAPI(t application.TechnicalFile) httpapi.LibraryFileTechnical {
	out := httpapi.LibraryFileTechnical{
		Path: t.Path, Size: t.Size, Readable: t.Readable, Duration: t.Duration.Seconds(),
		Video: httpapi.TechnicalVideo{Width: t.Video.Width, Height: t.Video.Height, Hdr: t.Video.HDR, Codec: t.Video.Codec},
		Audio: make([]httpapi.TechnicalAudio, 0, len(t.Audio)), Subtitles: make([]httpapi.TechnicalSubtitle, 0, len(t.Subtitles)),
	}
	for _, a := range t.Audio {
		out.Audio = append(out.Audio, httpapi.TechnicalAudio{Codec: a.Codec, Channels: a.Channels, Language: a.Language, Title: a.Title, Default: a.Default})
	}
	for _, s := range t.Subtitles {
		out.Subtitles = append(out.Subtitles, httpapi.TechnicalSubtitle{Codec: s.Codec, Language: s.Language, Title: s.Title, Forced: s.Forced, Sdh: s.SDH, Default: s.Default})
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
