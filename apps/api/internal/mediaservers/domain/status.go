// Package domain contains the media-server concepts shared by the application
// service and its adapters. It has no knowledge of HTTP or Jellyfin.
package domain

import "time"

// State is the connection state exposed to the settings and health layers.
type State string

const (
	// StateUnconfigured means no media-server credentials were configured.
	StateUnconfigured State = "unconfigured"
	// StateConnected means the adapter can notify the media server.
	StateConnected State = "connected"
	// StateError means the latest discovery or notification failed.
	StateError State = "error"
)

// TranslationSource says where the route mapping came from. None means that
// Jellyfin's roots were not found and a full-library refresh is used.
type TranslationSource string

const (
	// TranslationDetected means Jellyfin's virtual-folder locations supplied the mapping.
	TranslationDetected TranslationSource = "detected"
	// TranslationConfigured means JELLYFIN_LIBRARY_PATH supplied the mapping.
	TranslationConfigured TranslationSource = "configured"
	// TranslationNone means the adapter falls back to a full-library refresh.
	TranslationNone TranslationSource = "none"
)

// Translation describes how the media server sees the two library roots.
// Empty roots mean that the adapter could not map that root and must use a
// full-library refresh for changes below it.
type Translation struct {
	Forced      bool
	Source      TranslationSource
	LibraryRoot string
	MoviesRoot  string
	SeriesRoot  string
}

// Status is an in-memory snapshot of the media-server integration. It contains
// no credentials and is safe to pass to a future HTTP status handler.
type Status struct {
	State              State
	Translation        Translation
	FullRefresh        bool
	LastErrorCode      string
	LastError          string
	LastErrorAt        time.Time
	LastNotificationAt time.Time
}

// Connection is the result of a successful adapter path discovery.
type Connection struct {
	Translation Translation
}
