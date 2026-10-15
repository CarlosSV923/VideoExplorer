package application

import (
	"context"
	"errors"
	"testing"

	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/catalog/domain"
)

// viewRepo is the registry the reads see: titles with their files, and nothing else.
type viewRepo struct {
	titles []domain.LibraryTitle
	files  map[int64][]domain.LibraryFile
}

func (r *viewRepo) TitleByFolder(_ context.Context, folder string) (domain.LibraryTitle, bool, error) {
	for _, t := range r.titles {
		if t.Folder == folder {
			return t, true, nil
		}
	}
	return domain.LibraryTitle{}, false, nil
}

func (r *viewRepo) Titles(context.Context) ([]domain.LibraryTitle, error) { return r.titles, nil }

func (r *viewRepo) Files(_ context.Context, titleID int64) ([]domain.LibraryFile, error) {
	return r.files[titleID], nil
}

// viewTMDB answers the posters of the titles it knows, or fails.
type viewTMDB struct {
	configured bool
	fail       bool
	calls      int
}

func (t *viewTMDB) Configured() bool { return t.configured }

func (t *viewTMDB) SearchTitles(context.Context, domain.TitleKind, string, int) ([]TMDBTitle, error) {
	return nil, nil
}

func (t *viewTMDB) Title(_ context.Context, _ domain.TitleKind, id int64) (TMDBTitle, error) {
	t.calls++
	if t.fail {
		return TMDBTitle{}, ErrTMDBUnavailable
	}
	return TMDBTitle{ID: id, Title: "Title", DiscTitle: "Title", Year: 2008, PosterFile: "poster.jpg"}, nil
}

func (t *viewTMDB) Chapters(context.Context, int64, int, int, int) (TMDBChapters, error) {
	return TMDBChapters{}, nil
}

// liveFile is a video of the registry out of the trash.
func liveFile(id, titleID int64, rel string) domain.LibraryFile {
	return domain.LibraryFile{ID: id, TitleID: titleID, Kind: domain.FileVideo, Rel: rel}
}

// trashedFile is a file of the registry that is in the trash (RF-30).
func trashedFile(f domain.LibraryFile) domain.LibraryFile {
	f.TrashEntryID = 9
	return f
}

func movie(id int64, tmdbID int64, folder string) domain.LibraryTitle {
	return domain.LibraryTitle{ID: id, Kind: domain.KindMovie, Title: "Iron Man", DiscTitle: "Iron Man", Year: 2008, TMDBID: tmdbID, IDTag: tmdbID > 0, Folder: folder}
}

func TestTheLibraryListsOnlyTitlesWithAFileOutOfTheTrash(t *testing.T) {
	t.Parallel()
	repo := &viewRepo{
		titles: []domain.LibraryTitle{
			movie(1, 1726, "Movies/Iron Man (2008) [tmdbid-1726]"),
			movie(2, 0, "Movies/Free (2001)"),
			movie(3, 0, "Movies/Gone (1999)"),
		},
		files: map[int64][]domain.LibraryFile{
			1: {liveFile(10, 1, "Movies/Iron Man (2008) [tmdbid-1726]/Iron Man (2008) [tmdbid-1726].mkv")},
			2: {liveFile(20, 2, "Movies/Free (2001)/Free (2001).mkv")},
			3: {trashedFile(liveFile(30, 3, "Movies/Gone (1999)/Gone (1999).mkv"))},
		},
	}
	got, err := NewLibraryViews(repo, &viewTMDB{}, nil).Titles(t.Context())
	if err != nil {
		t.Fatalf("Titles: %v", err)
	}
	if len(got) != 2 || got[0].ID != 1 || got[1].ID != 2 {
		t.Fatalf("Titles = %+v, want titles 1 and 2 only", got)
	}
	if got[0].TMDBID != 1726 || !got[0].IDTag || got[1].TMDBID != 0 || got[1].IDTag {
		t.Errorf("the link and the tag: %+v", got)
	}
}

