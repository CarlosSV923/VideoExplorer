package application

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/catalog/domain"
)

// viewRepo is the registry the reads see: titles with their files, and nothing else.
type viewRepo struct {
	titles  []domain.LibraryTitle
	files   map[int64][]domain.LibraryFile
	missing map[int64]*int
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

func (r *viewRepo) MissingEpisodes(_ context.Context, titleID int64) (*int, error) {
	value := r.missing[titleID]
	if value == nil {
		return nil, nil
	}
	copy := *value
	return &copy, nil
}

func (r *viewRepo) SetMissingEpisodes(_ context.Context, titleID int64, count int) error {
	if r.missing == nil {
		r.missing = map[int64]*int{}
	}
	r.missing[titleID] = &count
	return nil
}

// viewTMDB answers the posters of the titles it knows, or fails.
type viewTMDB struct {
	configured bool
	fail       bool
	calls      int
}

type richViewTMDB struct {
	viewTMDB
	metadata TitleMetadata
	err      error
}

func (t *richViewTMDB) Details(context.Context, domain.TitleKind, int64) (TitleMetadata, error) {
	t.calls++
	return t.metadata, t.err
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

func TestTheTitleListReadsTheStoredCountWithoutCallingTMDB(t *testing.T) {
	t.Parallel()
	count := 4
	series := domain.LibraryTitle{ID: 5, Kind: domain.KindSeries, Title: "Show", DiscTitle: "Show", Year: 2020, TMDBID: 100, Folder: "TV Shows/Show (2020)"}
	repo := &viewRepo{
		titles:  []domain.LibraryTitle{series},
		files:   map[int64][]domain.LibraryFile{5: {liveFile(50, 5, "TV Shows/Show (2020)/Season 01/Show - S01E01.mkv")}},
		missing: map[int64]*int{5: &count},
	}
	tmdb := &richViewTMDB{viewTMDB: viewTMDB{configured: true}}
	got, err := NewLibraryViews(repo, tmdb, nil).Titles(t.Context())
	if err != nil {
		t.Fatalf("Titles: %v", err)
	}
	if len(got) != 1 || got[0].MissingEpisodes == nil || *got[0].MissingEpisodes != count {
		t.Fatalf("titles = %+v, want stored count %d", got, count)
	}
	if tmdb.calls != 0 {
		t.Fatalf("TMDB Details calls = %d, want 0", tmdb.calls)
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

func TestTheTitlePageBuildsTheTMDBCollectionAndMarksLibraryMovies(t *testing.T) {
	t.Parallel()
	metadata := TitleMetadata{Title: "Iron Man", OriginalTitle: "Iron Man", Collection: &CollectionMetadata{ID: 10, Name: "Iron Man Collection", Movies: []CollectionMovieMetadata{
		{ID: 1726, Title: "Iron Man", OriginalTitle: "Iron Man", Year: 2008},
		{ID: 1727, Title: "Iron Man 2", OriginalTitle: "Iron Man 2", Year: 2010},
	}}}
	for _, tt := range []struct {
		name           string
		sister         bool
		wantSisterLive bool
	}{
		{name: "sister in library", sister: true, wantSisterLive: true},
		{name: "sister absent", sister: false, wantSisterLive: false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			titles := []domain.LibraryTitle{movie(1, 1726, "Movies/Iron Man (2008) [tmdbid-1726]")}
			files := map[int64][]domain.LibraryFile{1: {liveFile(10, 1, "Movies/Iron Man (2008) [tmdbid-1726]/Iron Man.mkv")}}
			if tt.sister {
				titles = append(titles, movie(2, 1727, "Movies/Iron Man 2 (2010) [tmdbid-1727]"))
				files[2] = []domain.LibraryFile{liveFile(20, 2, "Movies/Iron Man 2 (2010) [tmdbid-1727]/Iron Man 2.mkv")}
			}
			tmdb := &richViewTMDB{viewTMDB: viewTMDB{configured: true}, metadata: metadata}
			got, err := NewLibraryViews(&viewRepo{titles: titles, files: files}, tmdb, nil).Title(t.Context(), 1)
			if err != nil {
				t.Fatalf("Title: %v", err)
			}
			if got.Collection == nil || len(got.Collection.Movies) != 2 {
				t.Fatalf("collection = %+v", got.Collection)
			}
			if got.Collection.Movies[1].LibraryTitleID != map[bool]int64{true: 2, false: 0}[tt.wantSisterLive] {
				t.Errorf("sister library id = %d, want live=%v", got.Collection.Movies[1].LibraryTitleID, tt.wantSisterLive)
			}
		})
	}
}

func TestTheSeriesPageMarksPresentMissingAndUpcomingEpisodes(t *testing.T) {
	t.Parallel()
	series := domain.LibraryTitle{ID: 7, Kind: domain.KindSeries, Title: "Show", DiscTitle: "Show", Year: 2020, TMDBID: 100, Folder: "TV Shows/Show (2020)"}
	video := domain.LibraryFile{ID: 70, TitleID: 7, Kind: domain.FileVideo, Rel: "TV Shows/Show (2020)/Season 01/Show - S01E05-E06.mkv", Place: domain.Place{Episode: true, Season: 1, First: 5, Last: 6}}
	files := []domain.LibraryFile{video, {ID: 71, TitleID: 7, Kind: domain.FileVideo, Rel: "TV Shows/Show (2020)/Season 01/Show - S01E01.mkv", Place: domain.Place{Episode: true, Season: 1, First: 1, Last: 1}}}
	episodes := []EpisodeMetadata{
		{ID: 11, Number: 1, Title: "One", AirDate: time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)},
		{ID: 12, Number: 2, Title: "Two", AirDate: time.Date(2020, 1, 2, 0, 0, 0, 0, time.UTC)},
		{ID: 13, Number: 3, Title: "Three", AirDate: time.Date(2026, 10, 16, 0, 0, 0, 0, time.UTC)},
		{ID: 14, Number: 4, Title: "Four"},
		{ID: 15, Number: 5, Title: "Five"},
		{ID: 16, Number: 6, Title: "Six"},
	}
	metadata := TitleMetadata{Title: "Show", OriginalTitle: "The Show"}
	metadata.Seasons = []SeasonMetadata{
		{Number: 0, Episodes: []EpisodeMetadata{{ID: 1, Number: 1, Title: "Special", AirDate: time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)}}},
		{Number: 1, Episodes: episodes},
	}
	tmdb := &richViewTMDB{viewTMDB: viewTMDB{configured: true}, metadata: metadata}
	now := func() time.Time { return time.Date(2026, 10, 15, 12, 0, 0, 0, time.UTC) }
	got, err := NewLibraryViews(&viewRepo{titles: []domain.LibraryTitle{series}, files: map[int64][]domain.LibraryFile{7: files}}, tmdb, nil, now).Title(t.Context(), 7)
	if err != nil {
		t.Fatalf("Title: %v", err)
	}
	if len(got.Seasons) != 2 || got.Seasons[0].Number != 0 || got.MissingEpisodes == nil || *got.MissingEpisodes != 1 {
		t.Fatalf("seasons=%+v missing=%v", got.Seasons, got.MissingEpisodes)
	}
	if got.Seasons[0].Episodes[0].Status != EpisodeMissing {
		t.Errorf("special status = %q, want missing without affecting count", got.Seasons[0].Episodes[0].Status)
	}
	season := got.Seasons[1].Episodes
	want := []EpisodeStatus{EpisodePresent, EpisodeMissing, EpisodeUpcoming, EpisodeUpcoming, EpisodePresent, EpisodePresent}
	for i, status := range want {
		if season[i].Status != status {
			t.Errorf("episode %d status = %q, want %q", season[i].Number, season[i].Status, status)
		}
	}
	if len(season[4].Files) != 1 || len(season[5].Files) != 1 || season[4].Files[0].ID != 70 || season[5].Files[0].ID != 70 {
		t.Errorf("multi-episode file = %+v %+v", season[4].Files, season[5].Files)
	}
}

func TestFreeTitleHasNoTMDBMissingEpisodes(t *testing.T) {
	t.Parallel()
	free := movie(1, 0, "Movies/Free (2020)")
	tmdb := &richViewTMDB{viewTMDB: viewTMDB{configured: true}}
	got, err := NewLibraryViews(&viewRepo{titles: []domain.LibraryTitle{free}, files: map[int64][]domain.LibraryFile{1: {liveFile(10, 1, "Movies/Free (2020)/Free.mkv")}}}, tmdb, nil).Title(t.Context(), 1)
	if err != nil {
		t.Fatalf("Title: %v", err)
	}
	if got.OriginalTitle != "" || len(got.Seasons) != 0 || got.MissingEpisodes != nil || tmdb.calls != 0 {
		t.Errorf("free title = %+v, TMDB calls=%d", got, tmdb.calls)
	}
}

func TestTMDBWithoutCachedDetailsDoesNotBreakTheTitlePage(t *testing.T) {
	t.Parallel()
	linked := movie(1, 1726, "Movies/Iron Man (2008) [tmdbid-1726]")
	tmdb := &richViewTMDB{viewTMDB: viewTMDB{configured: true}, err: ErrTMDBUnavailable}
	got, err := NewLibraryViews(&viewRepo{titles: []domain.LibraryTitle{linked}, files: map[int64][]domain.LibraryFile{1: {liveFile(10, 1, "Movies/Iron Man (2008) [tmdbid-1726]/Iron Man.mkv")}}}, tmdb, nil).Title(t.Context(), 1)
	if err != nil {
		t.Fatalf("Title: %v", err)
	}
	if got.Title != linked.Title || got.DiscTitle != linked.DiscTitle || got.Poster != "" || got.MissingEpisodes != nil {
		t.Errorf("record-only title = %+v", got)
	}
}

func TestTheTitlePageKeepsThePreviousCountWhenTMDBIsUnavailable(t *testing.T) {
	t.Parallel()
	previous := 7
	series := domain.LibraryTitle{ID: 1, Kind: domain.KindSeries, Title: "Show", DiscTitle: "Show", TMDBID: 100, Folder: "TV Shows/Show"}
	repo := &viewRepo{titles: []domain.LibraryTitle{series}, files: map[int64][]domain.LibraryFile{1: {{ID: 10, TitleID: 1, Kind: domain.FileVideo, Rel: "show.mkv", Place: domain.Place{Episode: true, Season: 1, First: 1, Last: 1}}}}, missing: map[int64]*int{1: &previous}}
	tmdb := &richViewTMDB{viewTMDB: viewTMDB{configured: true}, err: ErrTMDBUnavailable}
	got, err := NewLibraryViews(repo, tmdb, nil).Title(t.Context(), 1)
	if err != nil {
		t.Fatalf("Title: %v", err)
	}
	if got.MissingEpisodes == nil || *got.MissingEpisodes != previous {
		t.Fatalf("missing count = %v, want previous count %d", got.MissingEpisodes, previous)
	}
	if stored, _ := repo.MissingEpisodes(t.Context(), 1); stored == nil || *stored != previous {
		t.Fatalf("stored count = %v, want previous count %d", stored, previous)
	}
}

func TestRefreshMissingEpisodesKeepsThePreviousValueWhenTMDBFails(t *testing.T) {
	t.Parallel()
	previous := 3
	series := domain.LibraryTitle{ID: 1, Kind: domain.KindSeries, Title: "Show", DiscTitle: "Show", TMDBID: 100, Folder: "TV Shows/Show"}
	tests := []struct {
		name string
		err  error
		want int
	}{
		{name: "TMDB answers", want: 1},
		{name: "TMDB fails", err: ErrTMDBUnavailable, want: previous},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			repo := &viewRepo{titles: []domain.LibraryTitle{series}, files: map[int64][]domain.LibraryFile{1: {{ID: 10, TitleID: 1, Kind: domain.FileVideo, Rel: "show.mkv", Place: domain.Place{Episode: true, Season: 1, First: 1, Last: 1}}}}, missing: map[int64]*int{1: &previous}}
			metadata := TitleMetadata{Seasons: []SeasonMetadata{{Number: 1, Episodes: []EpisodeMetadata{{Number: 1, AirDate: time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)}, {Number: 2, AirDate: time.Date(2020, 1, 2, 0, 0, 0, 0, time.UTC)}}}}}
			tmdb := &richViewTMDB{viewTMDB: viewTMDB{configured: true}, metadata: metadata, err: tt.err}
			if err := NewLibraryViews(repo, tmdb, nil, func() time.Time { return time.Date(2026, 10, 15, 0, 0, 0, 0, time.UTC) }).RefreshMissingEpisodes(t.Context()); err != nil {
				t.Fatalf("RefreshMissingEpisodes: %v", err)
			}
			stored, _ := repo.MissingEpisodes(t.Context(), 1)
			if stored == nil || *stored != tt.want {
				t.Fatalf("stored count = %v, want %d", stored, tt.want)
			}
		})
	}
}
