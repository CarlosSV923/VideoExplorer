package application_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/catalog/application"
	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/catalog/domain"
)

// guard bounds every wait of the scan tests: a scan that never happens fails the test instead of hanging it.
const guard = 5 * time.Second

// await returns the next value of ch, or fails the test when none comes in time.
func await[T any](t *testing.T, ch chan T, what string) T {
	t.Helper()
	select {
	case v := <-ch:
		return v
	case <-time.After(guard):
		t.Fatalf("timed out waiting for %s", what)
		var zero T
		return zero
	}
}

// timerRequest is one wait that a scan asked the clock for; the test fires it when it wants the next run.
type timerRequest struct {
	every time.Duration
	fire  chan time.Time
}

// fakeTimers stands for the clock of the scans: every wait is handed to the test.
type fakeTimers struct {
	requests chan timerRequest
}

func (f *fakeTimers) After(every time.Duration) <-chan time.Time {
	fire := make(chan time.Time, 1)
	f.requests <- timerRequest{every: every, fire: fire}
	return fire
}

// fakeSection is the reading of No asignados. A held section keeps each run until its gate closes, or
// until the context ends.
type fakeSection struct {
	mu          sync.Mutex
	calls       int
	gotArrivals application.Arrivals
	err         error
	gate        chan struct{}
	started     chan struct{}
}

