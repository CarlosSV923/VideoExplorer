package application

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"strconv"
	"time"

	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/metadata/domain"
)

// Lifetimes of the cached TMDB answers (spec §6).
const (
	// searchTTL covers the search results.
	searchTTL = 24 * time.Hour
	// seriesTTL covers series, seasons and their chapters: new episodes appear often.
	seriesTTL = 24 * time.Hour
	// stableTTL covers movies, collections and genres, which change rarely.
	stableTTL = 30 * 24 * time.Hour
)

// cacheSchema prefixes every key. Bump it when the JSON of a cached answer
// changes shape, so old entries are never decoded into the new types.
const cacheSchema = "v1"

// ResponseStore keeps the answers of TMDB between runs. The decorator owns the
// encoding of the payload and decides the expiry; the store only saves them.
type ResponseStore interface {
	// Get returns the entry under key. found is false when there is none.
	Get(ctx context.Context, key string) (entry CachedResponse, found bool, err error)
	// Put stores entry under key, replacing any previous one.
	Put(ctx context.Context, key string, entry CachedResponse) error
}

// CachedResponse is one stored answer: its JSON payload and the instant after
// which it is stale. A stale answer is still kept, as the fallback when TMDB fails.
type CachedResponse struct {
	Payload   []byte
	ExpiresAt time.Time
}

// refreshKey is the context key of ForceRefresh.
type refreshKey struct{}

// ForceRefresh returns a context that makes the cached Provider ask TMDB even
// when the answer is fresh, and store the new one (the «Actualizar datos»
// action, spec §6 and RF-44). When TMDB fails in that case the error is
// returned: the caller asked for new data and must learn that it did not get it.
func ForceRefresh(ctx context.Context) context.Context {
	return context.WithValue(ctx, refreshKey{}, true)
}

func refreshRequested(ctx context.Context) bool {
	forced, _ := ctx.Value(refreshKey{}).(bool)
	return forced
}

// CachedProvider is a Provider that answers from a ResponseStore while the
// answers are fresh (spec §6). A stale answer is requested again; if TMDB
// fails with ErrUpstream (including a Retry-After longer than the limit), the
// stale answer is served instead. Without a stored answer the error goes up,
// and the caller treats TMDB as unavailable. Errors such as ErrNotFound are
// never stored, so they are asked again next time.
type CachedProvider struct {
	next  Provider
	store ResponseStore
	now   func() time.Time
	log   *slog.Logger
}

var _ Provider = (*CachedProvider)(nil)

// NewCachedProvider wraps next with the cache. now is the clock (time.Now in
// production, a fixed one in tests); log may be nil.
func NewCachedProvider(next Provider, store ResponseStore, now func() time.Time, log *slog.Logger) *CachedProvider {
	if log == nil {
		log = slog.New(slog.DiscardHandler)
	}
	return &CachedProvider{next: next, store: store, now: now, log: log}
}

// SearchMovies answers from the cache while fresh (24 h).
func (c *CachedProvider) SearchMovies(ctx context.Context, language, query string, year *int) ([]domain.MovieSummary, error) {
	params := url.Values{"q": {query}}
	setYear(params, year)
	return cached(ctx, c, key("search/movie", language, params), searchTTL, func(ctx context.Context) ([]domain.MovieSummary, error) {
		return c.next.SearchMovies(ctx, language, query, year)
	})
}

// SearchSeries answers from the cache while fresh (24 h).
func (c *CachedProvider) SearchSeries(ctx context.Context, language, query string, year *int) ([]domain.SeriesSummary, error) {
	params := url.Values{"q": {query}}
	setYear(params, year)
	return cached(ctx, c, key("search/tv", language, params), searchTTL, func(ctx context.Context) ([]domain.SeriesSummary, error) {
		return c.next.SearchSeries(ctx, language, query, year)
	})
}

// FindByIMDb answers from the cache while fresh (24 h, like a search).
func (c *CachedProvider) FindByIMDb(ctx context.Context, language, imdbID string) (domain.FindResult, error) {
	return cached(ctx, c, key("find/imdb/"+imdbID, language, nil), searchTTL, func(ctx context.Context) (domain.FindResult, error) {
		return c.next.FindByIMDb(ctx, language, imdbID)
	})
}

// Movie answers from the cache while fresh (30 days).
func (c *CachedProvider) Movie(ctx context.Context, language string, id int64) (domain.Movie, error) {
	return cached(ctx, c, key("movie/"+strconv.FormatInt(id, 10), language, nil), stableTTL, func(ctx context.Context) (domain.Movie, error) {
		return c.next.Movie(ctx, language, id)
	})
}

