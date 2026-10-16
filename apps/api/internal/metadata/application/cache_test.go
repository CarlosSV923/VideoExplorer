package application_test

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/metadata/application"
	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/metadata/domain"
)

var t0 = time.Date(2026, time.October, 10, 12, 0, 0, 0, time.UTC)

// clock is a fixed clock the tests move by hand.
type clock struct {
	mu  sync.Mutex
	now time.Time
}

func (c *clock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *clock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}

// memStore is an in-memory ResponseStore that can be made to fail.
type memStore struct {
	mu     sync.Mutex
	items  map[string]application.CachedResponse
	getErr error
	putErr error
	puts   int
}

func newMemStore() *memStore {
	return &memStore{items: map[string]application.CachedResponse{}}
}

func (s *memStore) Get(_ context.Context, key string) (application.CachedResponse, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.getErr != nil {
		return application.CachedResponse{}, false, s.getErr
	}
	e, ok := s.items[key]
	return e, ok, nil
}

func (s *memStore) Put(_ context.Context, key string, entry application.CachedResponse) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.puts++
	if s.putErr != nil {
		return s.putErr
	}
	s.items[key] = entry
	return nil
}

// corrupt replaces every payload with something that is not JSON of the answer.
func (s *memStore) corrupt() {
	s.mu.Lock()
	defer s.mu.Unlock()
	for k, e := range s.items {
		e.Payload = []byte("{not json")
		s.items[k] = e
	}
}

// countingProvider counts the calls of each method. Each successful answer is
// labelled "<language>-<n>", where n is the number of the call, so a test can
// tell which answer it got. A non-nil err fails every call.
type countingProvider struct {
	mu    sync.Mutex
	calls map[string]int
	err   error
}

func newCountingProvider() *countingProvider {
	return &countingProvider{calls: map[string]int{}}
}

// record counts a call and returns its label, or the configured error.
func (p *countingProvider) record(method, language string) (string, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.calls[method]++
	if p.err != nil {
		return "", p.err
	}
	return fmt.Sprintf("%s-%d", language, p.calls[method]), nil
}

func (p *countingProvider) count(method string) int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.calls[method]
}

func (p *countingProvider) setErr(err error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.err = err
}

func (p *countingProvider) SearchMovies(_ context.Context, language, _ string, _ *int) ([]domain.MovieSummary, error) {
	label, err := p.record("SearchMovies", language)
	return []domain.MovieSummary{{ID: 1, Title: label}}, err
}

func (p *countingProvider) SearchSeries(_ context.Context, language, _ string, _ *int) ([]domain.SeriesSummary, error) {
	label, err := p.record("SearchSeries", language)
	return []domain.SeriesSummary{{ID: 1, Title: label}}, err
}

func (p *countingProvider) FindByIMDb(_ context.Context, language, _ string) (domain.FindResult, error) {
	label, err := p.record("FindByIMDb", language)
	return domain.FindResult{Movies: []domain.MovieSummary{{ID: 1, Title: label}}}, err
}

func (p *countingProvider) Movie(_ context.Context, language string, id int64) (domain.Movie, error) {
	label, err := p.record("Movie", language)
	return domain.Movie{ID: id, Title: label}, err
}

func (p *countingProvider) Series(_ context.Context, language string, id int64) (domain.Series, error) {
	label, err := p.record("Series", language)
	return domain.Series{ID: id, Title: label}, err
}

func (p *countingProvider) Season(_ context.Context, language string, seriesID int64, number int) (domain.Season, error) {
	label, err := p.record("Season", language)
	return domain.Season{SeriesID: seriesID, Number: number, Episodes: []domain.Episode{{Number: 1, Title: label}}}, err
}

func (p *countingProvider) Collection(_ context.Context, language string, id int64) (domain.Collection, error) {
	label, err := p.record("Collection", language)
	return domain.Collection{ID: id, Name: label}, err
}

func (p *countingProvider) Genres(_ context.Context, language string, _ domain.MediaKind) ([]domain.Genre, error) {
	label, err := p.record("Genres", language)
	return []domain.Genre{{ID: 1, Name: label}}, err
}

// newCached wires the decorator over a counting provider, an in-memory store and a clock at t0.
func newCached(t *testing.T) (*application.CachedProvider, *countingProvider, *memStore, *clock) {
	t.Helper()

	p := newCountingProvider()
	s := newMemStore()
	c := &clock{now: t0}
	return application.NewCachedProvider(p, s, c.Now, nil), p, s, c
}

