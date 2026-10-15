package tmdb_test

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/metadata/domain"
	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/metadata/infrastructure/tmdb"
)

const testAPIKey = "SECRET-KEY-123"

// fakeTMDB serves fixed responses by request path (including the /3 prefix)
// and records how often each path was asked and with which query and auth.
type fakeTMDB struct {
	mu     sync.Mutex
	routes map[string]http.HandlerFunc
	hits   map[string]int
	query  map[string]url.Values
	auth   map[string]string
	srv    *httptest.Server
}

func newFakeTMDB(t *testing.T, routes map[string]http.HandlerFunc) *fakeTMDB {
	t.Helper()
	f := &fakeTMDB{routes: routes, hits: map[string]int{}, query: map[string]url.Values{}, auth: map[string]string{}}
	f.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		f.hits[r.URL.Path]++
		f.query[r.URL.Path] = r.URL.Query()
		f.auth[r.URL.Path] = r.Header.Get("Authorization")
		h, ok := f.routes[r.URL.Path]
		f.mu.Unlock()
		if !ok {
			http.NotFound(w, r)
			return
		}
		h(w, r)
	}))
	t.Cleanup(f.srv.Close)
	return f
}

func (f *fakeTMDB) hitCount(path string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.hits[path]
}

func (f *fakeTMDB) lastQuery(path string) url.Values {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.query[path]
}

func (f *fakeTMDB) lastAuth(path string) string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.auth[path]
}

func jsonOK(body string) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, body)
	}
}

func status(code int, body string) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(code)
		_, _ = io.WriteString(w, body)
	}
}

