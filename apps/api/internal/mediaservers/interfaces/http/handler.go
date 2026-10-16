// Package http adapts media-server status to the generated settings contract.
package http

import (
	"context"
	"time"

	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/mediaservers/domain"
	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/platform/httpapi"
)

// StatusProvider is the small application port needed by the settings
// endpoint. It deliberately exposes a snapshot, never credentials.
type StatusProvider interface {
	Status() domain.Status
}

// Handler implements the settings operation of the generated contract.
type Handler struct {
	provider StatusProvider
}

// NewHandler builds a settings handler. A nil provider is the valid
// unconfigured installation used by tests and by a future optional adapter.
func NewHandler(provider StatusProvider) *Handler {
	return &Handler{provider: provider}
}

// GetSettings returns the status needed by the Settings screen. The response
// contains no upstream URL, credentials, or adapter configuration.
func (h *Handler) GetSettings(context.Context, httpapi.GetSettingsRequestObject) (httpapi.GetSettingsResponseObject, error) {
	status := domain.Status{
		State:       domain.StateUnconfigured,
		Translation: domain.Translation{Source: domain.TranslationNone},
	}
	if h.provider != nil {
		status = h.provider.Status()
	}

	state := httpapi.JellyfinSettingsState(status.State)
	source := httpapi.JellyfinSettingsTranslationSource(status.Translation.Source)
	if source == "" {
		source = httpapi.JellyfinSettingsTranslationSourceNone
	}
	settings := httpapi.JellyfinSettings{
		State:             state,
		TranslationSource: source,
		Translation: httpapi.JellyfinTranslation{
			LibraryPath: status.Translation.LibraryRoot,
			Movies:      status.Translation.MoviesRoot,
			Series:      status.Translation.SeriesRoot,
		},
		LastErrorAt:        timePtr(status.LastErrorAt),
		LastNotificationAt: timePtr(status.LastNotificationAt),
	}
	if status.LastErrorCode != "" || status.LastError != "" {
		settings.LastError = &httpapi.JellyfinError{
			Code:    httpapi.JellyfinErrorCode(status.LastErrorCode),
			Message: status.LastError,
		}
	}

	return httpapi.GetSettings200JSONResponse{Jellyfin: settings}, nil
}

func timePtr(value time.Time) *time.Time {
	if value.IsZero() {
		return nil
	}
	copy := value
	return &copy
}
