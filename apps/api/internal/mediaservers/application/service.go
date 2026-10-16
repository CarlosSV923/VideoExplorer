// Package application coordinates media-server notifications without exposing
// a concrete server such as Jellyfin to the catalog context.
package application

import (
	"context"
	"log/slog"
	"path"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/mediaservers/domain"
)

const (
	// RefreshWindow is the debounce window required by RF-50.
	RefreshWindow = 30 * time.Second
	refreshPeriod = time.Hour
	// ErrorCodeDiscovery is the stable settings code for a failed route lookup.
	ErrorCodeDiscovery = "discovery_failed"
	// ErrorCodeNotification is the stable settings code for a failed refresh.
	ErrorCodeNotification = "notification_failed"
)

// Timer is the part of a timer needed by the injected clock.
type Timer interface {
	Stop() bool
}

// Clock makes notification grouping deterministic in tests.
type Clock interface {
	Now() time.Time
	AfterFunc(time.Duration, func()) Timer
}

// Backend is the port implemented by a media-server adapter. Discover updates
// the adapter's route cache; Refresh sends either folder updates or a full
// refresh when a folder cannot be translated.
type Backend interface {
	Configured() bool
	Discover(context.Context) (domain.Connection, error)
	Refresh(context.Context, []string) error
}

// MediaServerNotifier is the port used by the catalog commit hook. Its method
// only queues work: it never waits for or returns an upstream media-server
// request error.
type MediaServerNotifier interface {
	NotifyFolders(context.Context, []string)
}

// Service batches changed library folders and reports adapter failures without
// making them part of the library operation's result.
type Service struct {
	backend Backend
	clock   Clock
	log     *slog.Logger

	mu      sync.Mutex
	pending map[string]struct{}
	timer   Timer
	status  domain.Status
}

var _ MediaServerNotifier = (*Service)(nil)

// NewService builds the notifier. A nil clock or logger selects production
// defaults.
func NewService(backend Backend, clock Clock, log *slog.Logger) *Service {
	if clock == nil {
		clock = realClock{}
	}
	if log == nil {
		log = slog.New(slog.DiscardHandler)
	}
	state := domain.StateConnected
	if backend == nil || !backend.Configured() {
		state = domain.StateUnconfigured
	}
	return &Service{
		backend: backend,
		clock:   clock,
		log:     log,
		pending: map[string]struct{}{},
		status:  domain.Status{State: state, Translation: domain.Translation{Source: domain.TranslationNone}, FullRefresh: state == domain.StateConnected},
	}
}

// Start discovers the adapter's roots once and refreshes them every hour. It
// returns immediately so a slow Jellyfin does not delay serving the API.
func (s *Service) Start(ctx context.Context) {
	if s.backend == nil || !s.backend.Configured() {
		return
	}
	go func() {
		s.RefreshConfiguration(ctx)
		ticker := time.NewTicker(refreshPeriod)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				s.RefreshConfiguration(ctx)
			case <-ctx.Done():
				return
			}
		}
	}()
}

// RefreshConfiguration performs one path discovery. It is exported so the
// composition root and tests can request the same startup/hourly action.
func (s *Service) RefreshConfiguration(ctx context.Context) {
	if s.backend == nil || !s.backend.Configured() {
		s.setStatus(domain.Status{State: domain.StateUnconfigured})
		return
	}
	connection, err := s.backend.Discover(ctx)
	if err != nil {
		s.failed(err, ErrorCodeDiscovery)
		return
	}
	status := s.Status()
	status.State = domain.StateConnected
	status.Translation = connection.Translation
	status.FullRefresh = !connection.Translation.Forced &&
		(connection.Translation.MoviesRoot == "" || connection.Translation.SeriesRoot == "")
	status.LastErrorCode = ""
	status.LastError = ""
	status.LastErrorAt = time.Time{}
	s.setStatus(status)
}

// Status returns the latest in-memory state for the future settings/health
// handlers.
func (s *Service) Status() domain.Status {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.status
}

// HealthState returns only the state required by the public health endpoint.
func (s *Service) HealthState() string { return string(s.Status().State) }

// NotifyFolders queues title or season folders. No-assigned and metadata paths
// are deliberately ignored even if a caller passes them by mistake.
func (s *Service) NotifyFolders(_ context.Context, folders []string) {
	if s.backend == nil || !s.backend.Configured() {
		return
	}
	clean := changedFolders(folders)
	if len(clean) == 0 {
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	for _, folder := range clean {
		s.pending[folder] = struct{}{}
	}
	if s.timer == nil {
		s.timer = s.clock.AfterFunc(RefreshWindow, s.flush)
	}
}

func (s *Service) flush() {
	s.mu.Lock()
	folders := make([]string, 0, len(s.pending))
	for folder := range s.pending {
		folders = append(folders, folder)
	}
	s.pending = map[string]struct{}{}
	s.timer = nil
	s.mu.Unlock()
	if len(folders) == 0 {
		return
	}
	sort.Strings(folders)

	if err := s.backend.Refresh(context.Background(), folders); err != nil {
		s.failed(err, ErrorCodeNotification)
		return
	}
	status := s.Status()
	status.State = domain.StateConnected
	status.LastErrorCode = ""
	status.LastError = ""
	status.LastErrorAt = time.Time{}
	status.LastNotificationAt = s.clock.Now()
	s.setStatus(status)
}

func (s *Service) failed(err error, code string) {
	now := s.clock.Now()
	status := s.Status()
	status.State = domain.StateError
	status.LastErrorCode = code
	status.LastError = err.Error()
	status.LastErrorAt = now
	s.setStatus(status)
	s.log.Error("media server notification failed", "error", err)
}

func (s *Service) setStatus(status domain.Status) {
	s.mu.Lock()
	s.status = status
	s.mu.Unlock()
}

func changedFolders(folders []string) []string {
	seen := make(map[string]struct{}, len(folders))
	for _, raw := range folders {
		folder := strings.Trim(strings.ReplaceAll(raw, "\\", "/"), "/")
		if folder == "" || strings.HasPrefix(folder, "_unassigned/") || folder == "_unassigned" || strings.HasPrefix(folder, ".videoexplorer/") {
			continue
		}
		folder = path.Clean(folder)
		if folder == "." || strings.HasPrefix(folder, "../") || folder == ".." {
			continue
		}
		seen[folder] = struct{}{}
	}
	out := make([]string, 0, len(seen))
	for folder := range seen {
		out = append(out, folder)
	}
	sort.Strings(out)
	return out
}

type realClock struct{}

func (realClock) Now() time.Time { return time.Now() }

func (realClock) AfterFunc(d time.Duration, fn func()) Timer {
	return time.AfterFunc(d, fn)
}
