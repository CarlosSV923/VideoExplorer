package application_test

import (
	"context"
	"errors"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/metadata/application"
	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/metadata/domain"
)

// fakeProvider records the method and language of every call and answers from fixed data.
type fakeProvider struct {
	mu    sync.Mutex
	calls []string // "method:language" in call order

	series      domain.Series
	seasons     map[string]domain.Season // by language
	collections map[string]domain.Collection
	genres      map[string][]domain.Genre
	movie       domain.Movie
	err         error
	failures    map[string]error // "method:language" -> error for that call only
	searchQuery string
	searchYear  *int
	imdbID      string // the IMDb id FindByIMDb was asked for
}

func (f *fakeProvider) record(method, language string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, method+":"+language)
	if err := f.failures[method+":"+language]; err != nil {
		return err
	}
	return f.err
}

// callsTo returns the recorded calls of one method, in order.
func (f *fakeProvider) callsTo(method string) []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []string
	for _, c := range f.calls {
		if name, _, _ := strings.Cut(c, ":"); name == method {
			out = append(out, c)
		}
	}
	return out
}

func (f *fakeProvider) SearchMovies(_ context.Context, language, query string, year *int) ([]domain.MovieSummary, error) {
	f.searchQuery, f.searchYear = query, year
	if err := f.record("SearchMovies", language); err != nil {
		return nil, err
	}
	return []domain.MovieSummary{{ID: 1, Title: "Iron Man"}}, nil
}

func (f *fakeProvider) SearchSeries(_ context.Context, language, query string, year *int) ([]domain.SeriesSummary, error) {
	f.searchQuery, f.searchYear = query, year
	return nil, f.record("SearchSeries", language)
}

func (f *fakeProvider) FindByIMDb(_ context.Context, language, imdbID string) (domain.FindResult, error) {
	f.imdbID = imdbID
	if err := f.record("FindByIMDb", language); err != nil {
		return domain.FindResult{}, err
	}
	return domain.FindResult{Movies: []domain.MovieSummary{{ID: 1, Title: "Alien Romulus"}}}, nil
}

func (f *fakeProvider) Movie(_ context.Context, language string, _ int64) (domain.Movie, error) {
	if err := f.record("Movie", language); err != nil {
		return domain.Movie{}, err
	}
	return f.movie, nil
}

func (f *fakeProvider) Series(_ context.Context, language string, _ int64) (domain.Series, error) {
	if err := f.record("Series", language); err != nil {
		return domain.Series{}, err
	}
	return f.series, nil
}

func (f *fakeProvider) Season(_ context.Context, language string, _ int64, _ int) (domain.Season, error) {
	if err := f.record("Season", language); err != nil {
		return domain.Season{}, err
	}
	return f.seasons[language], nil
}

func (f *fakeProvider) Collection(_ context.Context, language string, _ int64) (domain.Collection, error) {
	if err := f.record("Collection", language); err != nil {
		return domain.Collection{}, err
	}
	return f.collections[language], nil
}

func (f *fakeProvider) Genres(_ context.Context, language string, _ domain.MediaKind) ([]domain.Genre, error) {
	if err := f.record("Genres", language); err != nil {
		return nil, err
	}
	return f.genres[language], nil
}

func TestNilProviderIsNotConfigured(t *testing.T) {
	t.Parallel()

	svc := application.NewService(nil, "es-MX")
	if svc.Configured() {
		t.Fatal("Configured = true without provider")
	}
	if _, err := svc.SearchMovies(t.Context(), "Iron Man", nil); !errors.Is(err, domain.ErrNotConfigured) {
		t.Errorf("SearchMovies err = %v, want ErrNotConfigured", err)
	}
	if _, err := svc.Movie(t.Context(), 1726); !errors.Is(err, domain.ErrNotConfigured) {
		t.Errorf("Movie err = %v, want ErrNotConfigured", err)
	}
	if _, err := svc.Genres(t.Context(), domain.KindMovie); !errors.Is(err, domain.ErrNotConfigured) {
		t.Errorf("Genres err = %v, want ErrNotConfigured", err)
	}
}

