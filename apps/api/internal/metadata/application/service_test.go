package application_test

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/metadata/application"
	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/metadata/domain"
)

// fakeProvider records the method and language of every call and answers from fixed data.
type fakeProvider struct {
	mu    sync.Mutex
	calls []string // "method:language" in call order

	seasons     map[string]domain.Season // by language
	collections map[string]domain.Collection
	genres      map[string][]domain.Genre
	movie       domain.Movie
	err         error
	searchQuery string
	searchYear  *int
}

func (f *fakeProvider) record(method, language string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, method+":"+language)
}

func (f *fakeProvider) callsTo(method string) []string {
	var out []string
	for _, c := range f.calls {
		if len(c) > len(method) && c[:len(method)] == method {
			out = append(out, c)
		}
	}
	return out
}

func (f *fakeProvider) SearchMovies(_ context.Context, language, query string, year *int) ([]domain.MovieSummary, error) {
	f.record("SearchMovies", language)
	f.searchQuery, f.searchYear = query, year
	return []domain.MovieSummary{{ID: 1, Title: "Iron Man"}}, f.err
}

func (f *fakeProvider) SearchSeries(_ context.Context, language, query string, year *int) ([]domain.SeriesSummary, error) {
	f.record("SearchSeries", language)
	f.searchQuery, f.searchYear = query, year
	return nil, f.err
}

func (f *fakeProvider) Movie(_ context.Context, language string, _ int64) (domain.Movie, error) {
	f.record("Movie", language)
	return f.movie, f.err
}

func (f *fakeProvider) Series(_ context.Context, language string, _ int64) (domain.Series, error) {
	f.record("Series", language)
	return domain.Series{}, f.err
}

func (f *fakeProvider) Season(_ context.Context, language string, _ int64, _ int) (domain.Season, error) {
	f.record("Season", language)
	if f.err != nil {
		return domain.Season{}, f.err
	}
	return f.seasons[language], nil
}

func (f *fakeProvider) Collection(_ context.Context, language string, _ int64) (domain.Collection, error) {
	f.record("Collection", language)
	if f.err != nil {
		return domain.Collection{}, f.err
	}
	return f.collections[language], nil
}

func (f *fakeProvider) Genres(_ context.Context, language string, _ domain.MediaKind) ([]domain.Genre, error) {
	f.record("Genres", language)
	if f.err != nil {
		return nil, f.err
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

func TestNonPositiveIDIsNotFound(t *testing.T) {
	t.Parallel()

	svc := application.NewService(&fakeProvider{}, "es-MX")
	if _, err := svc.Movie(t.Context(), 0); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("Movie(0) err = %v, want ErrNotFound", err)
	}
}

func TestSeasonFillsMissingTextFromEnglish(t *testing.T) {
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
	if got := f.callsTo("Season"); len(got) != 2 {
		t.Errorf("season calls = %v, want es-MX then en-US", got)
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
	if got := f.callsTo("Season"); len(got) != 1 {
		t.Errorf("season calls = %v, want one (no English needed)", got)
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
}

func TestMovieNamesUnnamedGenresInEnglish(t *testing.T) {
	t.Parallel()

	f := &fakeProvider{
		movie: domain.Movie{ID: 1726, Genres: []domain.Genre{{ID: 28, Name: ""}, {ID: 878, Name: "Ciencia ficción"}}},
		genres: map[string][]domain.Genre{
			"en-US": {{ID: 28, Name: "Action"}, {ID: 878, Name: "Science Fiction"}},
		},
	}
	svc := application.NewService(f, "es-MX")

	m, err := svc.Movie(t.Context(), 1726)
	if err != nil {
		t.Fatalf("Movie: %v", err)
	}
	if m.Genres[0].Name != "Action" || m.Genres[1].Name != "Ciencia ficción" {
		t.Errorf("genres = %+v", m.Genres)
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