func TestCachedProviderLifetimesByKind(t *testing.T) {
	t.Parallel()

	const day = 24 * time.Hour
	tests := []struct {
		name   string
		method string
		ttl    time.Duration
		call   func(ctx context.Context, p *application.CachedProvider) error
	}{
		{"search movies 24 h", "SearchMovies", day, func(ctx context.Context, p *application.CachedProvider) error {
			_, err := p.SearchMovies(ctx, "es-MX", "iron man", nil)
			return err
		}},
		{"search series 24 h", "SearchSeries", day, func(ctx context.Context, p *application.CachedProvider) error {
			_, err := p.SearchSeries(ctx, "es-MX", "breaking bad", nil)
			return err
		}},
		{"series 24 h", "Series", day, func(ctx context.Context, p *application.CachedProvider) error {
			_, err := p.Series(ctx, "es-MX", 1396)
			return err
		}},
		{"season 24 h", "Season", day, func(ctx context.Context, p *application.CachedProvider) error {
			_, err := p.Season(ctx, "es-MX", 1396, 1)
			return err
		}},
		{"find by IMDb id 24 h", "FindByIMDb", day, func(ctx context.Context, p *application.CachedProvider) error {
			_, err := p.FindByIMDb(ctx, "es-MX", "tt18412256")
			return err
		}},
		{"movie 30 days", "Movie", 30 * day, func(ctx context.Context, p *application.CachedProvider) error {
			_, err := p.Movie(ctx, "es-MX", 1726)
			return err
		}},
		{"collection 30 days", "Collection", 30 * day, func(ctx context.Context, p *application.CachedProvider) error {
			_, err := p.Collection(ctx, "es-MX", 86311)
			return err
		}},
		{"genres 30 days", "Genres", 30 * day, func(ctx context.Context, p *application.CachedProvider) error {
			_, err := p.Genres(ctx, "es-MX", domain.KindMovie)
			return err
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			p, provider, _, c := newCached(t)
			ctx := t.Context()

			if err := tt.call(ctx, p); err != nil {
				t.Fatal(err)
			}
			if err := tt.call(ctx, p); err != nil {
				t.Fatal(err)
			}
			if got := provider.count(tt.method); got != 1 {
				t.Fatalf("provider calls right after the first answer = %d, want 1", got)
			}

			c.Advance(tt.ttl - time.Minute)
			if err := tt.call(ctx, p); err != nil {
				t.Fatal(err)
			}
			if got := provider.count(tt.method); got != 1 {
				t.Errorf("provider calls one minute before expiry = %d, want 1 (still fresh)", got)
			}

			c.Advance(time.Minute)
			if err := tt.call(ctx, p); err != nil {
				t.Fatal(err)
			}
			if got := provider.count(tt.method); got != 2 {
				t.Errorf("provider calls at expiry = %d, want 2 (stale, asked again)", got)
			}
		})
	}
}

func TestCachedProviderKeyIncludesLanguageAndParameters(t *testing.T) {
	t.Parallel()

	p, provider, _, _ := newCached(t)
	ctx := t.Context()
	year := 2008

	calls := []struct {
		name string
		call func() (string, error)
	}{
		{"movie es-MX", func() (string, error) {
			m, err := p.Movie(ctx, "es-MX", 1726)
			return m.Title, err
		}},
		{"movie en-US", func() (string, error) {
			m, err := p.Movie(ctx, "en-US", 1726)
			return m.Title, err
		}},
		{"search without year", func() (string, error) {
			r, err := p.SearchMovies(ctx, "es-MX", "iron man", nil)
			return r[0].Title, err
		}},
		{"search with year", func() (string, error) {
			r, err := p.SearchMovies(ctx, "es-MX", "iron man", &year)
			return r[0].Title, err
		}},
		{"search other query", func() (string, error) {
			r, err := p.SearchMovies(ctx, "es-MX", "thor", nil)
			return r[0].Title, err
		}},
	}
	labels := map[string]string{}
	for _, c := range calls {
		label, err := c.call()
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		labels[c.name] = label
	}
	if got := provider.count("Movie") + provider.count("SearchMovies"); got != len(calls) {
		t.Fatalf("provider calls = %d, want %d: every distinct key is its own entry", got, len(calls))
	}

	// Each key now answers from its own entry, with the label it was stored with.
	for _, c := range calls {
		label, err := c.call()
		if err != nil {
			t.Fatalf("%s again: %v", c.name, err)
		}
		if label != labels[c.name] {
			t.Errorf("%s answered %q the second time, want the stored %q", c.name, label, labels[c.name])
		}
	}
	if got := provider.count("Movie") + provider.count("SearchMovies"); got != len(calls) {
		t.Errorf("provider calls after the second round = %d, want still %d", got, len(calls))
	}
}