func TestSearchValidatesInputAndPassesLanguage(t *testing.T) {
	t.Parallel()

	f := &fakeProvider{}
	svc := application.NewService(f, "es-MX")
	year := 2008

	if _, err := svc.SearchMovies(t.Context(), "  ", nil); !errors.Is(err, domain.ErrInvalidQuery) {
		t.Errorf("blank query err = %v, want ErrInvalidQuery", err)
	}
	zero := 0
	if _, err := svc.SearchMovies(t.Context(), "Iron Man", &zero); !errors.Is(err, domain.ErrInvalidQuery) {
		t.Errorf("year 0 err = %v, want ErrInvalidQuery", err)
	}
	if _, err := svc.SearchMovies(t.Context(), "  Iron   Man ", &year); err != nil {
		t.Fatalf("SearchMovies: %v", err)
	}
	if f.searchQuery != "Iron Man" || f.searchYear == nil || *f.searchYear != 2008 {
		t.Errorf("provider got query %q year %v", f.searchQuery, f.searchYear)
	}
	if got := f.callsTo("SearchMovies"); len(got) != 1 || got[0] != "SearchMovies:es-MX" {
		t.Errorf("calls = %v, want one call in es-MX", got)
	}
}

func TestFindByIMDbChecksTheIDAndPassesLanguage(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		id      string
		wantID  string
		wantErr error
	}{
		{"an id in capitals is sent in lower case", "TT18412256", "tt18412256", nil},
		{"surrounding spaces are removed", " tt18412256 ", "tt18412256", nil},
		{"a TMDB id is not an IMDb id", "1726", "", domain.ErrInvalidQuery},
		{"a path is not an IMDb id", "tt1/../../x", "", domain.ErrInvalidQuery},
		{"an empty id is refused", "", "", domain.ErrInvalidQuery},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			f := &fakeProvider{}
			svc := application.NewService(f, "es-MX")
			got, err := svc.FindByIMDb(t.Context(), tt.id)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("FindByIMDb(%q) err = %v, want %v", tt.id, err, tt.wantErr)
			}
			if tt.wantErr != nil {
				if calls := f.callsTo("FindByIMDb"); len(calls) != 0 {
					t.Errorf("the provider was asked for an invalid id: %v", calls)
				}
				return
			}
			if f.imdbID != tt.wantID {
				t.Errorf("provider got id %q, want %q", f.imdbID, tt.wantID)
			}
			if calls := f.callsTo("FindByIMDb"); len(calls) != 1 || calls[0] != "FindByIMDb:es-MX" {
				t.Errorf("calls = %v, want one call in es-MX", calls)
			}
			if len(got.Movies) != 1 || got.Movies[0].Title != "Alien Romulus" {
				t.Errorf("movies = %+v", got.Movies)
			}
		})
	}
}

func TestFindByIMDbWithoutProviderIsNotConfigured(t *testing.T) {
	t.Parallel()
	svc := application.NewService(nil, "es-MX")
	if _, err := svc.FindByIMDb(t.Context(), "tt18412256"); !errors.Is(err, domain.ErrNotConfigured) {
		t.Errorf("err = %v, want ErrNotConfigured", err)
	}
}

func TestNonPositiveIDIsNotFound(t *testing.T) {
	t.Parallel()

	svc := application.NewService(&fakeProvider{}, "es-MX")
	if _, err := svc.Movie(t.Context(), 0); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("Movie(0) err = %v, want ErrNotFound", err)
	}
}

func TestSeasonFillsMissingTextFromFallbacks(t *testing.T) {
	t.Parallel()

	f := &fakeProvider{seasons: map[string]domain.Season{
		"es-MX": {Number: 1, Episodes: []domain.Episode{
			{Number: 1, Title: "Piloto", Overview: ""},
			{Number: 2, Title: "", Overview: "Resumen"},
		}},
		"en-US": {Number: 1, Episodes: []domain.Episode{
			{Number: 1, Title: "Pilot", Overview: "English pilot"},
			{Number: 2, Title: "Cat's in the Bag", Overview: "English two"},
		}},
	}}
	svc := application.NewService(f, "es-MX")

	s, err := svc.Season(t.Context(), 1396, 1)
	if err != nil {
		t.Fatalf("Season: %v", err)
	}
	if s.Episodes[0].Title != "Piloto" || s.Episodes[0].Overview != "English pilot" {
		t.Errorf("episode 1 = %+v, want own title and English overview", s.Episodes[0])
	}
	if s.Episodes[1].Title != "Cat's in the Bag" || s.Episodes[1].Overview != "Resumen" {
		t.Errorf("episode 2 = %+v, want English title and own overview", s.Episodes[1])
	}
	// en-US is asked for the chapter names first; the bare language "es" is then the
	// first fallback of the text. One request per season, not per episode.
	want := []string{"Season:es-MX", "Season:en-US", "Season:es"}
	if got := f.callsTo("Season"); !slices.Equal(got, want) {
		t.Errorf("season calls = %v, want %v", got, want)
	}
}

