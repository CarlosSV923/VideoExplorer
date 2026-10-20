// Package tmdb implements the metadata Provider on TMDB's v3 API
// (https://developer.themoviedb.org/reference). It owns everything TMDB
// specific: the two kinds of credential, the query parameters, rate limits,
// retries and the JSON schema. Nothing outside this package sees TMDB types.
package tmdb

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"golang.org/x/time/rate"

	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/metadata/application"
	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/metadata/domain"
)

// Defaults of Config. TMDB allows roughly 40 requests per second; the client
// stays well below that and keeps few requests in flight.
const (
	DefaultBaseURL           = "https://api.themoviedb.org/3"
	DefaultRequestTimeout    = 10 * time.Second
	DefaultRequestsPerSecond = 10
	DefaultMaxConcurrent     = 4
	DefaultMaxAttempts       = 3
	DefaultRetryDelay        = 500 * time.Millisecond
	DefaultMaxRetryAfter     = 10 * time.Second
)

const (
	maxBodyBytes = 4 << 20
	redacted     = "[redacted]"
)

// errCredentials is returned when neither or both credentials are set.
var errCredentials = errors.New("tmdb: set exactly one of the v3 API key and the v4 read token")

// Config configures the client. Exactly one of APIKey (v3, sent as the api_key
// query parameter) and ReadToken (v4, sent as a Bearer header) is required.
// The zero value of every other field selects the default.
type Config struct {
	APIKey    string
	ReadToken string

	BaseURL    string
	HTTPClient *http.Client

	// RequestTimeout bounds each HTTP attempt.
	RequestTimeout time.Duration
	// RequestsPerSecond and MaxConcurrent keep the client polite with TMDB.
	RequestsPerSecond float64
	MaxConcurrent     int
	// MaxAttempts is the number of tries per request, including the first one.
	MaxAttempts int
	// RetryDelay is the first backoff for 5xx and network errors; it doubles each time.
	RetryDelay time.Duration
	// MaxRetryAfter caps the wait for a 429. A longer Retry-After fails at once
	// instead of holding the request open.
	MaxRetryAfter time.Duration
}

// Client is a concurrency-safe TMDB client. It implements application.Provider.
type Client struct {
	cfg     Config
	http    *http.Client
	limiter *rate.Limiter
	slots   chan struct{}
}

var _ application.Provider = (*Client)(nil)

// New builds a client. It fails when the credentials are not exactly one of
// the two kinds: without any, the provider does not exist (spec §6).
func New(cfg Config) (*Client, error) {
	if (cfg.APIKey == "") == (cfg.ReadToken == "") {
		return nil, errCredentials
	}
	if cfg.BaseURL == "" {
		cfg.BaseURL = DefaultBaseURL
	}
	cfg.BaseURL = strings.TrimSuffix(cfg.BaseURL, "/")
	if cfg.RequestTimeout <= 0 {
		cfg.RequestTimeout = DefaultRequestTimeout
	}
	if cfg.RequestsPerSecond <= 0 {
		cfg.RequestsPerSecond = DefaultRequestsPerSecond
	}
	if cfg.MaxConcurrent <= 0 {
		cfg.MaxConcurrent = DefaultMaxConcurrent
	}
	if cfg.MaxAttempts <= 0 {
		cfg.MaxAttempts = DefaultMaxAttempts
	}
	if cfg.RetryDelay <= 0 {
		cfg.RetryDelay = DefaultRetryDelay
	}
	if cfg.MaxRetryAfter <= 0 {
		cfg.MaxRetryAfter = DefaultMaxRetryAfter
	}
	hc := cfg.HTTPClient
	if hc == nil {
		hc = &http.Client{Transport: http.DefaultTransport.(*http.Transport).Clone()}
	}
	return &Client{
		cfg:     cfg,
		http:    hc,
		limiter: rate.NewLimiter(rate.Limit(cfg.RequestsPerSecond), max(1, int(cfg.RequestsPerSecond))),
		slots:   make(chan struct{}, cfg.MaxConcurrent),
	}, nil
}

// get performs a GET with retries. A 429 waits for Retry-After; 5xx and
// network errors back off exponentially. Other statuses fail at once.
func (c *Client) get(ctx context.Context, endpoint string, query url.Values, out any) error {
	select {
	case c.slots <- struct{}{}:
	case <-ctx.Done():
		return ctx.Err()
	}
	defer func() { <-c.slots }()

	for attempt := 1; ; attempt++ {
		if err := c.limiter.Wait(ctx); err != nil {
			return err
		}
		wait, retry, err := c.try(ctx, endpoint, query, out)
		if err == nil {
			return nil
		}
		if !retry || attempt >= c.cfg.MaxAttempts || ctx.Err() != nil {
			return err
		}
		if wait < 0 {
			wait = c.backoff(attempt)
		}
		if err := sleep(ctx, wait); err != nil {
			return err
		}
	}
}

