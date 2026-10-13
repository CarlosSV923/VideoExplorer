package application

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/ingestion/domain"
)

// settlerGuard bounds the waits of these tests. A healthy run never gets near it.
const settlerGuard = 10 * time.Second

// workerWakes reports whether a pass is pending, consuming its token as the worker does.
func workerWakes(st *settler) bool {
	select {
	case <-st.wake:
		return true
	default:
		return false
	}
}

// startedSettler returns a settler whose start-up pass ran and decided its requests.
func startedSettler(t *testing.T) *settler {
	t.Helper()
	st := newSettler()
	if !workerWakes(st) {
		t.Fatal("a new settler has no pending pass")
	}
	covers, _, _ := st.take()
	st.finish(covers)
	return st
}

func marks(ids ...string) []domain.BatchID {
	out := make([]domain.BatchID, 0, len(ids))
	for _, id := range ids {
		out = append(out, domain.BatchID(id))
	}
	return out
}

func repeat(id string, n int) []domain.BatchID {
	out := make([]domain.BatchID, 0, n)
	for range n {
		out = append(out, domain.BatchID(id))
	}
	return out
}

// TestSettlerCoalescesRequestsIntoOnePendingPass: however many requests arrive while the worker
// is busy, the worker runs one pass for them, and one more after the pass in which they arrived.
func TestSettlerCoalescesRequestsIntoOnePendingPass(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		// before are marked before the worker takes a pass.
		before []domain.BatchID
		// during are marked while the first pass runs.
		during []domain.BatchID
		// passes is how many passes the worker runs.
		passes int
		// last are the batches the last pass decides.
		last []domain.BatchID
	}{
		{name: "no request is no pass", passes: 0},
		{name: "one request is one pass", before: marks("a"), passes: 1, last: marks("a")},
		{name: "a burst before the pass is one pass", before: marks("a", "b", "a", "c"), passes: 1, last: marks("a", "b", "c")},
		{name: "requests during a pass are one more pass", before: marks("a"), during: marks("b", "c", "b"), passes: 2, last: marks("b", "c")},
		{name: "a hundred requests during a pass are still one more pass", before: marks("a"), during: repeat("b", 100), passes: 2, last: marks("b")},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			st := startedSettler(t)
			for _, b := range tc.before {
				st.mark(b)
			}
			var last []domain.BatchID
			passes := 0
			for workerWakes(st) {
				_, _, last = st.take()
				passes++
				if passes == 1 {
					for _, b := range tc.during {
						st.mark(b)
					}
				}
			}
			if passes != tc.passes {
				t.Errorf("passes = %d, want %d", passes, tc.passes)
			}
			if !slices.Equal(last, tc.last) {
				t.Errorf("last pass decides %v, want %v", last, tc.last)
			}
		})
	}
}

// TestSettlerRequestsNeverWait: a request returns at once even when no worker runs, so that no
// caller of a request (a DELETE, a retry, a filter) waits for a decision.
func TestSettlerRequestsNeverWait(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		requests int
		all      bool
	}{
		{name: "one request", requests: 1},
		{name: "ten thousand requests", requests: 10_000},
		{name: "a pass over every batch", requests: 1, all: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			st := startedSettler(t)
			done := make(chan struct{})
			go func() {
				defer close(done)
				for i := range tc.requests {
					st.mark(domain.BatchID(string(rune('a' + i%26))))
				}
				if tc.all {
					st.markAll()
				}
			}()
			select {
			case <-done:
			case <-time.After(settlerGuard):
				t.Fatal("a request waited for the worker")
			}
		})
	}
}

// TestSettlerWaitReturnsOnlyForTheRequestsItCovers: WaitSettled releases once the passes that
// took the requests made before it are over, and gives up with its context.
func TestSettlerWaitReturnsOnlyForTheRequestsItCovers(t *testing.T) {
	t.Parallel()

	cancelled, cancel := context.WithCancel(context.Background())
	cancel()

	tests := []struct {
		name string
		// run makes the requests and passes; it returns what wait returned.
		run  func(st *settler) error
		want error
	}{
		{
			name: "no request has nothing to wait for",
			run: func(st *settler) error {
				return st.wait(cancelled)
			},
		},
		{
			name: "a request decided by its pass returns at once",
			run: func(st *settler) error {
				st.mark("a")
				covers, _, _ := st.take()
				st.finish(covers)
				return st.wait(cancelled)
			},
		},
		{
			name: "a request made after the pass is not decided by it",
			run: func(st *settler) error {
				st.mark("a")
				covers, _, _ := st.take()
				st.mark("b")
				st.finish(covers)
				return st.wait(cancelled)
			},
			want: context.Canceled,
		},
		{
			name: "a request no pass takes gives up with its context",
			run: func(st *settler) error {
				st.mark("a")
				return st.wait(cancelled)
			},
			want: context.Canceled,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			st := startedSettler(t)
			if err := tc.run(st); !errors.Is(err, tc.want) {
				t.Fatalf("wait = %v, want %v", err, tc.want)
			}
		})
	}
}

// TestSettlerWaiterIsReleasedByThePassThatDecidesIt: a waiter wakes up when the pass that covers
// its request is over, and not before: a pass that started before the request does not release it.
func TestSettlerWaiterIsReleasedByThePassThatDecidesIt(t *testing.T) {
	t.Parallel()

	cancelled, cancel := context.WithCancel(context.Background())
	cancel()

	tests := []struct {
		name string
		// stale: a pass starts before a second request, and ends before its own pass.
		stale bool
	}{
		{name: "the pass that takes the request releases its waiter"},
		{name: "a pass that started before the request does not release its waiter", stale: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			st := startedSettler(t)
			st.mark("a")
			if tc.stale {
				covers, _, _ := st.take()
				st.mark("b")
				st.finish(covers)
				if err := st.wait(cancelled); !errors.Is(err, context.Canceled) {
					t.Fatalf("wait after the stale pass = %v, want the request still pending", err)
				}
			}

			released := make(chan error, 1)
			go func() { released <- st.wait(context.Background()) }()
			covers, _, _ := st.take()
			st.finish(covers)

			select {
			case err := <-released:
				if err != nil {
					t.Fatalf("wait = %v, want nil", err)
				}
			case <-time.After(settlerGuard):
				t.Fatal("the waiter was not released by the pass that decided its request")
			}
		})
	}
}
