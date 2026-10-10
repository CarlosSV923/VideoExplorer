// Package httpx contains the HTTP plumbing shared by every bounded context:
// RFC 9457 problem responses, client identification and cross-cutting
// middleware (security headers, request logging, panic recovery).
package httpx

import (
	"context"
	"encoding/json"
	"log/slog"
	"net"
	"net/http"
	"runtime/debug"
	"strings"
	"time"
)

// Problem is an RFC 9457 problem details body.
type Problem struct {
	Type   string `json:"type,omitempty"`
	Title  string `json:"title"`
	Status int    `json:"status"`
	Detail string `json:"detail,omitempty"`
}

// WriteProblem writes an application/problem+json response.
func WriteProblem(w http.ResponseWriter, status int, title, detail string) {
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(Problem{Title: title, Status: status, Detail: detail})
}

// contentSecurityPolicy is RNF-02: no external origins at all. TMDB images
// reach the browser through the local cache (RNF-10), never directly.
const contentSecurityPolicy = "default-src 'self'; img-src 'self' data:; " +
	"style-src 'self' 'unsafe-inline'; connect-src 'self'; frame-ancestors 'none'; base-uri 'self'"

// SecurityHeaders sets the browser security headers of RNF-02 on every
// response, and Cache-Control: no-store on the API.
func SecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Referrer-Policy", "no-referrer")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Content-Security-Policy", contentSecurityPolicy)
		if strings.HasPrefix(r.URL.Path, "/api/") {
			h.Set("Cache-Control", "no-store")
		}
		next.ServeHTTP(w, r)
	})
}

type clientIPKey struct{}

// ClientIP returns the client address stored by WithClientIP.
func ClientIP(ctx context.Context) string {
	ip, _ := ctx.Value(clientIPKey{}).(string)
	return ip
}

// WithClientIP stores the peer IP in the request context. The app is reached
// directly on the LAN, so forwarding headers are deliberately not trusted.
func WithClientIP(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host, _, err := net.SplitHostPort(r.RemoteAddr)
		if err != nil {
			host = r.RemoteAddr
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), clientIPKey{}, host)))
	})
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (s *statusRecorder) WriteHeader(code int) {
	s.status = code
	s.ResponseWriter.WriteHeader(code)
}

// Unwrap lets http.ResponseController reach Flush and deadlines (needed by SSE and tus).
func (s *statusRecorder) Unwrap() http.ResponseWriter { return s.ResponseWriter }

// Logging logs one line per request (health probes excluded). Only the path is
// logged, never the query string.
func Logging(log *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
			next.ServeHTTP(rec, r)
			if r.URL.Path == "/api/health" {
				return
			}
			log.LogAttrs(r.Context(), slog.LevelInfo, "http",
				slog.String("method", r.Method),
				slog.String("path", r.URL.Path),
				slog.Int("status", rec.status),
				slog.Duration("duration", time.Since(start)),
			)
		})
	}
}

// Recover turns panics into 500 problems instead of dropping the connection.
func Recover(log *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if v := recover(); v != nil {
					if v == http.ErrAbortHandler { //nolint:errorlint // sentinel value, re-panic as net/http expects
						panic(v)
					}
					log.Error("panic", "value", v, "path", r.URL.Path, "stack", string(debug.Stack()))
					WriteProblem(w, http.StatusInternalServerError, "Internal Server Error", "")
				}
			}()
			next.ServeHTTP(w, r)
		})
	}
}

// Chain applies middleware so that the first one is the outermost.
func Chain(h http.Handler, mws ...func(http.Handler) http.Handler) http.Handler {
	for i := len(mws) - 1; i >= 0; i-- {
		h = mws[i](h)
	}
	return h
}