func TestSeasonUsesSameLanguageBeforeEnglish(t *testing.T) {
	t.Parallel()

	f := &fakeProvider{seasons: map[string]domain.Season{
		"es-MX": {Episodes: []domain.Episode{{Number: 1, Title: "Piloto", Overview: ""}}},
		"es":    {Episodes: []domain.Episode{{Number: 1, Title: "Otro", Overview: "Resumen en español"}}},
		"en-US": {Episodes: []domain.Episode{{Number: 1, Title: "Pilot", Overview: "English"}}},
	}}
	svc := application.NewService(f, "es-MX")

	s, err := svc.Season(t.Context(), 1396, 1)
	if err != nil {
		t.Fatalf("Season: %v", err)
	}
	if s.Episodes[0].Title != "Piloto" || s.Episodes[0].Overview != "Resumen en español" {
		t.Errorf("episode = %+v, want own title and the es overview", s.Episodes[0])
	}
	// en-US is asked for the chapter names; the text itself comes from "es", which is asked first.
	want := []string{"Season:es-MX", "Season:en-US", "Season:es"}
	if got := f.callsTo("Season"); !slices.Equal(got, want) {
		t.Errorf("season calls = %v, want %v", got, want)
	}
}

func TestSeasonFallbackFailureIsSkipped(t *testing.T) {
	t.Parallel()

	f := &fakeProvider{
		seasons: map[string]domain.Season{
			"es-MX": {Episodes: []domain.Episode{{Number: 1, Title: "Piloto"}}},
			"en-US": {Episodes: []domain.Episode{{Number: 1, Title: "Pilot", Overview: "English"}}},
		},
		failures: map[string]error{"Season:es": domain.ErrUpstream},
	}
	svc := application.NewService(f, "es-MX")

	s, err := svc.Season(t.Context(), 1396, 1)
	if err != nil {
		t.Fatalf("Season with a failing bare-language fallback: %v", err)
	}
	if s.Episodes[0].Title != "Piloto" || s.Episodes[0].Overview != "English" {
		t.Errorf("episode = %+v, want the English overview after the skipped fallback", s.Episodes[0])
	}
}

func TestSeasonInEnglishNeverAsksAgain(t *testing.T) {
	t.Parallel()

	f := &fakeProvider{seasons: map[string]domain.Season{
		"en-US": {Episodes: []domain.Episode{{Number: 1}}},
	}}
	svc := application.NewService(f, "en-US")

	if _, err := svc.Season(t.Context(), 1396, 1); err != nil {
		t.Fatalf("Season: %v", err)
	}
	if got := f.callsTo("Season"); len(got) != 1 {
		t.Errorf("season calls = %v, want exactly one", got)
	}
}

func TestSeasonSkipsFallbackWhenTextIsComplete(t *testing.T) {
	t.Parallel()

	f := &fakeProvider{seasons: map[string]domain.Season{
		"es-MX": {Episodes: []domain.Episode{{Number: 1, Title: "Piloto", Overview: "Resumen"}}},
	}}
	svc := application.NewService(f, "es-MX")

	if _, err := svc.Season(t.Context(), 1396, 1); err != nil {
		t.Fatalf("Season: %v", err)
	}
	// No fallback for the text; only the en-US season for the chapter names.
	want := []string{"Season:es-MX", "Season:en-US"}
	if got := f.callsTo("Season"); !slices.Equal(got, want) {
		t.Errorf("season calls = %v, want %v (no text fallback needed)", got, want)
	}
}

