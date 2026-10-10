package main

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/metadata/application"
	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/metadata/domain"
)

// fakeTMDB answers the provider calls from fixed data, per language. Nothing
// here touches the network.
type fakeTMDB struct {
	movies      map[string]map[int64]domain.Movie
	series      map[string]map[int64]domain.Series
	seasons     map[string]map[int64]map[int]domain.Season
	collections map[string]map[int64]domain.Collection
	genres      map[string]map[domain.MediaKind][]domain.Genre
}

var _ application.Provider = (*fakeTMDB)(nil)

func (f *fakeTMDB) SearchMovies(context.Context, string, string, *int) ([]domain.MovieSummary, error) {
	return nil, nil
}

func (f *fakeTMDB) SearchSeries(context.Context, string, string, *int) ([]domain.SeriesSummary, error) {
	return nil, nil
}

func (f *fakeTMDB) Movie(_ context.Context, language string, id int64) (domain.Movie, error) {
	m, ok := f.movies[language][id]
	if !ok {
		return domain.Movie{}, domain.ErrNotFound
	}
	return m, nil
}

func (f *fakeTMDB) Series(_ context.Context, language string, id int64) (domain.Series, error) {
	s, ok := f.series[language][id]
	if !ok {
		return domain.Series{}, domain.ErrNotFound
	}
	return s, nil
}

func (f *fakeTMDB) Season(_ context.Context, language string, seriesID int64, number int) (domain.Season, error) {
	s, ok := f.seasons[language][seriesID][number]
	if !ok {
		return domain.Season{}, domain.ErrNotFound
	}
	return s, nil
}

func (f *fakeTMDB) Collection(_ context.Context, language string, id int64) (domain.Collection, error) {
	c, ok := f.collections[language][id]
	if !ok {
		return domain.Collection{}, domain.ErrNotFound
	}
	return c, nil
}

func (f *fakeTMDB) Genres(_ context.Context, language string, kind domain.MediaKind) ([]domain.Genre, error) {
	return f.genres[language][kind], nil
}

func intPtr(v int) *int { return &v }

