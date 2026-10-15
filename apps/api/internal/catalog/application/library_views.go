package application

import (
	"context"
	"errors"
	"log/slog"
	"strings"

	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/catalog/domain"
)

// ErrTitleNotFound means there is no title of the library with that id (or it is only in the trash).
var ErrTitleNotFound = errors.New("the title is not in the library")

// TitleSummary is a title of the library as the list shows it (RF-40, RF-43).
type TitleSummary struct {
	ID    int64
	Kind  domain.TitleKind
	Title string
	Year  int
	// TMDBID is 0 for a free title.
	TMDBID int64
	IDTag  bool
}

// TitleDetail is a title with its videos, as the page of the title shows them (RF-43).
type TitleDetail struct {
	TitleSummary
	DiscTitle string
	// Poster is the file name of the poster, "" when TMDB does not give one (spec §6).
	Poster string
	Videos []VideoView
}

// VideoView is a video of a title and the subtitles that go with it.
type VideoView struct {
	ID   int64
	Rel  string
	Size int64
	// Place is the part of a movie (0 for the whole movie) or the chapters of a series.
	Place   domain.Place
	Version string
	// ChapterTitle is the title of a chapter of a free series, read from its name; "" otherwise.
	ChapterTitle string
	Subtitles    []SubtitleView
}

// SubtitleView is a subtitle that goes with a video (RF-23).
type SubtitleView struct {
	ID       int64
	Rel      string
	Language string
	Forced   bool
	SDH      bool
}

// LibraryViews answers what the title pages show: the titles of the library and one of them with its
// files (RF-40, RF-43). It changes nothing.
type LibraryViews struct {
	repo LibraryRepository
	tmdb TMDB
	log  *slog.Logger
}

// NewLibraryViews builds the reads over the registry and TMDB. log may be nil.
func NewLibraryViews(repo LibraryRepository, tmdb TMDB, log *slog.Logger) *LibraryViews {
	if log == nil {
		log = slog.New(slog.DiscardHandler)
	}
	return &LibraryViews{repo: repo, tmdb: tmdb, log: log}
}

// Titles returns the titles of the library: those with at least one file out of the trash (RF-30), by id.
func (v *LibraryViews) Titles(ctx context.Context) ([]TitleSummary, error) {
	live, _, err := classifyTitles(ctx, v.repo)
	if err != nil {
		return nil, err
	}
	out := make([]TitleSummary, 0, len(live))
	for _, t := range live {
		out = append(out, summaryOf(t))
	}
	return out, nil
}

// Title returns a title of the library with its files that are out of the trash (RF-43). The poster comes
// from TMDB through its cache when TMDB is configured; a failure there leaves it out and is logged.
func (v *LibraryViews) Title(ctx context.Context, id int64) (TitleDetail, error) {
	live, _, err := classifyTitles(ctx, v.repo)
	if err != nil {
		return TitleDetail{}, err
	}
	var title domain.LibraryTitle
	found := false
	for _, t := range live {
		if t.ID == id {
			title, found = t, true
			break
		}
	}
	if !found {
		return TitleDetail{}, ErrTitleNotFound
	}
	files, err := v.repo.Files(ctx, id)
	if err != nil {
		return TitleDetail{}, err
	}
	out := TitleDetail{TitleSummary: summaryOf(title), DiscTitle: title.DiscTitle}
	out.Videos = videoViews(title, files)
	if title.TMDBID > 0 && v.tmdb != nil && v.tmdb.Configured() {
		t, err := v.tmdb.Title(ctx, title.Kind, title.TMDBID)
		if err != nil {
			v.log.WarnContext(ctx, "poster of a title not read from TMDB", "title", id, "error", err)
		} else {
			out.Poster = t.PosterFile
		}
	}
	return out, nil
}

// summaryOf returns the list's view of a title.
func summaryOf(t domain.LibraryTitle) TitleSummary {
	return TitleSummary{ID: t.ID, Kind: t.Kind, Title: t.Title, Year: t.Year, TMDBID: t.TMDBID, IDTag: t.IDTag}
}

// videoViews returns the live videos of a title in their order, each with its live subtitles.
func videoViews(title domain.LibraryTitle, files []domain.LibraryFile) []VideoView {
	subs := map[string][]SubtitleView{}
	for _, f := range files {
		if f.Kind != domain.FileSubtitle || !f.Live() {
			continue
		}
		key := strings.ToLower(f.VideoRel)
		subs[key] = append(subs[key], SubtitleView{ID: f.ID, Rel: f.Rel, Language: f.Language, Forced: f.Forced, SDH: f.SDH})
	}
	var out []VideoView
	for _, f := range files {
		if f.Kind != domain.FileVideo || !f.Live() {
			continue
		}
		v := VideoView{ID: f.ID, Rel: f.Rel, Size: f.Size, Place: f.Place, Version: f.Version, Subtitles: subs[strings.ToLower(f.Rel)]}
		if title.Kind == domain.KindSeries && title.TMDBID == 0 && f.Place.Episode {
			v.ChapterTitle = domain.ChapterTitleFromName(f.Rel, f.Place.Season, f.Place.First, f.Place.Last, f.Version)
		}
		out = append(out, v)
	}
	return out
}

// anyLive reports whether one of the files is out of the trash.
func anyLive(files []domain.LibraryFile) bool {
	for _, f := range files {
		if f.Live() {
			return true
		}
	}
	return false
}
