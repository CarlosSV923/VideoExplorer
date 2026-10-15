package application

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"sync"
	"time"

	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/catalog/domain"
)

// SectionReader is the reading of No asignados that the scan of _unassigned/ runs (Section.Sync).
type SectionReader interface {
	Sync(ctx context.Context, arrivals Arrivals) ([]SectionEntry, error)
}

// LibraryScan scans Movies/ and TV Shows/ (RF-29): it registers what is recognized in place, moves what
// cannot be read to No asignados and forgets what is gone (see LibraryImport).
type LibraryScan interface {
	Scan(ctx context.Context) error
}

// ScanStore keeps, for each scan, when it last finished (RF-28), so that the times survive a restart.
type ScanStore interface {
	// Finished returns when each scan last finished. A scan that never did is absent.
	Finished(ctx context.Context) (map[domain.ScanKind]time.Time, error)
	// SetFinished records when a scan finished.
	SetFinished(ctx context.Context, kind domain.ScanKind, at time.Time) error
}

// ScanDeps are what the scans of the library need (RF-28).
type ScanDeps struct {
	// Section is the reading of _unassigned/ that its scan runs.
	Section SectionReader
	// Arrivals takes the settled copies of Samba before the section is read (RF-27a). May be nil.
	Arrivals Arrivals
	// Library is the scan of the library. May be nil: the scan then only records its time.
	Library LibraryScan
	// Store keeps the times the scans finished.
	Store ScanStore
	// UnassignedEvery and LibraryEvery are the intervals of the two scans (UNASSIGNED_SCAN_INTERVAL and
	// LIBRARY_SCAN_INTERVAL). Both must be positive.
	UnassignedEvery time.Duration
	LibraryEvery    time.Duration
	// Now and After may be nil (time.Now and time.After). The tests replace them.
	Now   func() time.Time
	After func(time.Duration) <-chan time.Time
	// Log may be nil.
	Log *slog.Logger
}

// ScanStatus is one scan as the API shows it (RF-28).
type ScanStatus struct {
	Kind domain.ScanKind
	// Finished is when the scan last finished. It is the zero time when it never did.
	Finished time.Time
	// Running says whether the scan is running now.
	Running bool
}

// Scanner runs the two scans of the library (RF-28): _unassigned/ every UnassignedEvery and the library
// every LibraryEvery, each once when it starts and then at its interval, and both on demand (Trigger).
// A scan never overlaps itself: a scan that is running is not started again. The scans stop when the
// context given to NewScanner ends, and a scan that the end cuts short does not count as finished.
type Scanner struct {
	d    ScanDeps
	base context.Context

	mu       sync.Mutex
	closed   bool
	running  map[domain.ScanKind]bool
	finished map[domain.ScanKind]time.Time
	// wg counts the loops and the scans started on demand, so that the end of the scans can be awaited.
	wg sync.WaitGroup
}

// NewScanner builds the scanner and reads the times that the scans finished before (RF-28). ctx is the
// context the scans run under.
func NewScanner(ctx context.Context, d ScanDeps) (*Scanner, error) {
	if d.UnassignedEvery <= 0 || d.LibraryEvery <= 0 {
		return nil, errors.New("catalog: the scan intervals must be positive")
	}
	if d.Section == nil || d.Store == nil {
		return nil, errors.New("catalog: the scanner needs a section and a store")
	}
	if d.Now == nil {
		d.Now = time.Now
	}
	if d.After == nil {
		d.After = time.After
	}
	if d.Log == nil {
		d.Log = slog.New(slog.DiscardHandler)
	}
	finished, err := d.Store.Finished(ctx)
	if err != nil {
		return nil, fmt.Errorf("catalog: read the scan times: %w", err)
	}
	s := &Scanner{
		d: d, base: ctx,
		running:  map[domain.ScanKind]bool{},
		finished: map[domain.ScanKind]time.Time{},
	}
	maps.Copy(s.finished, finished)
	return s, nil
}

