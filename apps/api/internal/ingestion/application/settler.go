package application

import (
	"context"
	"slices"
	"sync"

	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/ingestion/domain"
)

// settler coalesces the requests to decide the batches of the filter (RF-07, RF-08a). A request
// never waits: it marks what to decide and wakes the one worker (RunSettler). The worker takes
// every mark at once, so the requests made while a pass runs are decided by one pass after it:
// at most one pass is pending, however many jobs end meanwhile.
//
// A pass costs what it decides, not what the batches hold: a batch that is still on its way is
// checked with an indexed query and left alone; the job that ends last marks it again (see
// settleBatch). A burst of n jobs ending costs n checks and one decision, not n passes over
// everything (the cost that made 3000 files take minutes).
type settler struct {
	mu sync.Mutex
	// all asks for a pass over every batch that has a job being filtered (start, purge).
	all bool
	// marked holds the batches to decide in the next pass.
	marked map[domain.BatchID]struct{}
	// asked counts the requests made; decided is the count the finished passes covered.
	asked, decided uint64
	// ended is closed, and replaced, when a pass finishes: the waiters of wait wake up.
	ended chan struct{}
	// wake holds one token while a pass is pending. It never blocks the requester.
	wake chan struct{}
}

// newSettler returns a settler whose first pass is over every batch: a previous run may have
// left batches complete, and they are decided once the worker runs (RF-08a).
func newSettler() *settler {
	st := &settler{
		marked: map[domain.BatchID]struct{}{},
		ended:  make(chan struct{}),
		wake:   make(chan struct{}, 1),
	}
	st.markAll()
	return st
}

// mark asks for the batch to be decided in the next pass.
func (st *settler) mark(batch domain.BatchID) {
	st.mu.Lock()
	st.marked[batch] = struct{}{}
	st.asked++
	st.mu.Unlock()
	st.signal()
}

// markAll asks for a pass over every batch that has a job being filtered.
func (st *settler) markAll() {
	st.mu.Lock()
	st.all = true
	st.asked++
	st.mu.Unlock()
	st.signal()
}

// retry puts a batch back for the next pass without waking the worker. A batch that failed
// waits for the next request, instead of being retried in a loop.
func (st *settler) retry(batch domain.BatchID) {
	st.mu.Lock()
	st.marked[batch] = struct{}{}
	st.mu.Unlock()
}

// retryAll is retry for a pass over every batch.
func (st *settler) retryAll() {
	st.mu.Lock()
	st.all = true
	st.mu.Unlock()
}

func (st *settler) signal() {
	select {
	case st.wake <- struct{}{}:
	default: // a pass is already pending: it will see this request too
	}
}

// take hands the pending work to the worker. covers is the number of requests the pass decides.
func (st *settler) take() (covers uint64, all bool, batches []domain.BatchID) {
	st.mu.Lock()
	defer st.mu.Unlock()
	batches = make([]domain.BatchID, 0, len(st.marked))
	for batch := range st.marked {
		batches = append(batches, batch)
	}
	clear(st.marked)
	slices.Sort(batches)
	all, st.all = st.all, false
	return st.asked, all, batches
}

// finish records that the pass that took the requests up to covers is over, and wakes the waiters.
func (st *settler) finish(covers uint64) {
	st.mu.Lock()
	defer st.mu.Unlock()
	if covers > st.decided {
		st.decided = covers
	}
	close(st.ended)
	st.ended = make(chan struct{})
}

// wait returns once every request made before the call is decided, or when ctx ends.
func (st *settler) wait(ctx context.Context) error {
	st.mu.Lock()
	target := st.asked
	st.mu.Unlock()
	for {
		st.mu.Lock()
		if st.decided >= target {
			st.mu.Unlock()
			return nil
		}
		ended := st.ended
		st.mu.Unlock()
		select {
		case <-ended:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}
