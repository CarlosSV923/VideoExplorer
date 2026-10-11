package application

import (
	"context"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/ingestion/domain"
)

// recordingProber records the paths it reads, in order, and fails for the paths in fail.
type recordingProber struct {
	mu   sync.Mutex
	read []string
	fail map[string]bool
}

func (p *recordingProber) Probe(_ context.Context, path string) (domain.MediaInfo, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.read = append(p.read, path)
	if p.fail[path] {
		return domain.MediaInfo{}, ErrNotVideo
	}
	return domain.MediaInfo{}, nil
}

func (p *recordingProber) paths() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]string(nil), p.read...)
}

func TestTheQueueTakesEachPathOnceInTheOrderItWasQueued(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		enqueue  []string
		wantRuns []string
	}{
		{name: "nothing queued", enqueue: nil, wantRuns: nil},
		{name: "in order", enqueue: []string{"a", "b", "c"}, wantRuns: []string{"a", "b", "c"}},
		{name: "a path queued twice waits once", enqueue: []string{"a", "b", "a", "c"}, wantRuns: []string{"a", "b", "c"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			p := &probing{queued: map[string]bool{}, wake: make(chan struct{}, 1)}
			for _, path := range tt.enqueue {
				p.enqueue(probeRequest{path: path})
			}
			var got []string
			for {
				req, ok := p.next()
				if !ok {
					break
				}
				got = append(got, req.path)
			}
			if !slices.Equal(got, tt.wantRuns) {
				t.Fatalf("taken = %v, want %v", got, tt.wantRuns)
			}
		})
	}
}

func TestRunProbingReadsTheQueueAndStopsWithItsContext(t *testing.T) {
	t.Parallel()

	prober := &recordingProber{fail: map[string]bool{"bad.mkv": true}}
	s := NewService(nil, nil, nil, nil, time.Now).WithProbing(prober)
	s.probes.enqueue(probeRequest{path: "bad.mkv"}) // a failing read does not stop the queue
	s.probes.enqueue(probeRequest{path: "good.mkv"})
	s.probes.enqueue(probeRequest{path: "good.mkv"}) // queued again while waiting: read once

	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan struct{})
	go func() {
		s.RunProbing(ctx)
		close(done)
	}()

	deadline := time.Now().Add(5 * time.Second)
	for len(prober.paths()) < 2 {
		if time.Now().After(deadline) {
			t.Fatalf("read %v, want the two queued paths", prober.paths())
		}
		time.Sleep(time.Millisecond)
	}
	s.probes.enqueue(probeRequest{path: "later.mkv"}) // a path queued after the worker waits is read too
	for len(prober.paths()) < 3 {
		if time.Now().After(deadline) {
			t.Fatalf("read %v, want later.mkv too", prober.paths())
		}
		time.Sleep(time.Millisecond)
	}
	cancel()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("RunProbing did not return after its context ended")
	}
	if want := []string{"bad.mkv", "good.mkv", "later.mkv"}; !slices.Equal(prober.paths(), want) {
		t.Fatalf("read %v, want %v", prober.paths(), want)
	}
}

func TestRunProbingWithoutProbingReturnsAtOnce(t *testing.T) {
	t.Parallel()

	s := NewService(nil, nil, nil, nil, time.Now)
	done := make(chan struct{})
	go func() {
		s.RunProbing(t.Context())
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("RunProbing blocked without WithProbing")
	}
}