func TestSeasonGivesChapterTitlesInOriginalAndEnglish(t *testing.T) {
	t.Parallel()

	f := &fakeProvider{
		series: domain.Series{OriginalLanguage: "ja"},
		seasons: map[string]domain.Season{
			"es-MX": {Episodes: []domain.Episode{{Number: 1, Title: "Piloto", Overview: "Resumen"}}},
			"ja":    {Episodes: []domain.Episode{{Number: 1, Title: "パイロット", Overview: "概要"}}},
			"en-US": {Episodes: []domain.Episode{{Number: 1, Title: "Pilot", Overview: "English"}}},
		},
	}
	svc := application.NewService(f, "es-MX")

	s, err := svc.Season(t.Context(), 1396, 1)
	if err != nil {
		t.Fatalf("Season: %v", err)
	}
	ep := s.Episodes[0]
	if ep.OriginalTitle != "パイロット" || ep.EnglishTitle != "Pilot" || ep.Title != "Piloto" {
		t.Errorf("episode = %+v, want the Japanese and English chapter titles and the own title", ep)
	}
	// The series' language and en-US: two season requests besides the configured one.
	want := []string{"Season:es-MX", "Season:en-US", "Season:ja"}
	if got := f.callsTo("Season"); !slices.Equal(got, want) {
		t.Errorf("season calls = %v, want %v", got, want)
	}
}

func TestSeasonAsksTheSameLanguageOnce(t *testing.T) {
	t.Parallel()

	f := &fakeProvider{
		series: domain.Series{OriginalLanguage: "ja"},
		seasons: map[string]domain.Season{
			"ja":    {Episodes: []domain.Episode{{Number: 1, Title: "パイロット"}}},
			"en-US": {Episodes: []domain.Episode{{Number: 1, Title: "Pilot"}}},
		},
	}
	svc := application.NewService(f, "ja")

	s, err := svc.Season(t.Context(), 1396, 1)
	if err != nil {
		t.Fatalf("Season: %v", err)
	}
	if ep := s.Episodes[0]; ep.OriginalTitle != "パイロット" || ep.EnglishTitle != "Pilot" {
		t.Errorf("episode = %+v, want the original and English titles", ep)
	}
	// The configured language is the original one: it is not asked again.
	want := []string{"Season:ja", "Season:en-US"}
	if got := f.callsTo("Season"); !slices.Equal(got, want) {
		t.Errorf("season calls = %v, want %v", got, want)
	}
}

func TestSeasonInAnEnglishSeriesUsesOneSourceForBothNames(t *testing.T) {
	t.Parallel()

	f := &fakeProvider{
		series: domain.Series{OriginalLanguage: "en"},
		seasons: map[string]domain.Season{
			"es-MX": {Episodes: []domain.Episode{{Number: 1, Title: "Piloto", Overview: "Resumen"}}},
			"en-US": {Episodes: []domain.Episode{{Number: 1, Title: "Pilot", Overview: "Summary"}}},
		},
	}
	svc := application.NewService(f, "es-MX")

	s, err := svc.Season(t.Context(), 1396, 1)
	if err != nil {
		t.Fatalf("Season: %v", err)
	}
	if ep := s.Episodes[0]; ep.OriginalTitle != "Pilot" || ep.EnglishTitle != "Pilot" {
		t.Errorf("episode = %+v, want Pilot as both names", ep)
	}
	want := []string{"Season:es-MX", "Season:en-US"}
	if got := f.callsTo("Season"); !slices.Equal(got, want) {
		t.Errorf("season calls = %v, want %v", got, want)
	}
}

func TestSeasonWithoutOriginalLanguageHasNoOriginalTitle(t *testing.T) {
	t.Parallel()

	f := &fakeProvider{seasons: map[string]domain.Season{
		"es-MX": {Episodes: []domain.Episode{{Number: 1, Title: "Piloto"}}},
		"en-US": {Episodes: []domain.Episode{{Number: 1, Title: "Pilot"}}},
	}}
	svc := application.NewService(f, "es-MX")

	s, err := svc.Season(t.Context(), 1396, 1)
	if err != nil {
		t.Fatalf("Season: %v", err)
	}
	if ep := s.Episodes[0]; ep.OriginalTitle != "" || ep.EnglishTitle != "Pilot" {
		t.Errorf("episode = %+v, want no original title and the English one", ep)
	}
	if got := f.callsTo("Season"); slices.Contains(got, "Season:ja") {
		t.Errorf("season calls = %v, want no request for an unknown original language", got)
	}
}