func TestATitleAnswersItsLiveVideosAndSubtitles(t *testing.T) {
	t.Parallel()
	series := domain.LibraryTitle{ID: 5, Kind: domain.KindSeries, Title: "Lizzie", DiscTitle: "Lizzie", Year: 1975, Folder: "TV Shows/Lizzie (1975)"}
	v1 := "TV Shows/Lizzie (1975)/Season 01/Lizzie (1975) - S01E01 - Pilot.mkv"
	v2 := "TV Shows/Lizzie (1975)/Season 01/Lizzie (1975) - S01E02 - Trial.mkv"
	files := []domain.LibraryFile{
		{ID: 51, TitleID: 5, Kind: domain.FileVideo, Rel: v1, Size: 100, Place: domain.Place{Episode: true, Season: 1, First: 1, Last: 1}},
		{ID: 52, TitleID: 5, Kind: domain.FileSubtitle, Rel: v1[:len(v1)-len(".mkv")] + ".es.srt", VideoRel: v1, Language: "es"},
		trashedFile(domain.LibraryFile{ID: 53, TitleID: 5, Kind: domain.FileSubtitle, Rel: v1[:len(v1)-len(".mkv")] + ".en.srt", VideoRel: v1, Language: "en"}),
		{ID: 54, TitleID: 5, Kind: domain.FileVideo, Rel: v2, Size: 200, Place: domain.Place{Episode: true, Season: 1, First: 2, Last: 2}},
		trashedFile(domain.LibraryFile{ID: 55, TitleID: 5, Kind: domain.FileVideo, Rel: "TV Shows/Lizzie (1975)/Season 01/old.mkv", Place: domain.Place{Episode: true, Season: 1, First: 3, Last: 3}}),
	}
	repo := &viewRepo{titles: []domain.LibraryTitle{series}, files: map[int64][]domain.LibraryFile{5: files}}
	got, err := NewLibraryViews(repo, &viewTMDB{}, nil).Title(t.Context(), 5)
	if err != nil {
		t.Fatalf("Title: %v", err)
	}
	if got.DiscTitle != "Lizzie" || len(got.Videos) != 2 {
		t.Fatalf("title = %+v", got)
	}
	first := got.Videos[0]
	if first.ID != 51 || len(first.Subtitles) != 1 || first.Subtitles[0].ID != 52 {
		t.Errorf("the first video and its live subtitle: %+v", first)
	}
	if first.ChapterTitle != "Pilot" {
		t.Errorf("a free series reads the chapter title from the name: %q", first.ChapterTitle)
	}
}

func TestATitleOutOfTheLibraryIsNotFound(t *testing.T) {
	t.Parallel()
	repo := &viewRepo{
		titles: []domain.LibraryTitle{movie(3, 0, "Movies/Gone (1999)")},
		files:  map[int64][]domain.LibraryFile{3: {trashedFile(liveFile(30, 3, "Movies/Gone (1999)/Gone (1999).mkv"))}},
	}
	if _, err := NewLibraryViews(repo, &viewTMDB{}, nil).Title(t.Context(), 3); !errors.Is(err, ErrTitleNotFound) {
		t.Errorf("a title with all its files in the trash: err = %v, want ErrTitleNotFound", err)
	}
	if _, err := NewLibraryViews(repo, &viewTMDB{}, nil).Title(t.Context(), 99); !errors.Is(err, ErrTitleNotFound) {
		t.Errorf("an unknown title: err = %v, want ErrTitleNotFound", err)
	}
}

func TestThePosterComesFromTMDBOnlyForALinkedTitle(t *testing.T) {
	t.Parallel()
	linked := movie(1, 1726, "Movies/Iron Man (2008) [tmdbid-1726]")
	free := movie(2, 0, "Movies/Free (2008)")
	repo := &viewRepo{
		titles: []domain.LibraryTitle{linked, free},
		files: map[int64][]domain.LibraryFile{
			1: {liveFile(10, 1, "Movies/Iron Man (2008) [tmdbid-1726]/Iron Man (2008) [tmdbid-1726].mkv")},
			2: {liveFile(20, 2, "Movies/Free (2008)/Free (2008).mkv")},
		},
	}
	tests := []struct {
		name       string
		id         int64
		configured bool
		fail       bool
		wantPoster string
		wantCalls  int
	}{
		{"linked, TMDB answers", 1, true, false, "poster.jpg", 1},
		{"linked, TMDB fails: no poster, no error", 1, true, true, "", 1},
		{"linked, TMDB not configured", 1, false, false, "", 0},
		{"free title never asks TMDB", 2, true, false, "", 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			tmdb := &viewTMDB{configured: tt.configured, fail: tt.fail}
			got, err := NewLibraryViews(repo, tmdb, nil).Title(t.Context(), tt.id)
			if err != nil {
				t.Fatalf("Title: %v", err)
			}
			if got.Poster != tt.wantPoster || tmdb.calls != tt.wantCalls {
				t.Errorf("poster = %q (calls %d), want %q (calls %d)", got.Poster, tmdb.calls, tt.wantPoster, tt.wantCalls)
			}
		})
	}
}
