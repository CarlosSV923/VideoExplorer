package http

import (
	"context"
	"testing"

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

func TestTheTitlePageAnswersTheVideosWithTheirChapters(t *testing.T) {
	t.Parallel()
	h := NewLibraryHandler(application.NewLibraryViews(libraryRegistry{}, nil, nil), nil, nil)
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

func TestATitleOutOfTheLibraryAnswersNotFound(t *testing.T) {
	t.Parallel()
	h := NewLibraryHandler(application.NewLibraryViews(libraryRegistry{}, nil, nil), nil, nil)
	got, err := h.GetLibraryTitle(t.Context(), httpapi.GetLibraryTitleRequestObject{Id: 99})
	if err != nil {
		t.Fatalf("GetLibraryTitle: %v", err)
	}
	if _, ok := got.(httpapi.GetLibraryTitle404ApplicationProblemPlusJSONResponse); !ok {
		t.Errorf("response = %T, want 404", got)
	}
}