func TestSeasonNotFoundInOriginalLanguageLeavesItsTitlesEmpty(t *testing.T) {
	t.Parallel()

	f := &fakeProvider{
		series: domain.Series{OriginalLanguage: "ja"},
		seasons: map[string]domain.Season{
			"es-MX": {Episodes: []domain.Episode{{Number: 1, Title: "Piloto", Overview: "R"}}},
			"en-US": {Episodes: []domain.Episode{{Number: 1, Title: "Pilot", Overview: "E"}}},
		},
		failures: map[string]error{"Season:ja": domain.ErrNotFound},
	}
	svc := application.NewService(f, "es-MX")

	s, err := svc.Season(t.Context(), 1396, 1)
	if err != nil {
		t.Fatalf("Season with the original language unknown to TMDB: %v", err)
	}
	if ep := s.Episodes[0]; ep.OriginalTitle != "" || ep.EnglishTitle != "Pilot" {
		t.Errorf("episode = %+v, want no original title and the English one", ep)
	}
}

func TestSeasonUpstreamFailureOfAChapterNameIsReturned(t *testing.T) {
	t.Parallel()

	f := &fakeProvider{
		series: domain.Series{OriginalLanguage: "ja"},
		seasons: map[string]domain.Season{
			"es-MX": {Episodes: []domain.Episode{{Number: 1, Title: "Piloto", Overview: "R"}}},
		},
		failures: map[string]error{"Season:ja": domain.ErrUpstream},
	}
	svc := application.NewService(f, "es-MX")

	if _, err := svc.Season(t.Context(), 1396, 1); !errors.Is(err, domain.ErrUpstream) {
		t.Errorf("Season err = %v, want the upstream error: the chapter names would be wrong", err)
	}
}

func TestCollectionFillsNameAndPartTitles(t *testing.T) {
	t.Parallel()

	f := &fakeProvider{collections: map[string]domain.Collection{
		"es-MX": {ID: 10, Name: "", Parts: []domain.CollectionPart{{ID: 1, Title: "Iron Man"}, {ID: 2, Title: ""}}},
		"en-US": {ID: 10, Name: "Iron Man Collection", Overview: "Saga", Parts: []domain.CollectionPart{
			{ID: 1, Title: "Iron Man"}, {ID: 2, Title: "Iron Man 2"},
		}},
	}}
	svc := application.NewService(f, "es-MX")

	c, err := svc.Collection(t.Context(), 10)
	if err != nil {
		t.Fatalf("Collection: %v", err)
	}
	if c.Name != "Iron Man Collection" || c.Overview != "Saga" || c.Parts[1].Title != "Iron Man 2" {
		t.Errorf("collection = %+v, want English name, overview and part title", c)
	}
	if got := f.callsTo("Collection"); !slices.Equal(got, []string{"Collection:es-MX", "Collection:es", "Collection:en-US"}) {
		t.Errorf("collection calls = %v", got)
	}
}

func TestMovieNamesUnnamedGenresFromFallbacks(t *testing.T) {
	t.Parallel()

	f := &fakeProvider{
		movie: domain.Movie{ID: 1726, Genres: []domain.Genre{{ID: 28, Name: ""}, {ID: 878, Name: "Ciencia ficción"}}},
		genres: map[string][]domain.Genre{
			"es":    {{ID: 28, Name: "Acción"}},
			"en-US": {{ID: 28, Name: "Action"}, {ID: 878, Name: "Science Fiction"}},
		},
	}
	svc := application.NewService(f, "es-MX")

	m, err := svc.Movie(t.Context(), 1726)
	if err != nil {
		t.Fatalf("Movie: %v", err)
	}
	if m.Genres[0].Name != "Acción" || m.Genres[1].Name != "Ciencia ficción" {
		t.Errorf("genres = %+v", m.Genres)
	}
	// "es" named the only unnamed genre, so en-US is not asked.
	if got := f.callsTo("Genres"); !slices.Equal(got, []string{"Genres:es"}) {
		t.Errorf("genre calls = %v, want only es", got)
	}
}

func TestProviderErrorsAreReturned(t *testing.T) {
	t.Parallel()

	boom := errors.New("boom")
	svc := application.NewService(&fakeProvider{err: boom}, "es-MX")

	if _, err := svc.Series(t.Context(), 1396); !errors.Is(err, boom) {
		t.Errorf("Series err = %v, want the provider error", err)
	}
	if _, err := svc.Season(t.Context(), 1396, 1); !errors.Is(err, boom) {
		t.Errorf("Season err = %v, want the provider error", err)
	}
}