// try makes one attempt. wait is the delay before a retry: negative when the
// response gives none, so the caller backs off.
func (c *Client) try(ctx context.Context, endpoint string, query url.Values, out any) (wait time.Duration, retry bool, err error) {
	reqCtx, cancel := context.WithTimeout(ctx, c.cfg.RequestTimeout)
	defer cancel()

	q := url.Values{}
	for k, v := range query {
		q[k] = v
	}
	if c.cfg.APIKey != "" {
		q.Set("api_key", c.cfg.APIKey)
	}
	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, c.cfg.BaseURL+endpoint+"?"+q.Encode(), nil)
	if err != nil {
		return -1, false, c.upstream(endpoint, "invalid request: "+err.Error())
	}
	req.Header.Set("Accept", "application/json")
	if c.cfg.ReadToken != "" {
		req.Header.Set("Authorization", "Bearer "+c.cfg.ReadToken)
	}

	res, err := c.http.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return -1, false, ctx.Err()
		}
		return -1, true, c.upstream(endpoint, "request failed: "+unwrapURLError(err).Error())
	}
	defer res.Body.Close()

	body := io.LimitReader(res.Body, maxBodyBytes)
	switch {
	case res.StatusCode == http.StatusOK:
		if err := json.NewDecoder(body).Decode(out); err != nil {
			return -1, false, c.upstream(endpoint, "malformed response: "+err.Error())
		}
		return -1, false, nil
	case res.StatusCode == http.StatusNotFound:
		return -1, false, domain.ErrNotFound
	case res.StatusCode == http.StatusUnauthorized || res.StatusCode == http.StatusForbidden:
		return -1, false, c.upstream(endpoint, "credentials rejected (check TMDB_API_KEY or TMDB_READ_TOKEN)")
	case res.StatusCode == http.StatusTooManyRequests:
		after, ok := retryAfter(res.Header.Get("Retry-After"), time.Now())
		if ok && after > c.cfg.MaxRetryAfter {
			return -1, false, c.upstream(endpoint, "rate limited, Retry-After "+after.String()+" is over the limit")
		}
		if !ok {
			after = -1
		}
		return after, true, c.upstream(endpoint, "rate limited (429)")
	case res.StatusCode >= 500:
		return -1, true, c.upstream(endpoint, "returned "+strconv.Itoa(res.StatusCode))
	default:
		return -1, false, c.upstream(endpoint, "returned "+strconv.Itoa(res.StatusCode)+": "+statusMessage(body))
	}
}

// backoff returns the delay after the given failed attempt, capped at MaxRetryAfter.
func (c *Client) backoff(attempt int) time.Duration {
	d := c.cfg.RetryDelay << (attempt - 1)
	return min(d, c.cfg.MaxRetryAfter)
}

// upstream builds a domain.ErrUpstream error that names the endpoint and never
// carries a credential.
func (c *Client) upstream(endpoint, msg string) error {
	return fmt.Errorf("%w: %s: %s", domain.ErrUpstream, endpoint, c.redact(msg))
}

// redact removes the credentials from any text that may end up in an error.
func (c *Client) redact(s string) string {
	for _, secret := range []string{c.cfg.APIKey, c.cfg.ReadToken} {
		if secret != "" {
			s = strings.ReplaceAll(s, secret, redacted)
		}
	}
	return s
}

// unwrapURLError drops the request URL from a transport error, because the
// URL of a v3 request carries the API key.
func unwrapURLError(err error) error {
	var ue *url.Error
	if errors.As(err, &ue) {
		return ue.Err
	}
	return err
}

// statusMessage reads TMDB's status_message from an error body, if it has one.
func statusMessage(body io.Reader) string {
	var e struct {
		Message string `json:"status_message"`
	}
	if err := json.NewDecoder(body).Decode(&e); err != nil {
		return "no message"
	}
	return e.Message
}

// retryAfter parses a Retry-After header, either seconds or an HTTP date. It
// reports false when the header is absent or unreadable.
func retryAfter(value string, now time.Time) (time.Duration, bool) {
	if value == "" {
		return 0, false
	}
	if secs, err := strconv.Atoi(value); err == nil && secs >= 0 {
		return time.Duration(secs) * time.Second, true
	}
	if t, err := http.ParseTime(value); err == nil {
		return max(0, t.Sub(now)), true
	}
	return 0, false
}

func sleep(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return nil
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-t.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