func (f *fakeSection) Sync(ctx context.Context, arrivals application.Arrivals) ([]application.SectionEntry, error) {
	f.mu.Lock()
	f.calls++
	f.gotArrivals = arrivals
	gate, err := f.gate, f.err
	f.mu.Unlock()
	f.started <- struct{}{}
	if gate != nil {
		select {
		case <-gate:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	return nil, err
}

func (f *fakeSection) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

// fakeLibrary is the library scan of a later phase: it counts its runs and can be held like the section.
type fakeLibrary struct {
	mu      sync.Mutex
	calls   int
	gate    chan struct{}
	started chan struct{}
}

type fakeMetadataRefresh struct {
	started chan struct{}
	gate    chan struct{}
	err     error
}

func (f *fakeMetadataRefresh) RefreshMissingEpisodes(ctx context.Context) error {
	f.started <- struct{}{}
	if f.gate != nil {
		select {
		case <-f.gate:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return f.err
}

func (f *fakeLibrary) Scan(ctx context.Context) error {
	f.mu.Lock()
	f.calls++
	gate := f.gate
	f.mu.Unlock()
	f.started <- struct{}{}
	if gate != nil {
		select {
		case <-gate:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return nil
}

func (f *fakeLibrary) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

// fakeScanStore keeps the times in memory and tells each saved end on saved.
type fakeScanStore struct {
	mu    sync.Mutex
	times map[domain.ScanKind]time.Time
	saved chan domain.ScanKind
}

func (f *fakeScanStore) Finished(context.Context) (map[domain.ScanKind]time.Time, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := map[domain.ScanKind]time.Time{}
	for k, v := range f.times {
		out[k] = v
	}
	return out, nil
}

func (f *fakeScanStore) SetFinished(_ context.Context, kind domain.ScanKind, at time.Time) error {
	f.mu.Lock()
	f.times[kind] = at
	f.mu.Unlock()
	f.saved <- kind
	return nil
}

func (f *fakeScanStore) when(kind domain.ScanKind) (time.Time, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	at, ok := f.times[kind]
	return at, ok
}

// fakeArrivals stands for the taking of the Samba copies.
type fakeArrivals struct{}

func (*fakeArrivals) Adopt(context.Context) error { return nil }

// scanRig is a scanner over fakes, with the clock the test moves.
type scanRig struct {
	t        *testing.T
	ctx      context.Context
	cancel   context.CancelFunc
	section  *fakeSection
	library  *fakeLibrary
	store    *fakeScanStore
	timers   *fakeTimers
	arrivals *fakeArrivals
	now      time.Time
	scanner  *application.Scanner
	// pending holds the saves that arrived before the test asked for them.
	pending []domain.ScanKind
}

// newScanRig builds the fakes and a scanner over them with the intervals of RF-28 (5 min and 1 h). A nil
// gate lets its scan finish at once; a gate holds the scan until it is closed.
func newScanRig(t *testing.T, sectionGate, libraryGate chan struct{}) *scanRig {
	t.Helper()
	ctx, cancel := context.WithCancel(t.Context())
	t.Cleanup(cancel)
	r := &scanRig{
		t: t, ctx: ctx, cancel: cancel,
		section:  &fakeSection{gate: sectionGate, started: make(chan struct{}, 32)},
		library:  &fakeLibrary{gate: libraryGate, started: make(chan struct{}, 32)},
		store:    &fakeScanStore{times: map[domain.ScanKind]time.Time{}, saved: make(chan domain.ScanKind, 32)},
		timers:   &fakeTimers{requests: make(chan timerRequest, 32)},
		arrivals: &fakeArrivals{},
		now:      time.Date(2026, time.October, 13, 10, 0, 0, 0, time.UTC),
	}
	r.scanner = r.newScanner(r.deps())
	return r
}

// deps are the dependencies of a scanner over the rig's fakes, with the intervals of RF-28.
func (r *scanRig) deps() application.ScanDeps {
	return application.ScanDeps{
		Section:         r.section,
		Arrivals:        r.arrivals,
		Library:         r.library,
		Store:           r.store,
		UnassignedEvery: 5 * time.Minute,
		LibraryEvery:    time.Hour,
		Now:             func() time.Time { return r.now },
		After:           r.timers.After,
	}
}

// newScanner builds a scanner over d, which must be valid.
func (r *scanRig) newScanner(d application.ScanDeps) *application.Scanner {
	r.t.Helper()
	s, err := application.NewScanner(r.ctx, d)
	if err != nil {
		r.t.Fatalf("NewScanner: %v", err)
	}
	return s
}

// awaitSave waits until the scan of kind saves its end. Saves of the other scan that come first are kept
// for the next wait.
func (r *scanRig) awaitSave(kind domain.ScanKind) {
	r.t.Helper()
	for i, k := range r.pending {
		if k == kind {
			r.pending = append(r.pending[:i], r.pending[i+1:]...)
			return
		}
	}
	for {
		got := await(r.t, r.store.saved, "the "+string(kind)+" scan to end")
		if got == kind {
			return
		}
		r.pending = append(r.pending, got)
	}
}

// awaitAnySave waits until some scan saves its end.
func (r *scanRig) awaitAnySave() {
	r.t.Helper()
	if len(r.pending) > 0 {
		r.pending = r.pending[1:]
		return
	}
	await(r.t, r.store.saved, "a scan to end")
}

// statusOf returns the status of one scan from a list of both.
func statusOf(list []application.ScanStatus, kind domain.ScanKind) application.ScanStatus {
	for _, st := range list {
		if st.Kind == kind {
			return st
		}
	}
	return application.ScanStatus{}
}

func TestNewScannerRejectsWhatItCannotRun(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		edit func(d *application.ScanDeps)
	}{
		{name: "zero unassigned interval", edit: func(d *application.ScanDeps) { d.UnassignedEvery = 0 }},
		{name: "negative library interval", edit: func(d *application.ScanDeps) { d.LibraryEvery = -time.Hour }},
		{name: "no section", edit: func(d *application.ScanDeps) { d.Section = nil }},
		{name: "no store", edit: func(d *application.ScanDeps) { d.Store = nil }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			r := newScanRig(t, nil, nil)
			d := r.deps()
			tt.edit(&d)
			if _, err := application.NewScanner(t.Context(), d); err == nil {
				t.Fatal("NewScanner accepted a scanner it cannot run")
			}
		})
	}
}

func TestTheLibraryScanStartsMetadataRefreshInTheBackground(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		err  error
	}{
		{name: "TMDB answers"},
		{name: "TMDB fails", err: errors.New("TMDB unavailable")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			r := newScanRig(t, nil, nil)
			refresh := &fakeMetadataRefresh{started: make(chan struct{}, 1), gate: make(chan struct{}), err: tt.err}
			d := r.deps()
			d.LibraryMetadataRefresh = refresh
			s := r.newScanner(d)
			s.Trigger()
			r.awaitSave(domain.ScanLibrary)
			await(t, refresh.started, "metadata refresh")
			close(refresh.gate)
			if at, ok := r.store.when(domain.ScanLibrary); !ok || at.IsZero() {
				t.Fatalf("library scan was not saved after background refresh: %v %v", at, ok)
			}
		})
	}
}

func TestEachScanWaitsItsOwnIntervalAndRunsAtStart(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		fire        time.Duration
		wantSection int
		wantLibrary int
	}{
		{name: "the unassigned scan runs again after its interval", fire: 5 * time.Minute, wantSection: 2, wantLibrary: 1},
		{name: "the library scan runs again after its interval", fire: time.Hour, wantSection: 1, wantLibrary: 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			r := newScanRig(t, nil, nil)
			r.scanner.Start()
			await(t, r.section.started, "the unassigned scan at start")
			await(t, r.library.started, "the library scan at start")
			r.awaitSave(domain.ScanUnassigned)
			r.awaitSave(domain.ScanLibrary)

			// Each loop asks the clock for its own interval once its scan at start has ended.
			asked := map[time.Duration]timerRequest{}
			for range 2 {
				req := await(t, r.timers.requests, "a wait for the next run")
				asked[req.every] = req
			}
			if len(asked) != 2 {
				t.Fatalf("waits asked for %d distinct intervals, want 5m and 1h", len(asked))
			}
			if _, ok := asked[5*time.Minute]; !ok {
				t.Fatal("the unassigned scan did not ask for its 5m interval")
			}
			if _, ok := asked[time.Hour]; !ok {
				t.Fatal("the library scan did not ask for its 1h interval")
			}

			asked[tt.fire].fire <- r.now
			if tt.wantSection == 2 {
				await(t, r.section.started, "the unassigned scan after its interval")
				r.awaitSave(domain.ScanUnassigned)
			} else {
				await(t, r.library.started, "the library scan after its interval")
				r.awaitSave(domain.ScanLibrary)
			}
			if got := r.section.count(); got != tt.wantSection {
				t.Fatalf("unassigned runs = %d, want %d", got, tt.wantSection)
			}
			if got := r.library.count(); got != tt.wantLibrary {
				t.Fatalf("library runs = %d, want %d", got, tt.wantLibrary)
			}
		})
	}
}

