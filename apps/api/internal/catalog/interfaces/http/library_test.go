package http

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/catalog/application"
	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/catalog/domain"
	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/platform/httpapi"
)

// libraryRegistry is a registry with one series, one of whose chapters is in the trash.
type libraryRegistry struct{}

func (libraryRegistry) TitleByFolder(context.Context, string) (domain.LibraryTitle, bool, error) {
	return domain.LibraryTitle{}, false, nil
}

func (libraryRegistry) Titles(context.Context) ([]domain.LibraryTitle, error) {
	return []domain.LibraryTitle{{ID: 7, Kind: domain.KindSeries, Title: "Lizzie", DiscTitle: "Lizzie", Year: 1975, Folder: "TV Shows/Lizzie (1975)"}}, nil
}

func (libraryRegistry) Files(context.Context, int64) ([]domain.LibraryFile, error) {
	video := "TV Shows/Lizzie (1975)/Season 01/Lizzie (1975) - S01E01 - Pilot.mkv"
	return []domain.LibraryFile{
		{ID: 71, TitleID: 7, Kind: domain.FileVideo, Rel: video, Size: 10, Place: domain.Place{Episode: true, Season: 1, First: 1, Last: 1}},
		{ID: 72, TitleID: 7, Kind: domain.FileSubtitle, Rel: "TV Shows/Lizzie (1975)/Season 01/Lizzie (1975) - S01E01 - Pilot.es.srt", VideoRel: video, Language: "es"},
		{ID: 73, TitleID: 7, Kind: domain.FileVideo, Rel: "TV Shows/Lizzie (1975)/Season 01/old.mkv", Place: domain.Place{Episode: true, Season: 1, First: 2, Last: 2}, TrashEntryID: 4},
	}, nil
}

type richLibraryRegistry struct{}

func (richLibraryRegistry) TitleByFolder(context.Context, string) (domain.LibraryTitle, bool, error) {
	return domain.LibraryTitle{}, false, nil
}

func (richLibraryRegistry) Titles(context.Context) ([]domain.LibraryTitle, error) {
	return []domain.LibraryTitle{{ID: 7, Kind: domain.KindSeries, Title: "Show", DiscTitle: "Show", Year: 2020, TMDBID: 100, Folder: "TV Shows/Show (2020)"}}, nil
}

func (richLibraryRegistry) Files(context.Context, int64) ([]domain.LibraryFile, error) {
	return []domain.LibraryFile{{ID: 71, TitleID: 7, Kind: domain.FileVideo, Rel: "TV Shows/Show (2020)/Season 01/Show - S01E01.mkv", Size: 10, Place: domain.Place{Episode: true, Season: 1, First: 1, Last: 1}}}, nil
}

type richLibraryTMDB struct{ calls *int }

func (richLibraryTMDB) Configured() bool { return true }
func (richLibraryTMDB) SearchTitles(context.Context, domain.TitleKind, string, int) ([]application.TMDBTitle, error) {
	return nil, nil
}

func (richLibraryTMDB) Title(context.Context, domain.TitleKind, int64) (application.TMDBTitle, error) {
	return application.TMDBTitle{}, nil
}

func (richLibraryTMDB) Chapters(context.Context, int64, int, int, int) (application.TMDBChapters, error) {
	return application.TMDBChapters{}, nil
}

func (t richLibraryTMDB) Details(context.Context, domain.TitleKind, int64) (application.TitleMetadata, error) {
	if t.calls != nil {
		(*t.calls)++
	}
	return application.TitleMetadata{
		Title: "Show", OriginalTitle: "The Show", Overview: "A synopsis", Backdrop: "backdrop.jpg", Poster: "poster.jpg", Logo: "logo.png", Genres: []string{"Drama"},
		Seasons: []application.SeasonMetadata{{Number: 0}, {Number: 1, Episodes: []application.EpisodeMetadata{{ID: 11, Number: 1, Title: "Pilot", Overview: "First", Still: "still.jpg", AirDate: time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)}, {ID: 12, Number: 2, Title: "Second", AirDate: time.Date(2020, 1, 2, 0, 0, 0, 0, time.UTC)}}}},
	}, nil
}

func TestTheTitleListDoesNotAskTMDBForEachSeries(t *testing.T) {
	t.Parallel()
	calls := 0
	h := NewLibraryHandler(application.NewLibraryViews(richLibraryRegistry{}, richLibraryTMDB{calls: &calls}, nil), nil, nil, nil)
	got, err := h.ListLibraryTitles(t.Context(), httpapi.ListLibraryTitlesRequestObject{})
	if err != nil {
		t.Fatalf("ListLibraryTitles: %v", err)
	}
	response, ok := got.(httpapi.ListLibraryTitles200JSONResponse)
	if !ok {
		t.Fatalf("response = %T, want 200", got)
	}
	if len(response.Titles) != 1 || response.Titles[0].MissingEpisodes != nil {
		t.Fatalf("titles = %+v, want one title without an unknown count", response.Titles)
	}
	if calls != 0 {
		t.Fatalf("TMDB Details calls = %d, want 0", calls)
	}
}