// Start begins the scans: each runs at once, without waiting for its interval, and then every interval.
// It returns at once. The channel it returns closes when the scans have ended after the context given to
// NewScanner ends. Start is called once.
func (s *Scanner) Start() <-chan struct{} {
	s.mu.Lock()
	s.wg.Add(2)
	s.mu.Unlock()
	go s.loop(domain.ScanUnassigned, s.d.UnassignedEvery)
	go s.loop(domain.ScanLibrary, s.d.LibraryEvery)

	done := make(chan struct{})
	go func() {
		<-s.base.Done()
		s.mu.Lock()
		s.closed = true
		s.mu.Unlock()
		s.wg.Wait()
		close(done)
	}()
	return done
}

// Trigger starts each scan that is not running now («Escanear ahora», RF-28). A scan that is running is
// not started again. It returns the state of both scans right after the call; nothing waits for them.
func (s *Scanner) Trigger() []ScanStatus {
	s.mu.Lock()
	if !s.closed && s.base.Err() == nil {
		for _, kind := range domain.ScanKinds() {
			if s.running[kind] {
				continue
			}
			s.running[kind] = true
			s.wg.Add(1)
			go func() {
				defer s.wg.Done()
				s.execute(kind)
			}()
		}
	}
	s.mu.Unlock()
	return s.Status()
}

// Status returns the state of both scans.
func (s *Scanner) Status() []ScanStatus {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]ScanStatus, 0, len(domain.ScanKinds()))
	for _, kind := range domain.ScanKinds() {
		out = append(out, ScanStatus{Kind: kind, Finished: s.finished[kind], Running: s.running[kind]})
	}
	return out
}

// loop runs the scan of one kind at start and then after every interval, until the context ends. The
// wait starts when a run ends, so the runs of a kind never overlap.
func (s *Scanner) loop(kind domain.ScanKind, every time.Duration) {
	defer s.wg.Done()
	for {
		s.tick(kind)
		select {
		case <-s.base.Done():
			return
		case <-s.d.After(every):
		}
	}
}

// tick runs the scan of kind for a timer, unless it is running or the scans are ending.
func (s *Scanner) tick(kind domain.ScanKind) {
	s.mu.Lock()
	ok := !s.closed && s.base.Err() == nil && !s.running[kind]
	if ok {
		s.running[kind] = true
	}
	s.mu.Unlock()
	if ok {
		s.execute(kind)
	}
}

// execute runs the scan of kind, which the caller has marked as running, and records when it finished.
// A scan that the end of the context cuts short is not recorded: the next start runs it again. A scan
// that fails is recorded as finished too, and its error is logged.
func (s *Scanner) execute(kind domain.ScanKind) {
	ctx := s.base
	err := s.scan(ctx, kind)
	if err != nil && ctx.Err() != nil {
		s.setRunning(kind, false)
		return
	}
	if err != nil {
		s.d.Log.WarnContext(ctx, "scan failed", "scan", string(kind), "error", err)
	}
	at := s.d.Now()
	s.mu.Lock()
	s.finished[kind] = at
	s.running[kind] = false
	s.mu.Unlock()
	if err := s.d.Store.SetFinished(context.WithoutCancel(ctx), kind, at); err != nil {
		s.d.Log.WarnContext(ctx, "scan time not saved", "scan", string(kind), "error", err)
	}
}

// scan runs the work of one scan.
func (s *Scanner) scan(ctx context.Context, kind domain.ScanKind) error {
	switch kind {
	case domain.ScanUnassigned:
		_, err := s.d.Section.Sync(ctx, s.d.Arrivals)
		return err
	case domain.ScanLibrary:
		if s.d.Library == nil {
			return nil
		}
		return s.d.Library.Scan(ctx)
	default:
		return fmt.Errorf("catalog: unknown scan %q", kind)
	}
}

// setRunning marks a scan as running or not.
func (s *Scanner) setRunning(kind domain.ScanKind, running bool) {
	s.mu.Lock()
	s.running[kind] = running
	s.mu.Unlock()
}