func TestTheStartDoesNotWaitForTheScanAtStart(t *testing.T) {
	t.Parallel()

	gate := make(chan struct{})
	r := newScanRig(t, gate, nil)
	done := r.scanner.Start() // it returns at once, while the scan at start is held
	await(t, r.section.started, "the unassigned scan at start")

	if got := statusOf(r.scanner.Status(), domain.ScanUnassigned); !got.Running {
		t.Fatalf("status while the scan at start runs = %+v, want running", got)
	}
	select {
	case <-done:
		t.Fatal("the scans ended while one of them was held")
	default:
	}
	close(gate)
	r.awaitSave(domain.ScanUnassigned)
	if got := statusOf(r.scanner.Status(), domain.ScanUnassigned); got.Running || got.Finished.IsZero() {
		t.Fatalf("status after the scan at start = %+v, want finished and not running", got)
	}
}

func TestEscanearAhoraStartsOnlyTheScansThatAreNotRunning(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		// preRun says an «Escanear ahora» ran first, with the holds in place.
		preRun      bool
		sectionHold bool
		libraryHold bool
		wantSection int
		wantLibrary int
	}{
		{name: "nothing runs: both start", sectionHold: true, libraryHold: true, wantSection: 1, wantLibrary: 1},
		{name: "the unassigned scan runs: only the library starts", preRun: true, sectionHold: true, wantSection: 1, wantLibrary: 2},
		{name: "the library scan runs: only the unassigned starts", preRun: true, libraryHold: true, wantSection: 2, wantLibrary: 1},
		{name: "both run: none starts", preRun: true, sectionHold: true, libraryHold: true, wantSection: 1, wantLibrary: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var sectionGate, libraryGate chan struct{}
			if tt.sectionHold {
				sectionGate = make(chan struct{})
			}
			if tt.libraryHold {
				libraryGate = make(chan struct{})
			}
			r := newScanRig(t, sectionGate, libraryGate)
			// The scans are started by the trigger here, not by Start: the test drives every run.
			saves := 0
			if tt.preRun {
				r.scanner.Trigger()
				if !tt.sectionHold {
					r.awaitSave(domain.ScanUnassigned)
					saves++
				}
				if !tt.libraryHold {
					r.awaitSave(domain.ScanLibrary)
					saves++
				}
			}

			// A held scan is still running when the trigger answers; a scan that is not held may already have
			// ended, so only the held ones are asserted here. The counts below show what started.
			started := r.scanner.Trigger()
			if got := statusOf(started, domain.ScanUnassigned); tt.sectionHold && !got.Running {
				t.Fatalf("unassigned status right after the trigger = %+v, want running", got)
			}
			if got := statusOf(started, domain.ScanLibrary); tt.libraryHold && !got.Running {
				t.Fatalf("library status right after the trigger = %+v, want running", got)
			}

			// Release what is held, then wait for the rest of the runs to end.
			if tt.sectionHold {
				close(sectionGate)
			}
			if tt.libraryHold {
				close(libraryGate)
			}
			for range tt.wantSection + tt.wantLibrary - saves {
				r.awaitAnySave()
			}
			if got := r.section.count(); got != tt.wantSection {
				t.Fatalf("unassigned runs = %d, want %d", got, tt.wantSection)
			}
			if got := r.library.count(); got != tt.wantLibrary {
				t.Fatalf("library runs = %d, want %d", got, tt.wantLibrary)
			}
		})
	}
}

