// Package broker fans job changes out to the subscribers of the event stream (RF-10b).
package broker

import (
	"sync"
	"time"

	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/ingestion/application"
	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/ingestion/domain"
)

// ProgressInterval is the least time between two progress-only events of one
// job: at most two a second (RF-10b). A change of status or of failure reason
// is never held back by it.
const ProgressInterval = 500 * time.Millisecond

// subscriberBuffer is how many events a subscriber may fall behind. A subscriber
// that falls further behind is disconnected rather than left with missed
// changes: its client reconnects and receives the snapshot again.
const subscriberBuffer = 64

// sent is the last event published for a job, to tell progress from a change.
type sent struct {
	status  domain.Status
	failure domain.FailureReason
	at      time.Time
}

// Broker is an in-memory publish/subscribe hub for job changes.
type Broker struct {
	mu   sync.Mutex
	now  func() time.Time
	subs map[chan domain.JobView]struct{}
	last map[domain.JobID]sent
}

var _ application.Publisher = (*Broker)(nil)

// New builds a broker. now may be nil (time.Now).
func New(now func() time.Time) *Broker {
	if now == nil {
		now = time.Now
	}
	return &Broker{now: now, subs: map[chan domain.JobView]struct{}{}, last: map[domain.JobID]sent{}}
}

// Subscribe returns the channel of job changes and a function that stops the
// subscription. The channel is closed when the subscription ends, either by
// the function or because the subscriber fell too far behind.
func (b *Broker) Subscribe() (<-chan domain.JobView, func()) {
	ch := make(chan domain.JobView, subscriberBuffer)
	b.mu.Lock()
	b.subs[ch] = struct{}{}
	b.mu.Unlock()
	return ch, func() {
		b.mu.Lock()
		defer b.mu.Unlock()
		if _, ok := b.subs[ch]; ok {
			delete(b.subs, ch)
			close(ch)
		}
	}
}

// Publish implements application.Publisher. It never blocks. A change of status
// or failure reason is always sent; a progress-only event of a job is dropped
// when the previous event of that job was sent less than ProgressInterval ago.
func (b *Broker) Publish(v domain.JobView) {
	b.mu.Lock()
	defer b.mu.Unlock()

	now := b.now()
	job := v.Job
	prev, seen := b.last[job.ID]
	changed := !seen || prev.status != job.Status || prev.failure != job.Error
	if !changed && now.Sub(prev.at) < ProgressInterval {
		return
	}
	// Forget the jobs whose last event is old enough not to limit anything any more.
	for id, s := range b.last {
		if now.Sub(s.at) >= ProgressInterval {
			delete(b.last, id)
		}
	}
	b.last[job.ID] = sent{status: job.Status, failure: job.Error, at: now}

	for ch := range b.subs {
		select {
		case ch <- v:
		default:
			delete(b.subs, ch)
			close(ch)
		}
	}
}