func TestCachedProviderServesStaleWhenTMDBFails(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		err  error
	}{
		{"upstream", domain.ErrUpstream},
		{"wrapped upstream (Retry-After over the limit)", fmt.Errorf("%w: rate limited, Retry-After 1h0m0s is over the limit", domain.ErrUpstream)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			p, provider, store, c := newCached(t)
			ctx := t.Context()
			first, err := p.Series(ctx, "es-MX", 1396)
			if err != nil {
				t.Fatal(err)
			}
			putsBefore := store.puts

			c.Advance(25 * time.Hour)
			provider.setErr(tt.err)
			got, err := p.Series(ctx, "es-MX", 1396)
			if err != nil {
				t.Fatalf("err = %v, want the stale answer", err)
			}
			if got.Title != first.Title {
				t.Errorf("title = %q, want the stale %q", got.Title, first.Title)
			}
			if store.puts != putsBefore {
				t.Errorf("a failed reload wrote to the store (%d puts, was %d)", store.puts, putsBefore)
			}

			// When TMDB answers again, the new answer replaces the stale one.
			provider.setErr(nil)
			got, err = p.Series(ctx, "es-MX", 1396)
			if err != nil {
				t.Fatal(err)
			}
			if got.Title == first.Title {
				t.Errorf("title still %q after TMDB recovered, want a new answer", got.Title)
			}
		})
	}
}

func TestCachedProviderFailsWithoutAStoredAnswer(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		prepare func(t *testing.T, p *application.CachedProvider, provider *countingProvider, c *clock)
		err     error
		want    error
	}{
		{
			name: "no entry and TMDB down: the error goes up",
			err:  domain.ErrUpstream,
			want: domain.ErrUpstream,
		},
		{
			name: "stale entry and TMDB says not found: not served as stale",
			prepare: func(t *testing.T, p *application.CachedProvider, _ *countingProvider, c *clock) {
				t.Helper()
				if _, err := p.Movie(t.Context(), "es-MX", 1); err != nil {
					t.Fatal(err)
				}
				c.Advance(31 * 24 * time.Hour)
			},
			err:  domain.ErrNotFound,
			want: domain.ErrNotFound,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			p, provider, _, c := newCached(t)
			if tt.prepare != nil {
				tt.prepare(t, p, provider, c)
			}
			provider.setErr(tt.err)

			_, err := p.Movie(t.Context(), "es-MX", 1)
			if !errors.Is(err, tt.want) {
				t.Errorf("err = %v, want %v", err, tt.want)
			}
		})
	}
}

func TestCachedProviderDoesNotStoreErrors(t *testing.T) {
	t.Parallel()

	p, provider, store, _ := newCached(t)
	provider.setErr(domain.ErrNotFound)
	ctx := t.Context()

	for range 2 {
		if _, err := p.Movie(ctx, "es-MX", 1); !errors.Is(err, domain.ErrNotFound) {
			t.Fatalf("err = %v, want ErrNotFound", err)
		}
	}
	if got := provider.count("Movie"); got != 2 {
		t.Errorf("provider calls = %d, want 2: an error is asked again, not remembered", got)
	}
	if store.puts != 0 {
		t.Errorf("store puts = %d, want 0", store.puts)
	}
}

func TestCachedProviderForceRefreshIgnoresTheCache(t *testing.T) {
	t.Parallel()

	p, provider, _, _ := newCached(t)
	ctx := t.Context()
	if _, err := p.Movie(ctx, "es-MX", 1726); err != nil {
		t.Fatal(err)
	}

	forced := application.ForceRefresh(ctx)
	got, err := p.Movie(forced, "es-MX", 1726)
	if err != nil {
		t.Fatal(err)
	}
	if provider.count("Movie") != 2 {
		t.Fatalf("provider calls = %d, want 2: a forced refresh asks TMDB even when fresh", provider.count("Movie"))
	}
	if got.Title != "es-MX-2" {
		t.Errorf("title = %q, want the new answer", got.Title)
	}

	// The new answer is what the next ordinary read gets, within the lifetime.
	again, err := p.Movie(ctx, "es-MX", 1726)
	if err != nil {
		t.Fatal(err)
	}
	if again.Title != "es-MX-2" || provider.count("Movie") != 2 {
		t.Errorf("ordinary read after refresh: title %q, provider calls %d; want the stored new answer without asking TMDB",
			again.Title, provider.count("Movie"))
	}
}