func TestTheTitlePageAnswersTheVideosWithTheirChapters(t *testing.T) {
	t.Parallel()
	h := NewLibraryHandler(application.NewLibraryViews(libraryRegistry{}, nil, nil), nil, nil, nil)
	got, err := h.GetLibraryTitle(t.Context(), httpapi.GetLibraryTitleRequestObject{Id: 7})
	if err != nil {
		t.Fatalf("GetLibraryTitle: %v", err)
	}
	title, ok := got.(httpapi.GetLibraryTitle200JSONResponse)
	if !ok {
		t.Fatalf("response = %T, want 200", got)
	}
	if title.Id != 7 || title.TmdbId != nil || title.IdTag || title.Poster != nil {
		t.Errorf("the free title: %+v", title)
	}
	if len(title.Videos) != 1 {
		t.Fatalf("videos = %+v, want the live one only", title.Videos)
	}
	v := title.Videos[0]
	if v.Season == nil || *v.Season != 1 || v.Chapter == nil || *v.Chapter != 1 || v.LastChapter == nil || *v.LastChapter != 1 {
		t.Errorf("the chapter: %+v", v)
	}
	if v.ChapterTitle == nil || *v.ChapterTitle != "Pilot" {
		t.Errorf("the chapter title of a free series: %v", v.ChapterTitle)
	}
	if len(v.Subtitles) != 1 || v.Subtitles[0].Language != "es" {
		t.Errorf("the subtitles: %+v", v.Subtitles)
	}
}

func TestTheTitleHandlerAnswersTMDBDetailsAndEpisodeFiles(t *testing.T) {
	t.Parallel()
	h := NewLibraryHandler(application.NewLibraryViews(richLibraryRegistry{}, richLibraryTMDB{}, nil), nil, nil, nil)
	got, err := h.GetLibraryTitle(t.Context(), httpapi.GetLibraryTitleRequestObject{Id: 7})
	if err != nil {
		t.Fatalf("GetLibraryTitle: %v", err)
	}
	title, ok := got.(httpapi.GetLibraryTitle200JSONResponse)
	if !ok {
		t.Fatalf("response = %T, want 200", got)
	}
	if title.Title != "Show" || title.OriginalTitle != "The Show" || title.Overview != "A synopsis" || title.Backdrop == nil || *title.Backdrop != "/api/images/backdrop/backdrop.jpg" || title.Logo == nil || *title.Logo != "/api/images/logo/logo.png" {
		t.Errorf("TMDB details = %+v", title)
	}
	if len(title.Genres) != 1 || title.Genres[0] != "Drama" || len(title.Seasons) != 2 || title.Seasons[1].Episodes[0].Status != httpapi.Present || len(title.Seasons[1].Episodes[0].Files) != 1 || title.Seasons[1].Episodes[1].Status != httpapi.Missing {
		t.Errorf("seasons = %+v", title.Seasons)
	}
}

func TestATitleOutOfTheLibraryAnswersNotFound(t *testing.T) {
	t.Parallel()
	h := NewLibraryHandler(application.NewLibraryViews(libraryRegistry{}, nil, nil), nil, nil, nil)
	got, err := h.GetLibraryTitle(t.Context(), httpapi.GetLibraryTitleRequestObject{Id: 99})
	if err != nil {
		t.Fatalf("GetLibraryTitle: %v", err)
	}
	if _, ok := got.(httpapi.GetLibraryTitle404ApplicationProblemPlusJSONResponse); !ok {
		t.Errorf("response = %T, want 404", got)
	}
}

type technicalHTTPPaths struct{}

func (technicalHTTPPaths) LibraryPath(elem ...string) string {
	return "/library/" + strings.Join(elem, "/")
}

type technicalHTTPProber struct{}

func (technicalHTTPProber) Probe(context.Context, string) (application.TechnicalMedia, error) {
	return application.TechnicalMedia{
		Duration: time.Minute,
		Video:    application.TechnicalVideo{Width: 1280, Height: 720, HDR: true, Codec: "h264"},
		Audio:    []application.TechnicalAudio{{Codec: "aac", Channels: 2, Language: "es", Title: "Latino", Default: true}},
	}, nil
}

func TestTheTechnicalFileHandlerAnswersTheProbeAndPath(t *testing.T) {
	t.Parallel()
	repo := libraryRegistry{}
	technical := application.NewLibraryTechnical(repo, technicalHTTPPaths{}, technicalHTTPProber{}, nil)
	h := NewLibraryHandler(application.NewLibraryViews(repo, nil, nil), nil, technical, nil)

	got, err := h.GetLibraryFileTechnical(t.Context(), httpapi.GetLibraryFileTechnicalRequestObject{Id: 7, FileId: 71})
	if err != nil {
		t.Fatalf("GetLibraryFileTechnical: %v", err)
	}
	response, ok := got.(httpapi.GetLibraryFileTechnical200JSONResponse)
	if !ok {
		t.Fatalf("response = %T, want 200", got)
	}
	if response.Path != "TV Shows/Lizzie (1975)/Season 01/Lizzie (1975) - S01E01 - Pilot.mkv" || response.Size != 10 || !response.Readable {
		t.Errorf("file identity = %+v", response)
	}
	if response.Duration != 60 || response.Video.Width != 1280 || response.Video.Codec != "h264" || len(response.Audio) != 1 || response.Audio[0].Language != "es" {
		t.Errorf("technical response = %+v", response)
	}
}

func TestTheTechnicalFileHandlerAnswersNotFoundForAFileOfAnotherTitle(t *testing.T) {
	t.Parallel()
	repo := libraryRegistry{}
	technical := application.NewLibraryTechnical(repo, technicalHTTPPaths{}, technicalHTTPProber{}, nil)
	h := NewLibraryHandler(application.NewLibraryViews(repo, nil, nil), nil, technical, nil)

	got, err := h.GetLibraryFileTechnical(t.Context(), httpapi.GetLibraryFileTechnicalRequestObject{Id: 7, FileId: 999})
	if err != nil {
		t.Fatalf("GetLibraryFileTechnical: %v", err)
	}
	if _, ok := got.(httpapi.GetLibraryFileTechnical404ApplicationProblemPlusJSONResponse); !ok {
		t.Errorf("response = %T, want 404", got)
	}
}