func TestAScanNeverOverlapsItselfWhileItRuns(t *testing.T) {
	t.Parallel()

	gate := make(chan struct{})
	r := newScanRig(t, gate, nil)
	r.scanner.Start()
	await(t, r.section.started, "the unassigned scan at start")
	r.awaitSave(domain.ScanLibrary) // only the library can end while the section is held

	// Two «Escanear ahora» while the scan at start is held: the unassigned scan is not started again.
	r.scanner.Trigger()
	r.scanner.Trigger()
	close(gate)
	r.awaitSave(domain.ScanUnassigned)
	if got := r.section.count(); got != 1 {
		t.Fatalf("unassigned runs = %d, want 1: a running scan was started again", got)
	}
}

func TestTheTimesOfTheScansAreSavedAndSurviveARestart(t *testing.T) {
	t.Parallel()

	r := newScanRig(t, nil, nil)
	r.scanner.Start()
	r.awaitSave(domain.ScanUnassigned)
	r.awaitSave(domain.ScanLibrary)

	// The process restarts: a new scanner over the same store reads the times the old one saved.
	restarted := r.newScanner(application.ScanDeps{
		Section: r.section, Store: r.store, UnassignedEvery: time.Minute, LibraryEvery: time.Minute,
	})
	for _, kind := range domain.ScanKinds() {
		want, saved := r.store.when(kind)
		if !saved {
			t.Fatalf("%s: no time saved", kind)
		}
		got := statusOf(restarted.Status(), kind)
		if !got.Finished.Equal(want) || got.Running {
			t.Fatalf("%s after restart = %+v, want finished at %s and not running", kind, got, want)
		}
	}
}

func TestAScanCutByTheEndOfTheContextIsNotCountedAsFinished(t *testing.T) {
	t.Parallel()

	gate := make(chan struct{})
	r := newScanRig(t, gate, nil)
	done := r.scanner.Start()
	await(t, r.section.started, "the unassigned scan at start")

	r.cancel()
	select {
	case <-done:
	case <-time.After(guard):
		t.Fatal("the scans did not end after their context ended")
	}
	if _, saved := r.store.when(domain.ScanUnassigned); saved {
		t.Fatal("a scan cut by the shutdown was saved as finished")
	}
	if got := statusOf(r.scanner.Status(), domain.ScanUnassigned); got.Running || !got.Finished.IsZero() {
		t.Fatalf("status after the shutdown = %+v, want not running and never finished", got)
	}
}

func TestTheScansReadTheSectionWithTheSambaArrivals(t *testing.T) {
	t.Parallel()

	r := newScanRig(t, nil, nil)
	r.scanner.Start()
	r.awaitSave(domain.ScanUnassigned)
	r.awaitSave(domain.ScanLibrary)

	r.section.mu.Lock()
	got := r.section.gotArrivals
	r.section.mu.Unlock()
	if got != application.Arrivals(r.arrivals) {
		t.Fatalf("the scan read the section with arrivals %v, want the scanner's Samba arrivals", got)
	}
}

func TestAScanWithoutALibraryOnlyRecordsItsTime(t *testing.T) {
	t.Parallel()

	r := newScanRig(t, nil, nil)
	d := r.deps()
	d.Library = nil
	s := r.newScanner(d)
	s.Trigger()
	r.awaitSave(domain.ScanUnassigned)
	r.awaitSave(domain.ScanLibrary)
	if _, saved := r.store.when(domain.ScanLibrary); !saved {
		t.Fatal("the library scan without a library did not record its time")
	}
	if got := r.library.count(); got != 0 {
		t.Fatalf("the rig's library ran %d times, want 0 when the scanner has none", got)
	}
}

func TestAFailedScanStillRecordsItsEnd(t *testing.T) {
	t.Parallel()

	r := newScanRig(t, nil, nil)
	r.section.mu.Lock()
	r.section.err = errors.New("library unmounted")
	r.section.mu.Unlock()
	r.scanner.Trigger()
	r.awaitSave(domain.ScanUnassigned)
	r.awaitSave(domain.ScanLibrary)

	got := statusOf(r.scanner.Status(), domain.ScanUnassigned)
	if got.Running || got.Finished.IsZero() {
		t.Fatalf("status of a failed scan = %+v, want finished and not running", got)
	}
}