// newClient builds a client against the fake server with fast retries.
func newClient(t *testing.T, f *fakeTMDB, cfg tmdb.Config) *tmdb.Client {
	t.Helper()
	cfg.BaseURL = f.srv.URL + "/3"
	if cfg.APIKey == "" && cfg.ReadToken == "" {
		cfg.APIKey = testAPIKey
	}
	if cfg.RetryDelay == 0 {
		cfg.RetryDelay = time.Millisecond
	}
	c, err := tmdb.New(cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return c
}

func TestNewRequiresExactlyOneCredential(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		cfg     tmdb.Config
		wantErr bool
	}{
		{"no credentials", tmdb.Config{}, true},
		{"both credentials", tmdb.Config{APIKey: "k", ReadToken: "t"}, true},
		{"v3 key only", tmdb.Config{APIKey: "k"}, false},
		{"v4 token only", tmdb.Config{ReadToken: "t"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, err := tmdb.New(tt.cfg)
			if (err != nil) != tt.wantErr {
				t.Errorf("New err = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestV3KeyIsSentAsQueryAndNoBearer(t *testing.T) {
	t.Parallel()

	f := newFakeTMDB(t, map[string]http.HandlerFunc{"/3/search/movie": jsonOK(`{"results":[]}`)})
	c := newClient(t, f, tmdb.Config{APIKey: testAPIKey})

	if _, err := c.SearchMovies(t.Context(), "es-MX", "Iron Man", nil); err != nil {
		t.Fatalf("SearchMovies: %v", err)
	}
	if got := f.lastQuery("/3/search/movie").Get("api_key"); got != testAPIKey {
		t.Errorf("api_key = %q, want the configured key", got)
	}
	if got := f.lastAuth("/3/search/movie"); got != "" {
		t.Errorf("Authorization = %q, want none with a v3 key", got)
	}
}

func TestV4TokenIsSentAsBearerAndNoKeyQuery(t *testing.T) {
	t.Parallel()

	f := newFakeTMDB(t, map[string]http.HandlerFunc{"/3/search/movie": jsonOK(`{"results":[]}`)})
	c := newClient(t, f, tmdb.Config{ReadToken: "read-token"})

	if _, err := c.SearchMovies(t.Context(), "es-MX", "Iron Man", nil); err != nil {
		t.Fatalf("SearchMovies: %v", err)
	}
	if got := f.lastAuth("/3/search/movie"); got != "Bearer read-token" {
		t.Errorf("Authorization = %q, want the bearer token", got)
	}
	if _, ok := f.lastQuery("/3/search/movie")["api_key"]; ok {
		t.Error("api_key sent with a v4 token")
	}
}

func TestSearchMoviesSendsLanguageAndYear(t *testing.T) {
	t.Parallel()

	f := newFakeTMDB(t, map[string]http.HandlerFunc{"/3/search/movie": jsonOK(`{"results":[
		{"id":1726,"title":"Hombre de Hierro","original_title":"Iron Man","original_language":"en","release_date":"2008-05-02","poster_path":"/p1.jpg"},
		{"id":9,"title":"","original_title":"Only original","original_language":"ja","release_date":""}
	]}`)})
	c := newClient(t, f, tmdb.Config{})
	year := 2008

	got, err := c.SearchMovies(t.Context(), "es-MX", "Iron Man", &year)
	if err != nil {
		t.Fatalf("SearchMovies: %v", err)
	}
	q := f.lastQuery("/3/search/movie")
	if q.Get("language") != "es-MX" || q.Get("year") != "2008" || q.Get("query") != "Iron Man" {
		t.Errorf("query = %v", q)
	}
	if len(got) != 2 || got[0].Title != "Hombre de Hierro" || *got[0].Year != 2008 || got[0].PosterPath != "/p1.jpg" {
		t.Fatalf("results = %+v", got)
	}
	if got[1].Title != "Only original" || got[1].Year != nil || got[1].PosterPath != "" {
		t.Errorf("result without title or date = %+v", got[1])
	}
}

func TestFindByIMDbSendsExternalSourceAndMapsBothKinds(t *testing.T) {
	t.Parallel()

	f := newFakeTMDB(t, map[string]http.HandlerFunc{"/3/find/tt18412256": jsonOK(`{
		"movie_results":[{"id":1000001,"title":"Alien: Romulus","original_title":"Alien: Romulus","release_date":"2024-08-13"}],
		"tv_results":[]
	}`)})
	c := newClient(t, f, tmdb.Config{})

	got, err := c.FindByIMDb(t.Context(), "es-MX", "TT18412256")
	if err != nil {
		t.Fatalf("FindByIMDb: %v", err)
	}
	q := f.lastQuery("/3/find/tt18412256")
	if q.Get("external_source") != "imdb_id" || q.Get("language") != "es-MX" {
		t.Errorf("query = %v", q)
	}
	if len(got.Movies) != 1 || got.Movies[0].ID != 1000001 || *got.Movies[0].Year != 2024 {
		t.Errorf("movies = %+v", got.Movies)
	}
	if len(got.Series) != 0 {
		t.Errorf("series = %+v, want none", got.Series)
	}
}

func TestFindByIMDbRefusesAnIDThatIsNotOne(t *testing.T) {
	t.Parallel()

	f := newFakeTMDB(t, nil)
	c := newClient(t, f, tmdb.Config{})
	if _, err := c.FindByIMDb(t.Context(), "es-MX", "tt1/../search"); !errors.Is(err, domain.ErrInvalidQuery) {
		t.Errorf("err = %v, want ErrInvalidQuery", err)
	}
	if n := f.hitCount("/3/find/tt1/../search"); n != 0 {
		t.Errorf("the request was sent %d times", n)
	}
}

func TestSearchSeriesUsesFirstAirDateYear(t *testing.T) {
	t.Parallel()

	f := newFakeTMDB(t, map[string]http.HandlerFunc{"/3/search/tv": jsonOK(`{"results":[
		{"id":1396,"name":"Breaking Bad","original_name":"Breaking Bad","original_language":"en","first_air_date":"2008-01-20"}
	]}`)})
	c := newClient(t, f, tmdb.Config{})
	year := 2008

	got, err := c.SearchSeries(t.Context(), "es-MX", "Breaking Bad", &year)
	if err != nil {
		t.Fatalf("SearchSeries: %v", err)
	}
	if f.lastQuery("/3/search/tv").Get("first_air_date_year") != "2008" {
		t.Errorf("query = %v", f.lastQuery("/3/search/tv"))
	}
	if len(got) != 1 || got[0].Title != "Breaking Bad" || *got[0].Year != 2008 {
		t.Errorf("results = %+v", got)
	}
}

func TestMovieLocalizesWithEnglishFallback(t *testing.T) {
	t.Parallel()

	f := newFakeTMDB(t, map[string]http.HandlerFunc{
		"/3/movie/1726": jsonOK(`{"id":1726,"original_title":"Iron Man","original_language":"en",
			"release_date":"2008-05-02","runtime":126,"poster_path":"/poster.jpg","backdrop_path":"/backdrop.jpg",
			"genres":[{"id":28,"name":"Acción"}],
			"belongs_to_collection":{"id":86311,"name":"Colección","poster_path":"/col.jpg"}}`),
		"/3/movie/1726/translations": jsonOK(`{"id":1726,"translations":[
			{"iso_639_1":"en","iso_3166_1":"US","data":{"title":"Iron Man","overview":"English synopsis"}},
			{"iso_639_1":"es","iso_3166_1":"MX","data":{"title":"Hombre de Hierro","overview":""}}]}`),
		"/3/movie/1726/images": jsonOK(`{"logos":[
			{"file_path":"/logo-es.png","iso_639_1":"es"},
			{"file_path":"/logo-en.png","iso_639_1":"en"},
			{"file_path":"/logo-none.svg","iso_639_1":null},
			{"file_path":"/bad path.png","iso_639_1":"en"}]}`),
	})
	c := newClient(t, f, tmdb.Config{})

	m, err := c.Movie(t.Context(), "es-MX", 1726)
	if err != nil {
		t.Fatalf("Movie: %v", err)
	}
	if got := f.lastQuery("/3/movie/1726").Get("language"); got != "es-MX" {
		t.Errorf("language = %q, want es-MX", got)
	}
	if got := f.lastQuery("/3/movie/1726/images").Get("include_image_language"); got != "es,en,null" {
		t.Errorf("include_image_language = %q, want es,en,null", got)
	}
	if m.Title != "Hombre de Hierro" || m.EnglishTitle != "Iron Man" || m.OriginalTitle != "Iron Man" {
		t.Errorf("titles = %q / %q / %q", m.Title, m.EnglishTitle, m.OriginalTitle)
	}
	if m.Overview != "English synopsis" {
		t.Errorf("overview = %q, want the English one (es-MX had none)", m.Overview)
	}
	if m.Runtime != 126 || *m.Year != 2008 || m.OriginalLanguage != "en" {
		t.Errorf("runtime, year or language = %d, %v, %q", m.Runtime, m.Year, m.OriginalLanguage)
	}
	if m.Images.Poster != "/poster.jpg" || m.Images.Backdrop != "/backdrop.jpg" {
		t.Errorf("images = %+v", m.Images)
	}
	if len(m.Genres) != 1 || m.Genres[0].Name != "Acción" {
		t.Errorf("genres = %+v", m.Genres)
	}
	if m.Collection == nil || m.Collection.ID != 86311 || m.Collection.Name != "Colección" {
		t.Errorf("collection = %+v", m.Collection)
	}
	// The SVG logo and the malformed path are dropped: the app serves only JPEG and PNG (spec §6).
	wantLogos := []domain.Logo{{Path: "/logo-es.png", Language: "es"}, {Path: "/logo-en.png", Language: "en"}}
	if len(m.Images.Logos) != len(wantLogos) {
		t.Fatalf("logos = %+v, want %+v (SVG and malformed paths are dropped)", m.Images.Logos, wantLogos)
	}
	for i, l := range wantLogos {
		if m.Images.Logos[i] != l {
			t.Errorf("logo %d = %+v, want %+v", i, m.Images.Logos[i], l)
		}
	}
}

func TestMovieWithoutTranslationsUsesOriginalTitle(t *testing.T) {
	t.Parallel()

	f := newFakeTMDB(t, map[string]http.HandlerFunc{
		"/3/movie/5":              jsonOK(`{"id":5,"original_title":"Il Padrino","release_date":""}`),
		"/3/movie/5/translations": jsonOK(`{"id":5,"translations":[]}`),
		"/3/movie/5/images":       jsonOK(`{"logos":[]}`),
	})
	c := newClient(t, f, tmdb.Config{})

	m, err := c.Movie(t.Context(), "es-MX", 5)
	if err != nil {
		t.Fatalf("Movie: %v", err)
	}
	if m.Title != "Il Padrino" || m.EnglishTitle != "" || m.Year != nil || m.Overview != "" {
		t.Errorf("movie = %+v", m)
	}
	if got := f.lastQuery("/3/movie/5/images").Get("include_image_language"); got != "es,en,null" {
		t.Errorf("include_image_language = %q", got)
	}
}

func TestSeriesSeasonsAndEpisodes(t *testing.T) {
	t.Parallel()

	f := newFakeTMDB(t, map[string]http.HandlerFunc{
		"/3/tv/1396": jsonOK(`{"id":1396,"original_name":"Breaking Bad","original_language":"en","first_air_date":"2008-01-20",
			"number_of_episodes":62,"poster_path":"/tv.jpg","genres":[{"id":18,"name":"Drama"}],
			"seasons":[{"season_number":0,"episode_count":2,"air_date":""},{"season_number":1,"episode_count":7,"air_date":"2008-01-20","poster_path":"/s1.jpg"}]}`),
		"/3/tv/1396/translations": jsonOK(`{"translations":[{"iso_639_1":"en","iso_3166_1":"US","data":{"name":"Breaking Bad","overview":"Synopsis"}}]}`),
		"/3/tv/1396/images":       jsonOK(`{"logos":[{"file_path":"/logo.png","iso_639_1":"en"}]}`),
		"/3/tv/1396/season/1": jsonOK(`{"episodes":[
			{"id":1,"episode_number":1,"name":"Pilot","overview":"","air_date":"2008-01-20","runtime":58,"still_path":"/still.jpg"},
			{"id":2,"episode_number":2,"name":"","overview":"Second","air_date":null,"runtime":null,"still_path":null}]}`),
	})
	c := newClient(t, f, tmdb.Config{})

	s, err := c.Series(t.Context(), "es-MX", 1396)
	if err != nil {
		t.Fatalf("Series: %v", err)
	}
	if s.Title != "Breaking Bad" || s.Overview != "Synopsis" || *s.Year != 2008 || s.EpisodeCount != 62 {
		t.Errorf("series = %+v", s)
	}
	if len(s.Seasons) != 2 || s.Seasons[0].Number != 0 || s.Seasons[1].EpisodeCount != 7 || s.Seasons[1].PosterPath != "/s1.jpg" {
		t.Errorf("seasons = %+v", s.Seasons)
	}
	if !s.Seasons[1].AirDate.Equal(time.Date(2008, time.January, 20, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("season air date = %v", s.Seasons[1].AirDate)
	}
	if len(s.Images.Logos) != 1 || s.Images.Logos[0].Path != "/logo.png" {
		t.Errorf("logos = %+v", s.Images.Logos)
	}

	season, err := c.Season(t.Context(), "es-MX", 1396, 1)
	if err != nil {
		t.Fatalf("Season: %v", err)
	}
	if len(season.Episodes) != 2 {
		t.Fatalf("episodes = %+v", season.Episodes)
	}
	ep := season.Episodes[0]
	if ep.Number != 1 || ep.SeasonNumber != 1 || ep.Title != "Pilot" || ep.Overview != "" || ep.Runtime != 58 || ep.StillPath != "/still.jpg" {
		t.Errorf("episode 1 = %+v", ep)
	}
	if !season.Episodes[1].AirDate.IsZero() || season.Episodes[1].Runtime != 0 || season.Episodes[1].StillPath != "" {
		t.Errorf("episode 2 with nulls = %+v", season.Episodes[1])
	}
}

func TestCollectionAndGenres(t *testing.T) {
	t.Parallel()

	f := newFakeTMDB(t, map[string]http.HandlerFunc{
		"/3/collection/86311": jsonOK(`{"id":86311,"name":"Iron Man Collection","overview":"Saga","poster_path":"/c.jpg",
			"parts":[{"id":1726,"title":"Hombre de Hierro","original_title":"Iron Man","release_date":"2008-05-02","poster_path":"/p.jpg"}]}`),
		"/3/genre/movie/list": jsonOK(`{"genres":[{"id":28,"name":"Acción"}]}`),
		"/3/genre/tv/list":    jsonOK(`{"genres":[{"id":18,"name":"Drama"}]}`),
	})
	c := newClient(t, f, tmdb.Config{})

	col, err := c.Collection(t.Context(), "es-MX", 86311)
	if err != nil {
		t.Fatalf("Collection: %v", err)
	}
	if col.Name != "Iron Man Collection" || len(col.Parts) != 1 || *col.Parts[0].Year != 2008 || col.Parts[0].OriginalTitle != "Iron Man" {
		t.Errorf("collection = %+v", col)
	}

	movies, err := c.Genres(t.Context(), "es-MX", domain.KindMovie)
	if err != nil || len(movies) != 1 || movies[0].Name != "Acción" {
		t.Fatalf("movie genres = %+v, %v", movies, err)
	}
	series, err := c.Genres(t.Context(), "es-MX", domain.KindSeries)
	if err != nil || len(series) != 1 || series[0].Name != "Drama" {
		t.Fatalf("series genres = %+v, %v", series, err)
	}
	if _, err := c.Genres(t.Context(), "es-MX", domain.MediaKind("book")); err == nil {
		t.Error("unknown kind accepted")
	}
}

func TestRateLimitRetriesAfterRetryAfter(t *testing.T) {
	t.Parallel()

	var calls int
	var mu sync.Mutex
	f := newFakeTMDB(t, map[string]http.HandlerFunc{"/3/search/movie": func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		calls++
		first := calls == 1
		mu.Unlock()
		if first {
			w.Header().Set("Retry-After", "0")
			http.Error(w, `{"status_message":"slow down"}`, http.StatusTooManyRequests)
			return
		}
		jsonOK(`{"results":[{"id":1,"title":"Ok"}]}`)(w, r)
	}})
	c := newClient(t, f, tmdb.Config{})

	got, err := c.SearchMovies(t.Context(), "es-MX", "Iron Man", nil)
	if err != nil {
		t.Fatalf("SearchMovies after one 429: %v", err)
	}
	if len(got) != 1 || f.hitCount("/3/search/movie") != 2 {
		t.Errorf("results = %+v after %d hits, want one result after a retry", got, f.hitCount("/3/search/movie"))
	}
}

func TestRetryAfterOverTheLimitFailsAtOnce(t *testing.T) {
	t.Parallel()

	f := newFakeTMDB(t, map[string]http.HandlerFunc{"/3/search/movie": func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "3600")
		http.Error(w, "", http.StatusTooManyRequests)
	}})
	c := newClient(t, f, tmdb.Config{MaxRetryAfter: time.Second})

	start := time.Now()
	_, err := c.SearchMovies(t.Context(), "es-MX", "Iron Man", nil)
	if !errors.Is(err, domain.ErrUpstream) {
		t.Fatalf("err = %v, want ErrUpstream", err)
	}
	if f.hitCount("/3/search/movie") != 1 || time.Since(start) > 500*time.Millisecond {
		t.Errorf("hits = %d after %s, want no wait and no retry", f.hitCount("/3/search/movie"), time.Since(start))
	}
}

func TestServerErrorsRetryThenFail(t *testing.T) {
	t.Parallel()

	f := newFakeTMDB(t, map[string]http.HandlerFunc{"/3/movie/7": status(http.StatusServiceUnavailable, `{}`)})
	c := newClient(t, f, tmdb.Config{MaxAttempts: 3})

	_, err := c.Movie(t.Context(), "es-MX", 7)
	if !errors.Is(err, domain.ErrUpstream) {
		t.Fatalf("err = %v, want ErrUpstream", err)
	}
	if got := f.hitCount("/3/movie/7"); got != 3 {
		t.Errorf("attempts = %d, want 3", got)
	}
}

func TestNotFoundIsNotRetried(t *testing.T) {
	t.Parallel()

	f := newFakeTMDB(t, nil)
	c := newClient(t, f, tmdb.Config{MaxAttempts: 3})

	if _, err := c.Movie(t.Context(), "es-MX", 999); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
	if got := f.hitCount("/3/movie/999"); got != 1 {
		t.Errorf("attempts = %d, want 1", got)
	}
}

func TestRejectedCredentialsAreUpstreamErrors(t *testing.T) {
	t.Parallel()

	f := newFakeTMDB(t, map[string]http.HandlerFunc{"/3/movie/1": status(http.StatusUnauthorized, `{"status_message":"Invalid API key"}`)})
	c := newClient(t, f, tmdb.Config{})

	_, err := c.Movie(t.Context(), "es-MX", 1)
	if !errors.Is(err, domain.ErrUpstream) || !strings.Contains(err.Error(), "credentials rejected") {
		t.Fatalf("err = %v, want an upstream credentials error", err)
	}
	if got := f.hitCount("/3/movie/1"); got != 1 {
		t.Errorf("attempts = %d, want 1 (no retry for 401)", got)
	}
}

// countingTransport counts the attempts that leave the client. A 30 ms attempt
// timeout can fire before a request reaches the fake server, so counting on
// the server side would depend on how fast the machine is.
type countingTransport struct {
	next  http.RoundTripper
	calls atomic.Int32
}

func (c *countingTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	c.calls.Add(1)
	return c.next.RoundTrip(r)
}

func TestTimeoutIsRetriedAndReported(t *testing.T) {
	t.Parallel()

	slow := func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-time.After(2 * time.Second):
		}
	}
	f := newFakeTMDB(t, map[string]http.HandlerFunc{"/3/movie/2": slow})
	transport := &countingTransport{next: http.DefaultTransport}
	c := newClient(t, f, tmdb.Config{
		RequestTimeout: 30 * time.Millisecond,
		MaxAttempts:    2,
		HTTPClient:     &http.Client{Transport: transport},
	})

	_, err := c.Movie(t.Context(), "es-MX", 2)
	if !errors.Is(err, domain.ErrUpstream) {
		t.Fatalf("err = %v, want ErrUpstream", err)
	}
	if got := transport.calls.Load(); got != 2 {
		t.Errorf("attempts = %d, want 2", got)
	}
}

func TestCallerCancellationStopsRequest(t *testing.T) {
	t.Parallel()

	// The handler reports its arrival, then blocks until the client gives up.
	// The test cancels only after the request is in flight, so the outcome does
	// not depend on how fast the machine is. The timeouts below are only guards
	// for a broken client; they never decide a passing run.
	arrived := make(chan struct{}, 1)
	slow := func(w http.ResponseWriter, r *http.Request) {
		select {
		case arrived <- struct{}{}:
		default:
		}
		select {
		case <-r.Context().Done():
		case <-time.After(10 * time.Second):
		}
	}
	f := newFakeTMDB(t, map[string]http.HandlerFunc{"/3/movie/3": slow})
	c := newClient(t, f, tmdb.Config{MaxAttempts: 3})

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	result := make(chan error, 1)
	go func() {
		_, err := c.Movie(ctx, "es-MX", 3)
		result <- err
	}()

	select {
	case <-arrived:
	case <-time.After(10 * time.Second):
		t.Fatal("the request never reached the server")
	}
	cancel()

	select {
	case err := <-result:
		if err == nil {
			t.Fatal("Movie with a cancelled context returned no error")
		}
	case <-time.After(10 * time.Second):
		t.Fatal("Movie did not return after the caller cancelled")
	}
	if got := f.hitCount("/3/movie/3"); got != 1 {
		t.Errorf("attempts = %d, want 1 (the caller gave up)", got)
	}
}

func TestMalformedResponsesAreUpstreamErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		path string
		body string
		call func(ctx context.Context, c *tmdb.Client) error
	}{
		{
			name: "invalid JSON",
			path: "/3/movie/4",
			body: `{not json`,
			call: func(ctx context.Context, c *tmdb.Client) error { _, err := c.Movie(ctx, "es-MX", 4); return err },
		},
		{
			name: "movie without id",
			path: "/3/movie/4",
			body: `{"original_title":"x"}`,
			call: func(ctx context.Context, c *tmdb.Client) error { _, err := c.Movie(ctx, "es-MX", 4); return err },
		},
		{
			name: "search result without id",
			path: "/3/search/movie",
			body: `{"results":[{"title":"x"}]}`,
			call: func(ctx context.Context, c *tmdb.Client) error {
				_, err := c.SearchMovies(ctx, "es-MX", "x", nil)
				return err
			},
		},
		{
			name: "episode without number",
			path: "/3/tv/4/season/1",
			body: `{"episodes":[{"id":9,"episode_number":0}]}`,
			call: func(ctx context.Context, c *tmdb.Client) error { _, err := c.Season(ctx, "es-MX", 4, 1); return err },
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			f := newFakeTMDB(t, map[string]http.HandlerFunc{tt.path: jsonOK(tt.body)})
			c := newClient(t, f, tmdb.Config{})
			if err := tt.call(t.Context(), c); !errors.Is(err, domain.ErrUpstream) {
				t.Errorf("err = %v, want ErrUpstream", err)
			}
		})
	}
}

func TestCredentialNeverAppearsInErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		cfg    tmdb.Config
		secret string
	}{
		{"v3 key echoed by the server", tmdb.Config{APIKey: testAPIKey}, testAPIKey},
		{"v4 token echoed by the server", tmdb.Config{ReadToken: "READ-TOKEN-XYZ"}, "READ-TOKEN-XYZ"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			echo := status(http.StatusBadRequest, `{"status_message":"bad credential `+tt.secret+`"}`)
			f := newFakeTMDB(t, map[string]http.HandlerFunc{"/3/movie/8": echo})
			c := newClient(t, f, tmdb.Config{APIKey: tt.cfg.APIKey, ReadToken: tt.cfg.ReadToken})

			_, err := c.Movie(t.Context(), "es-MX", 8)
			if err == nil {
				t.Fatal("expected an error")
			}
			if strings.Contains(err.Error(), tt.secret) {
				t.Errorf("error leaks the credential: %v", err)
			}
		})
	}
}

func TestTransportErrorDoesNotLeakTheKey(t *testing.T) {
	t.Parallel()

	f := newFakeTMDB(t, nil)
	c := newClient(t, f, tmdb.Config{APIKey: testAPIKey, MaxAttempts: 2})
	f.srv.Close() // the connection fails; the error carries the request URL

	_, err := c.Movie(t.Context(), "es-MX", 8)
	if !errors.Is(err, domain.ErrUpstream) {
		t.Fatalf("err = %v, want ErrUpstream", err)
	}
	if strings.Contains(err.Error(), testAPIKey) || strings.Contains(err.Error(), "api_key") {
		t.Errorf("transport error leaks the key: %v", err)
	}
}