func TestCachedProviderForceRefreshFailureIsAnError(t *testing.T) {
	t.Parallel()

	p, provider, _, _ := newCached(t)
	ctx := t.Context()
	if _, err := p.Movie(ctx, "es-MX", 1726); err != nil {
		t.Fatal(err)
	}

	// The fresh entry exists, but the caller asked for new data: TMDB's failure must reach it.
	provider.setErr(domain.ErrUpstream)
	if _, err := p.Movie(application.ForceRefresh(ctx), "es-MX", 1726); !errors.Is(err, domain.ErrUpstream) {
		t.Fatalf("err = %v, want ErrUpstream", err)
	}
}

func TestCachedProviderRefreshDoesNotCommitWhenOneRequestFails(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		fail bool
	}{
		{name: "all TMDB requests succeed", fail: false},
		{name: "a request fails halfway", fail: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			p, provider, store, _ := newCached(t)
			ctx := t.Context()
			oldMovie, err := p.Movie(ctx, "es-MX", 1726)
			if err != nil {
				t.Fatal(err)
			}
			oldSeries, err := p.Series(ctx, "es-MX", 1396)
			if err != nil {
				t.Fatal(err)
			}
			putsBefore := store.puts

			refreshCtx, tx := application.BeginRefresh(application.ForceRefresh(ctx))
			if _, err := p.Movie(refreshCtx, "es-MX", 1726); err != nil {
				t.Fatal(err)
			}
			if tt.fail {
				provider.setErr(domain.ErrUpstream)
			}
			if _, err := p.Series(refreshCtx, "es-MX", 1396); tt.fail {
				if !errors.Is(err, domain.ErrUpstream) {
					t.Fatalf("Series refresh err = %v, want ErrUpstream", err)
				}
			} else if err != nil {
				t.Fatal(err)
			}
			provider.setErr(nil)

			if !tt.fail {
				if err := tx.Commit(ctx); err != nil {
					t.Fatalf("Commit: %v", err)
				}
			}
			gotMovie, err := p.Movie(ctx, "es-MX", 1726)
			if err != nil {
				t.Fatal(err)
			}
			gotSeries, err := p.Series(ctx, "es-MX", 1396)
			if err != nil {
				t.Fatal(err)
			}
			if tt.fail {
				if gotMovie.Title != oldMovie.Title || gotSeries.Title != oldSeries.Title {
					t.Errorf("cache after failed refresh = movie %q, series %q; want %q, %q", gotMovie.Title, gotSeries.Title, oldMovie.Title, oldSeries.Title)
				}
				if store.puts != putsBefore {
					t.Errorf("failed refresh writes = %d, want %d", store.puts, putsBefore)
				}
			} else if gotMovie.Title == oldMovie.Title || gotSeries.Title == oldSeries.Title {
				t.Errorf("cache after successful refresh = movie %q, series %q; want new answers", gotMovie.Title, gotSeries.Title)
			}
		})
	}
}

func TestCachedProviderStoreProblemsAreNotFatal(t *testing.T) {
	t.Parallel()

	t.Run("read fails: TMDB is asked and answers", func(t *testing.T) {
		t.Parallel()

		p, provider, store, _ := newCached(t)
		store.getErr = errors.New("disk on fire")

		got, err := p.Movie(t.Context(), "es-MX", 1)
		if err != nil {
			t.Fatalf("err = %v, want the answer", err)
		}
		if got.Title != "es-MX-1" || provider.count("Movie") != 1 {
			t.Errorf("title %q, provider calls %d; want the answer from TMDB", got.Title, provider.count("Movie"))
		}
	})

	t.Run("write fails: the answer is still returned", func(t *testing.T) {
		t.Parallel()

		p, _, store, _ := newCached(t)
		store.putErr = errors.New("disk full")

		if _, err := p.Movie(t.Context(), "es-MX", 1); err != nil {
			t.Fatalf("err = %v, want the answer", err)
		}
	})

	t.Run("unreadable entry is treated as missing", func(t *testing.T) {
		t.Parallel()

		p, provider, store, _ := newCached(t)
		ctx := t.Context()
		if _, err := p.Movie(ctx, "es-MX", 1); err != nil {
			t.Fatal(err)
		}
		store.corrupt()

		got, err := p.Movie(ctx, "es-MX", 1)
		if err != nil {
			t.Fatal(err)
		}
		if provider.count("Movie") != 2 || got.Title != "es-MX-2" {
			t.Errorf("provider calls %d, title %q; want TMDB asked again", provider.count("Movie"), got.Title)
		}
	})
}
