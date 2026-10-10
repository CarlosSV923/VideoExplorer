// Package http adapts the identity use cases to the generated HTTP contract
// and provides the authentication middleware for every other operation.
package http

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/identity/application"
	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/identity/domain"
	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/platform/httpapi"
	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/platform/httpx"
)

// CookieName is the session cookie (declared in api/openapi.yaml).
const CookieName = "ve_session"

// publicOperations are reachable without a session (RF-60). Everything else is
// protected by default, so a new operation can never be exposed by mistake.
var publicOperations = map[string]bool{
	"GetHealth": true,
	"Login":     true,
}

// Handler implements the auth operations of httpapi.StrictServerInterface.
type Handler struct {
	svc          *application.Service
	cookieSecure bool
}

// NewHandler builds the handler. cookieSecure is COOKIE_SECURE: true only when
// the app is served over HTTPS.
func NewHandler(svc *application.Service, cookieSecure bool) *Handler {
	return &Handler{svc: svc, cookieSecure: cookieSecure}
}

type sessionKey struct{}

// SessionFrom returns the session attached by the middleware.
func SessionFrom(ctx context.Context) (domain.Session, bool) {
	s, ok := ctx.Value(sessionKey{}).(domain.Session)
	return s, ok
}

// Middleware rejects requests without a valid session cookie, except for
// public operations. The rejection is an RFC 9457 401.
func (h *Handler) Middleware() httpapi.StrictMiddlewareFunc {
	return func(next httpapi.StrictHandlerFunc, operationID string) httpapi.StrictHandlerFunc {
		return func(ctx context.Context, w http.ResponseWriter, r *http.Request, req any) (any, error) {
			if publicOperations[operationID] {
				return next(ctx, w, r, req)
			}
			session, ok := h.authenticate(r)
			if !ok {
				writeUnauthorized(w)
				return nil, nil
			}
			return next(context.WithValue(ctx, sessionKey{}, session), w, r, req)
		}
	}
}

func (h *Handler) authenticate(r *http.Request) (domain.Session, bool) {
	var token string
	if c, err := r.Cookie(CookieName); err == nil {
		token = c.Value
	}
	session, err := h.svc.Authenticate(r.Context(), token)
	return session, err == nil
}

func writeUnauthorized(w http.ResponseWriter) {
	httpx.WriteProblem(w, http.StatusUnauthorized, "Unauthorized", "Sign in to continue.")
}

// Login implements httpapi.StrictServerInterface.
func (h *Handler) Login(ctx context.Context, req httpapi.LoginRequestObject) (httpapi.LoginResponseObject, error) {
	token, session, err := h.svc.Login(ctx, httpx.ClientIP(ctx), req.Body.Password)
	switch {
	case errors.Is(err, domain.ErrTooManyAttempts):
		return httpapi.Login429ApplicationProblemPlusJSONResponse{
			TooManyRequestsApplicationProblemPlusJSONResponse: httpapi.TooManyRequestsApplicationProblemPlusJSONResponse(problem(http.StatusTooManyRequests,
				"Too Many Requests", "Too many attempts. Wait a moment and try again.")),
		}, nil
	case errors.Is(err, domain.ErrInvalidCredentials):
		return httpapi.Login401ApplicationProblemPlusJSONResponse{
			UnauthorizedApplicationProblemPlusJSONResponse: httpapi.UnauthorizedApplicationProblemPlusJSONResponse(problem(http.StatusUnauthorized,
				"Unauthorized", "The password is incorrect.")),
		}, nil
	case err != nil:
		return nil, err
	}

	cookie := h.cookie(token, session.ExpiresAt, int(h.svc.TTL().Seconds()))
	return httpapi.Login204Response{Headers: httpapi.Login204ResponseHeaders{SetCookie: &cookie}}, nil
}

// Logout implements httpapi.StrictServerInterface. Sessions are stateless, so
// ending one means clearing the cookie on this device.
func (h *Handler) Logout(context.Context, httpapi.LogoutRequestObject) (httpapi.LogoutResponseObject, error) {
	cookie := h.cookie("", time.Unix(0, 0), -1)
	return httpapi.Logout204Response{Headers: httpapi.Logout204ResponseHeaders{SetCookie: &cookie}}, nil
}

// GetSession implements httpapi.StrictServerInterface.
func (h *Handler) GetSession(ctx context.Context, _ httpapi.GetSessionRequestObject) (httpapi.GetSessionResponseObject, error) {
	s, ok := SessionFrom(ctx)
	if !ok {
		return httpapi.GetSession401ApplicationProblemPlusJSONResponse{
			UnauthorizedApplicationProblemPlusJSONResponse: httpapi.UnauthorizedApplicationProblemPlusJSONResponse(problem(http.StatusUnauthorized, "Unauthorized", "")),
		}, nil
	}
	return httpapi.GetSession200JSONResponse{ExpiresAt: s.ExpiresAt}, nil
}

func (h *Handler) cookie(value string, expires time.Time, maxAge int) string {
	// Secure is configurable on purpose: COOKIE_SECURE=true when the app is
	// served over HTTPS, false for plain HTTP on the LAN.
	c := http.Cookie{ //nolint:gosec // see comment above
		Name:     CookieName,
		Value:    value,
		Path:     "/",
		Expires:  expires,
		MaxAge:   maxAge,
		HttpOnly: true,
		Secure:   h.cookieSecure,
		SameSite: http.SameSiteStrictMode,
	}
	return c.String()
}

func problem(status int, title, detail string) httpapi.Problem {
	p := httpapi.Problem{Status: status, Title: title}
	if detail != "" {
		p.Detail = &detail
	}
	return p
}