func TestImageURLBuildsSizesOfSpec(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		kind domain.ImageKind
		path string
		want string
		ok   bool
	}{
		{"poster w500", domain.ImagePoster, "/p.jpg", "https://image.tmdb.org/t/p/w500/p.jpg", true},
		{"backdrop w1280", domain.ImageBackdrop, "/b.jpg", "https://image.tmdb.org/t/p/w1280/b.jpg", true},
		{"logo w500", domain.ImageLogo, "/l.png", "https://image.tmdb.org/t/p/w500/l.png", true},
		{"still w300", domain.ImageStill, "/s.jpg", "https://image.tmdb.org/t/p/w300/s.jpg", true},
		{"svg rejected", domain.ImageLogo, "/l.svg", "", false},
		{"query injection", domain.ImagePoster, "/p.jpg?api_key=x", "", false},
		{"path traversal", domain.ImagePoster, "/../p.jpg", "", false},
		{"no leading slash", domain.ImagePoster, "p.jpg", "", false},
		{"unknown kind", domain.ImageKind("avatar"), "/p.jpg", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, ok := tmdb.ImageURL(tt.kind, tt.path)
			if ok != tt.ok || got != tt.want {
				t.Errorf("ImageURL = (%q, %v), want (%q, %v)", got, ok, tt.want, tt.ok)
			}
		})
	}
}

