package application_test

import (
	"context"
	"errors"
	"log/slog"
	"reflect"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/mediaservers/application"
	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/mediaservers/domain"
)

type fakeBackend struct {
	configured bool
	err        error
	mu         sync.Mutex
	calls      [][]string
}

func (f *fakeBackend) Configured() bool { return f.configured }

func (f *fakeBackend) Discover(context.Context) (domain.Connection, error) {
	return domain.Connection{Translation: domain.Translation{MoviesRoot: "/jelly/Movies", SeriesRoot: "/jelly/TV Shows"}}, nil
}

func (f *fakeBackend) Refresh(_ context.Context, folders []string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, slices.Clone(folders))
	return f.err
}

func (f *fakeBackend) refreshed() [][]string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.calls)
}

type fakeTimer struct {
	clock *fakeClock
	fire  func()
	at    time.Time
	stop  bool
}

func (t *fakeTimer) Stop() bool {
	t.clock.mu.Lock()
	defer t.clock.mu.Unlock()
	if t.stop {
		return false
	}
	t.stop = true
	return true
}

type fakeClock struct {
	mu     sync.Mutex
	now    time.Time
	timers []*fakeTimer
}

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *fakeClock) AfterFunc(d time.Duration, fn func()) application.Timer {
	c.mu.Lock()
	defer c.mu.Unlock()
	t := &fakeTimer{clock: c, fire: fn, at: c.now.Add(d)}
	c.timers = append(c.timers, t)
	return t
}

func (c *fakeClock) advance(d time.Duration) {
	c.mu.Lock()
	c.now = c.now.Add(d)
	var ready []*fakeTimer
	remaining := c.timers[:0]
	for _, timer := range c.timers {
		if !timer.stop && !timer.at.After(c.now) {
			ready = append(ready, timer)
		} else if !timer.stop {
			remaining = append(remaining, timer)
		}
	}
	c.timers = remaining
	c.mu.Unlock()
	for _, timer := range ready {
		timer.fire()
	}
}

func newTestService(t *testing.T, backend *fakeBackend, clock *fakeClock) *application.Service {
	t.Helper()
	return application.NewService(backend, clock, slog.New(slog.DiscardHandler))
}

func TestNotifyFoldersGroupsChangesInThirtySecondWindows(t *testing.T) {
	t.Parallel()

	clock := &fakeClock{now: time.Date(2026, time.October, 15, 12, 0, 0, 0, time.UTC)}
	backend := &fakeBackend{configured: true}
	service := newTestService(t, backend, clock)

	service.NotifyFolders(t.Context(), []string{"TV Shows/Show/Season 02", "Movies/Movie", "Movies/Movie"})
	service.NotifyFolders(t.Context(), []string{"TV Shows/Show/Season 02", "_unassigned/incoming"})
	if got := backend.refreshed(); len(got) != 0 {
		t.Fatalf("refreshes before the window = %v, want none", got)
	}
	clock.advance(29 * time.Second)
	if got := backend.refreshed(); len(got) != 0 {
		t.Fatalf("refreshes before 30 seconds = %v, want none", got)
	}
	clock.advance(time.Second)
	if got, want := backend.refreshed(), [][]string{{"Movies/Movie", "TV Shows/Show/Season 02"}}; !reflect.DeepEqual(got, want) {
		t.Fatalf("refreshes = %v, want %v", got, want)
	}
}

func TestNotifyFoldersUsesSeparateRequestsForSeparateWindows(t *testing.T) {
	t.Parallel()

	clock := &fakeClock{now: time.Date(2026, time.October, 15, 12, 0, 0, 0, time.UTC)}
	backend := &fakeBackend{configured: true}
	service := newTestService(t, backend, clock)

	service.NotifyFolders(t.Context(), []string{"Movies/First"})
	clock.advance(30 * time.Second)
	service.NotifyFolders(t.Context(), []string{"Movies/Second"})
	clock.advance(30 * time.Second)

	if got, want := backend.refreshed(), [][]string{{"Movies/First"}, {"Movies/Second"}}; !reflect.DeepEqual(got, want) {
		t.Fatalf("refreshes = %v, want %v", got, want)
	}
}

func TestNotificationFailureDoesNotEscapeAndIsKeptInStatus(t *testing.T) {
	t.Parallel()

	clock := &fakeClock{now: time.Date(2026, time.October, 15, 12, 0, 0, 0, time.UTC)}
	backend := &fakeBackend{configured: true, err: errors.New("upstream unavailable")}
	service := newTestService(t, backend, clock)

	service.NotifyFolders(t.Context(), []string{"Movies/Movie"})
	clock.advance(30 * time.Second)

	status := service.Status()
	if status.State != domain.StateError || status.LastErrorCode != application.ErrorCodeNotification || status.LastError != "upstream unavailable" || !status.LastErrorAt.Equal(clock.Now()) {
		t.Fatalf("status = %+v, want the failed notification and its time", status)
	}
}

func TestSuccessfulNotificationKeepsItsTimestamp(t *testing.T) {
	t.Parallel()

	clock := &fakeClock{now: time.Date(2026, time.October, 15, 12, 0, 0, 0, time.UTC)}
	backend := &fakeBackend{configured: true}
	service := newTestService(t, backend, clock)

	service.NotifyFolders(t.Context(), []string{"Movies/Movie"})
	clock.advance(30 * time.Second)

	status := service.Status()
	if status.State != domain.StateConnected || !status.LastNotificationAt.Equal(clock.Now()) {
		t.Fatalf("status = %+v, want a successful notification timestamp", status)
	}
}

func TestUnconfiguredServiceDoesNotCallBackend(t *testing.T) {
	t.Parallel()

	clock := &fakeClock{now: time.Date(2026, time.October, 15, 12, 0, 0, 0, time.UTC)}
	backend := &fakeBackend{}
	service := newTestService(t, backend, clock)
	service.Start(t.Context())
	service.NotifyFolders(t.Context(), []string{"Movies/Movie"})
	clock.advance(30 * time.Second)

	if got := backend.refreshed(); len(got) != 0 {
		t.Fatalf("refreshes = %v, want none", got)
	}
	if got := service.Status().State; got != domain.StateUnconfigured {
		t.Fatalf("state = %q, want %q", got, domain.StateUnconfigured)
	}
}