// Series answers from the cache while fresh (24 h).
func (c *CachedProvider) Series(ctx context.Context, language string, id int64) (domain.Series, error) {
	return cached(ctx, c, key("tv/"+strconv.FormatInt(id, 10), language, nil), seriesTTL, func(ctx context.Context) (domain.Series, error) {
		return c.next.Series(ctx, language, id)
	})
}

// Season answers from the cache while fresh (24 h, with its chapters).
func (c *CachedProvider) Season(ctx context.Context, language string, seriesID int64, number int) (domain.Season, error) {
	path := "tv/" + strconv.FormatInt(seriesID, 10) + "/season/" + strconv.Itoa(number)
	return cached(ctx, c, key(path, language, nil), seriesTTL, func(ctx context.Context) (domain.Season, error) {
		return c.next.Season(ctx, language, seriesID, number)
	})
}

// Collection answers from the cache while fresh (30 days).
func (c *CachedProvider) Collection(ctx context.Context, language string, id int64) (domain.Collection, error) {
	return cached(ctx, c, key("collection/"+strconv.FormatInt(id, 10), language, nil), stableTTL, func(ctx context.Context) (domain.Collection, error) {
		return c.next.Collection(ctx, language, id)
	})
}

// Genres answers from the cache while fresh (30 days).
func (c *CachedProvider) Genres(ctx context.Context, language string, kind domain.MediaKind) ([]domain.Genre, error) {
	return cached(ctx, c, key("genres/"+string(kind), language, nil), stableTTL, func(ctx context.Context) ([]domain.Genre, error) {
		return c.next.Genres(ctx, language, kind)
	})
}

// cached is the one place where the lifetimes and the fallback rules apply.
// Problems with the store are logged and treated as a miss, so a damaged
// cache never blocks TMDB.
func cached[T any](ctx context.Context, c *CachedProvider, cacheKey string, ttl time.Duration, fetch func(context.Context) (T, error)) (T, error) {
	var zero T
	forced := refreshRequested(ctx)

	old, expiresAt, hasOld := lookup[T](ctx, c, cacheKey)
	if hasOld && !forced && c.now().Before(expiresAt) {
		return old, nil
	}
	// The stale answer is only a fallback for an ordinary read: a forced
	// refresh must not hide a TMDB failure behind old data.
	fallback := hasOld && !forced

	v, err := fetch(ctx)
	if err != nil {
		if fallback && errors.Is(err, domain.ErrUpstream) {
			c.log.WarnContext(ctx, "metadata: TMDB failed, serving the stale answer", "key", cacheKey, "error", err)
			return old, nil
		}
		return zero, err
	}

	c.save(ctx, cacheKey, v, c.now().Add(ttl))
	return v, nil
}

// lookup reads and decodes the entry under key. A missing, unreadable or
// undecodable entry reports ok false, so the caller asks TMDB.
func lookup[T any](ctx context.Context, c *CachedProvider, key string) (value T, expiresAt time.Time, ok bool) {
	var zero T
	entry, found, err := c.store.Get(ctx, key)
	if err != nil {
		c.log.WarnContext(ctx, "metadata cache: read failed, asking TMDB", "key", key, "error", err)
		return zero, time.Time{}, false
	}
	if !found {
		return zero, time.Time{}, false
	}
	if err := json.Unmarshal(entry.Payload, &value); err != nil {
		c.log.WarnContext(ctx, "metadata cache: entry unreadable, asking TMDB", "key", key, "error", err)
		return zero, time.Time{}, false
	}
	return value, entry.ExpiresAt, true
}

// save stores an answer. A failure is logged: the answer is still valid.
func (c *CachedProvider) save(ctx context.Context, key string, v any, expiresAt time.Time) {
	payload, err := json.Marshal(v)
	if err != nil {
		c.log.WarnContext(ctx, "metadata cache: encode failed", "key", key, "error", err)
		return
	}
	if err := c.store.Put(ctx, key, CachedResponse{Payload: payload, ExpiresAt: expiresAt}); err != nil {
		c.log.WarnContext(ctx, "metadata cache: write failed", "key", key, "error", err)
	}
}

// key builds the cache key of one call: schema, operation path, language and
// parameters. url.Values encodes its parameters in sorted order, so equal calls
// always get the same key and different ones never collide.
func key(path, language string, params url.Values) string {
	q := url.Values{"lang": {language}}
	for k, v := range params {
		q[k] = v
	}
	return cacheSchema + " " + path + "?" + q.Encode()
}

func setYear(params url.Values, year *int) {
	if year != nil {
		params.Set("year", fmt.Sprint(*year))
	}
}