// fixtureTMDB is a small world with every case the build rules touch: a
// collection with an absent part, a movie with a non-Latin original title, a
// series with specials, and a series whose chapter titles are non-Latin in
// their original language.
func fixtureTMDB() *fakeTMDB {
	airDate := time.Date(2019, 3, 4, 0, 0, 0, 0, time.UTC)
	return &fakeTMDB{
		movies: map[string]map[int64]domain.Movie{
			"es-MX": {
				101: {
					ID: 101, Title: "Alfa", EnglishTitle: "Alpha", OriginalTitle: "Alpha", OriginalLanguage: "en", Overview: "Resumen tmdb A", Year: intPtr(2001), Runtime: 100,
					Genres:     []domain.Genre{{ID: 1, Name: "Drama"}},
					Collection: &domain.CollectionRef{ID: 500, Name: "Saga"},
				},
				102: {
					ID: 102, Title: "Alfa 2", EnglishTitle: "Alpha Two", OriginalTitle: "Alpha Two", OriginalLanguage: "en", Overview: "Resumen tmdb B", Year: intPtr(2004),
					Genres:     []domain.Genre{{ID: 1, Name: "Drama"}},
					Collection: &domain.CollectionRef{ID: 500, Name: "Saga"},
				},
				103: {
					ID: 103, Title: "Sakura", EnglishTitle: "Sakura", OriginalTitle: "さくら", OriginalLanguage: "ja", Overview: "Resumen tmdb C", Year: intPtr(2010),
					Genres: []domain.Genre{{ID: 2, Name: "Animación"}},
				},
			},
			"en-US": {
				101: {
					ID: 101, Title: "Alpha", EnglishTitle: "Alpha", OriginalTitle: "Alpha", OriginalLanguage: "en", Overview: "Overview tmdb A", Year: intPtr(2001),
					Genres: []domain.Genre{{ID: 1, Name: "Drama"}},
				},
				102: {
					ID: 102, Title: "Alpha Two", EnglishTitle: "Alpha Two", OriginalTitle: "Alpha Two", OriginalLanguage: "en", Overview: "Overview tmdb B", Year: intPtr(2004),
					Genres: []domain.Genre{{ID: 1, Name: "Drama"}},
				},
				103: {
					ID: 103, Title: "Sakura", EnglishTitle: "Sakura", OriginalTitle: "さくら", OriginalLanguage: "ja", Overview: "Overview tmdb C", Year: intPtr(2010),
					Genres: []domain.Genre{{ID: 2, Name: "Animation"}},
				},
			},
		},
		series: map[string]map[int64]domain.Series{
			"es-MX": {
				201: {
					ID: 201, Title: "Bravo", EnglishTitle: "Bravo", OriginalTitle: "Bravo", OriginalLanguage: "en", Overview: "Resumen tmdb serie", Year: intPtr(2019),
					Genres:  []domain.Genre{{ID: 3, Name: "Comedia"}},
					Seasons: []domain.SeasonSummary{{Number: 1, EpisodeCount: 2}, {Number: 0, EpisodeCount: 1}},
				},
				202: {
					ID: 202, Title: "Gana", EnglishTitle: "Gana", OriginalTitle: "가나", OriginalLanguage: "ko", Overview: "Resumen tmdb coreana", Year: intPtr(2021),
					Genres:  []domain.Genre{{ID: 3, Name: "Comedia"}},
					Seasons: []domain.SeasonSummary{{Number: 1, EpisodeCount: 1}},
				},
			},
			"en-US": {
				201: {
					ID: 201, Title: "Bravo", EnglishTitle: "Bravo", OriginalTitle: "Bravo", OriginalLanguage: "en", Overview: "Overview tmdb series", Year: intPtr(2019),
					Genres: []domain.Genre{{ID: 3, Name: "Comedy"}},
				},
				202: {
					ID: 202, Title: "Gana", EnglishTitle: "Gana", OriginalTitle: "가나", OriginalLanguage: "ko", Overview: "Overview tmdb korean", Year: intPtr(2021),
					Genres: []domain.Genre{{ID: 3, Name: "Comedy"}},
				},
			},
		},
		seasons: map[string]map[int64]map[int]domain.Season{
			"es-MX": {
				201: {
					0: {SeriesID: 201, Number: 0, Episodes: []domain.Episode{{Number: 1, Title: "Especial", AirDate: airDate}}},
					1: {SeriesID: 201, Number: 1, Episodes: []domain.Episode{{Number: 2, Title: "Episodio 2"}, {Number: 1, Title: "Piloto", AirDate: airDate}}},
				},
				202: {1: {SeriesID: 202, Number: 1, Episodes: []domain.Episode{{Number: 1, Title: "Rojo", AirDate: airDate}}}},
			},
			"en-US": {
				201: {
					0: {SeriesID: 201, Number: 0, Episodes: []domain.Episode{{Number: 1, Title: "Special"}}},
					1: {SeriesID: 201, Number: 1, Episodes: []domain.Episode{{Number: 1, Title: "Pilot"}, {Number: 2, Title: "Episode 2"}}},
				},
				202: {1: {SeriesID: 202, Number: 1, Episodes: []domain.Episode{{Number: 1, Title: "Red"}}}},
			},
			// The original language of 201 is English, so its own season is the en-US one.
			"en": {
				201: {
					0: {SeriesID: 201, Number: 0, Episodes: []domain.Episode{{Number: 1, Title: "Special"}}},
					1: {SeriesID: 201, Number: 1, Episodes: []domain.Episode{{Number: 1, Title: "Pilot"}, {Number: 2, Title: "Episode 2"}}},
				},
			},
			"ko": {202: {1: {SeriesID: 202, Number: 1, Episodes: []domain.Episode{{Number: 1, Title: "빨강"}}}}},
		},
		collections: map[string]map[int64]domain.Collection{
			"es-MX": {500: {ID: 500, Name: "Saga Alfa", Parts: []domain.CollectionPart{{ID: 101}, {ID: 102}}}},
			"en-US": {500: {ID: 500, Name: "Alpha Saga", Parts: []domain.CollectionPart{{ID: 101}, {ID: 102}}}},
		},
		genres: map[string]map[domain.MediaKind][]domain.Genre{
			"en-US": {domain.KindMovie: {{ID: 1, Name: "Drama"}, {ID: 2, Name: "Animation"}}},
		},
	}
}

func fixtureSynopses() synopsesFile {
	return synopsesFile{
		Movies: map[int64]localized{
			101: {ES: "Sinopsis propia A.", EN: "Own synopsis A."},
			102: {ES: "Sinopsis propia B.", EN: "Own synopsis B."},
			103: {ES: "Sinopsis propia C.", EN: "Own synopsis C."},
		},
		Series: map[int64]localized{
			201: {ES: "Sinopsis propia serie.", EN: "Own series synopsis."},
			202: {ES: "Sinopsis propia coreana.", EN: "Own Korean synopsis."},
		},
	}
}

func fixtureCurated() []curatedTitle {
	return []curatedTitle{
		{id: 101, kind: domain.KindMovie, original: "Alpha"},
		{id: 103, kind: domain.KindMovie, original: "さくら"},
		{id: 201, kind: domain.KindSeries, original: "Bravo"},
		{id: 202, kind: domain.KindSeries, original: "가나"},
	}
}