// A series whose original language is the requested one, and that TMDB does
// not translate, keeps its original title in es-MX (RF-61), not the English one.
func TestSeriesKeepsSpanishOriginalTitle(t *testing.T) {
	t.Parallel()

	f := newFakeTMDB(t, map[string]http.HandlerFunc{
		"/3/tv/71446": jsonOK(`{"id":71446,"original_name":"La casa de papel","original_language":"es",
			"first_air_date":"2017-05-02","number_of_episodes":0,"genres":[],"seasons":[]}`),
		"/3/tv/71446/translations": jsonOK(`{"id":71446,"translations":[
			{"iso_639_1":"en","iso_3166_1":"US","data":{"name":"Money Heist","overview":"English synopsis"}}]}`),
		"/3/tv/71446/images": jsonOK(`{"logos":[]}`),
	})
	c := newClient(t, f, tmdb.Config{})

	s, err := c.Series(t.Context(), "es-MX", 71446)
	if err != nil {
		t.Fatalf("Series: %v", err)
	}
	if s.Title != "La casa de papel" || s.EnglishTitle != "Money Heist" || s.OriginalTitle != "La casa de papel" {
		t.Errorf("titles = %q / %q / %q", s.Title, s.EnglishTitle, s.OriginalTitle)
	}
	if s.Overview != "English synopsis" {
		t.Errorf("overview = %q, want the English one", s.Overview)
	}
}