func TestBuildFixtureCatalog(t *testing.T) {
	t.Parallel()
	c, err := build(t.Context(), fixtureTMDB(), fixtureCurated(), fixtureSynopses())
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	byID := map[int64]catalogTitle{}
	for _, title := range c.Titles {
		byID[title.TMDBID] = title
	}
	tests := []struct {
		name     string
		id       int64
		disc     string
		titleEN  string
		colName  string
		episodes []string // chapter disc titles, in season then number order
	}{
		{name: "Latin original is the disc title", id: 101, disc: "Alpha", titleEN: "Alpha", colName: "Alpha Saga"},
		{name: "collection part is built too", id: 102, disc: "Alpha Two", titleEN: "Alpha Two", colName: "Alpha Saga"},
		{name: "non-Latin original takes the English title", id: 103, disc: "Sakura", titleEN: "Sakura"},
		{
			name: "series: chapters use Latin originals and drop generic titles", id: 201, disc: "Bravo", titleEN: "Bravo",
			episodes: []string{"Special", "Pilot", ""},
		},
		{
			name: "series: non-Latin chapter takes the English title", id: 202, disc: "Gana", titleEN: "Gana",
			episodes: []string{"Red"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, ok := byID[tt.id]
			if !ok {
				t.Fatalf("title %d missing from the catalog", tt.id)
			}
			if got.DiscTitle != tt.disc || got.Title.EN != tt.titleEN {
				t.Errorf("disc %q en %q, want %q %q", got.DiscTitle, got.Title.EN, tt.disc, tt.titleEN)
			}
			if tt.colName != "" && (got.Collection == nil || got.Collection.Name.EN != tt.colName) {
				t.Errorf("collection = %+v, want %q", got.Collection, tt.colName)
			}
			var discs []string
			for _, s := range got.Seasons {
				for _, e := range s.Episodes {
					discs = append(discs, e.DiscTitle)
				}
			}
			if tt.episodes != nil && strings.Join(discs, "|") != strings.Join(tt.episodes, "|") {
				t.Errorf("chapter disc titles = %q, want %q", discs, tt.episodes)
			}
		})
	}
}

func TestBuildSeasonsAreOrderedAndKeepSpecials(t *testing.T) {
	t.Parallel()
	c, err := build(t.Context(), fixtureTMDB(), fixtureCurated(), fixtureSynopses())
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	for _, title := range c.Titles {
		if title.TMDBID != 201 {
			continue
		}
		if len(title.Seasons) != 2 || title.Seasons[0].Number != 0 || title.Seasons[1].Number != 1 {
			t.Fatalf("seasons = %+v, want 0 then 1", title.Seasons)
		}
		ep := title.Seasons[1].Episodes
		if ep[0].Number != 1 || ep[1].Number != 2 || ep[0].Title.ES != "Piloto" || ep[1].AirDate != "" {
			t.Errorf("episodes of season 1 = %+v", ep)
		}
		if got := title.Seasons[0].Episodes[0].AirDate; got != "2019-03-04" {
			t.Errorf("air date = %q, want 2019-03-04", got)
		}
	}
}

func TestBuildIsDeterministic(t *testing.T) {
	t.Parallel()
	var outputs [][]byte
	for range 2 {
		c, err := build(t.Context(), fixtureTMDB(), fixtureCurated(), fixtureSynopses())
		if err != nil {
			t.Fatalf("build: %v", err)
		}
		data, err := marshal(c)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		outputs = append(outputs, data)
	}
	if !bytes.Equal(outputs[0], outputs[1]) {
		t.Error("two runs over the same data give different JSON")
	}
}

func TestBuildSynopsisRules(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		mutate   func(*synopsesFile)
		curated  []curatedTitle
		wantErr  string
		wantNone bool
	}{
		{
			name:     "every title has its synopsis",
			wantNone: true,
		},
		{
			name:    "collection part without synopsis fails",
			mutate:  func(s *synopsesFile) { delete(s.Movies, 102) },
			wantErr: "missing synopsis for movie 102",
		},
		{
			name:    "synopsis for an id outside the catalog fails",
			mutate:  func(s *synopsesFile) { s.Movies[999] = localized{ES: "x", EN: "x"} },
			wantErr: "synopsis of movie 999 matches no title",
		},
		{
			name:    "synopsis copying the TMDB overview fails",
			mutate:  func(s *synopsesFile) { s.Movies[101] = localized{ES: "Resumen tmdb A", EN: "Own"} },
			wantErr: "copies the TMDB overview",
		},
		{
			name:    "synopsis copying the English TMDB overview fails",
			mutate:  func(s *synopsesFile) { s.Series[201] = localized{ES: "Propia", EN: "overview TMDB series"} },
			wantErr: "copies the TMDB overview",
		},
		{
			name:    "a curated original title that differs from TMDB fails",
			curated: []curatedTitle{{id: 101, kind: domain.KindMovie, original: "Beta"}},
			wantErr: `movie 101 is "Alpha" in TMDB, not "Beta"`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			syn := fixtureSynopses()
			if tt.mutate != nil {
				tt.mutate(&syn)
			}
			cur := tt.curated
			if cur == nil {
				cur = fixtureCurated()
			}
			_, err := build(t.Context(), fixtureTMDB(), cur, syn)
			if tt.wantNone {
				if err != nil {
					t.Fatalf("build: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("error = %v, want one containing %q", err, tt.wantErr)
			}
		})
	}
}

func TestSameText(t *testing.T) {
	t.Parallel()
	tests := []struct {
		a, b string
		want bool
	}{
		{"Resumen  de   prueba", "resumen de prueba", true},
		{"", "", false},
		{"algo", "", false},
		{"uno", "dos", false},
	}
	for _, tt := range tests {
		if got := sameText(tt.a, tt.b); got != tt.want {
			t.Errorf("sameText(%q, %q) = %v, want %v", tt.a, tt.b, got, tt.want)
		}
	}
}
