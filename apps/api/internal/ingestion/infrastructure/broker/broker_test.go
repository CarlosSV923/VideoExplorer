package broker_test

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/ingestion/domain"
	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/ingestion/infrastructure/broker"
)

// clock is a time the test moves forward by hand.
type clock struct {
	mu  sync.Mutex
	now time.Time
}

func (c *clock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *clock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}

var start = time.Date(2026, time.October, 10, 12, 0, 0, 0, time.UTC)

// view is a job of the given id in the given state, received bytes and failure reason.
func view(id domain.JobID, status domain.Status, received int64, failure domain.FailureReason) domain.JobView {
	return domain.JobView{
		Job:   domain.UploadJob{ID: id, Status: status, Received: received, Size: 100, Error: failure},
		Batch: domain.Batch{ID: "b-1", Name: "Show"},
	}
}

// drain returns the events waiting on the channel without blocking.
func drain(ch <-chan domain.JobView) []domain.JobView {
	var out []domain.JobView
	for {
		select {
		case v, ok := <-ch:
			if !ok {
				return out
			}
			out = append(out, v)
		default:
			return out
		}
	}
}

func TestPublishLimitsProgressOnlyEventsOfAJob(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		// steps are the events published, and how long after the previous one.
		steps []struct {
			after    time.Duration
			view     domain.JobView
			wantSent bool
		}
	}{
		{
			name: "burst of progress within the interval is cut to the first",
			steps: []struct {
				after    time.Duration
				view     domain.JobView
				wantSent bool
			}{
				{view: view("j", domain.StatusUploading, 0, ""), wantSent: true},
				{after: 10 * time.Millisecond, view: view("j", domain.StatusUploading, 10, "")},
				{after: 100 * time.Millisecond, view: view("j", domain.StatusUploading, 20, "")},
				{after: 100 * time.Millisecond, view: view("j", domain.StatusUploading, 30, "")},
				{after: broker.ProgressInterval, view: view("j", domain.StatusUploading, 40, ""), wantSent: true},
			},
		},
		{
			name: "a change of status is never held back",
			steps: []struct {
				after    time.Duration
				view     domain.JobView
				wantSent bool
			}{
				{view: view("j", domain.StatusUploading, 5, ""), wantSent: true},
				{after: time.Millisecond, view: view("j", domain.StatusUploaded, 100, ""), wantSent: true},
			},
		},
		{
			name: "a failure reason is never held back",
			steps: []struct {
				after    time.Duration
				view     domain.JobView
				wantSent bool
			}{
				{view: view("j", domain.StatusUploading, 5, ""), wantSent: true},
				{after: time.Millisecond, view: view("j", domain.StatusFailed, 5, domain.FailureUploadAbandoned), wantSent: true},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			c := &clock{now: start}
			b := broker.New(c.Now)
			ch, unsubscribe := b.Subscribe()
			defer unsubscribe()

			for i, step := range tt.steps {
				c.Advance(step.after)
				b.Publish(step.view)
				got := drain(ch)
				if sent := len(got) == 1; sent != step.wantSent {
					t.Fatalf("step %d: sent = %v (%d events), want %v", i, sent, len(got), step.wantSent)
				}
			}
		})
	}
}

func TestProgressOfDifferentJobsIsLimitedSeparately(t *testing.T) {
	t.Parallel()

	c := &clock{now: start}
	b := broker.New(c.Now)
	ch, unsubscribe := b.Subscribe()
	defer unsubscribe()

	b.Publish(view("a", domain.StatusUploading, 1, ""))
	b.Publish(view("b", domain.StatusUploading, 1, ""))
	if got := drain(ch); len(got) != 2 {
		t.Fatalf("two jobs at the same instant: %d events, want 2", len(got))
	}
}

func TestEverySubscriberGetsEveryPublishedEventInOrder(t *testing.T) {
	t.Parallel()

	c := &clock{now: start}
	b := broker.New(c.Now)
	first, unsubFirst := b.Subscribe()
	defer unsubFirst()
	second, unsubSecond := b.Subscribe()
	defer unsubSecond()

	statuses := []domain.Status{domain.StatusUploading, domain.StatusUploaded, domain.StatusFailed}
	for _, s := range statuses {
		b.Publish(view("j", s, 0, ""))
	}
	for name, ch := range map[string]<-chan domain.JobView{"first": first, "second": second} {
		got := drain(ch)
		if len(got) != len(statuses) {
			t.Fatalf("%s subscriber got %d events, want %d", name, len(got), len(statuses))
		}
		for i, v := range got {
			if v.Job.Status != statuses[i] {
				t.Errorf("%s subscriber event %d = %s, want %s", name, i, v.Job.Status, statuses[i])
			}
		}
	}
}

func TestUnsubscribeStopsDeliveryAndClosesTheChannel(t *testing.T) {
	t.Parallel()

	c := &clock{now: start}
	b := broker.New(c.Now)
	ch, unsubscribe := b.Subscribe()
	unsubscribe()
	unsubscribe() // a second call is harmless

	b.Publish(view("j", domain.StatusUploading, 0, ""))
	if _, ok := <-ch; ok {
		t.Fatal("the channel of an unsubscribed stream delivered an event")
	}
}

func TestASubscriberThatFallsBehindIsDisconnected(t *testing.T) {
	t.Parallel()

	c := &clock{now: start}
	b := broker.New(c.Now)
	slow, unsubscribe := b.Subscribe()
	defer unsubscribe()

	// Nobody reads slow: its buffer fills and the broker cuts it off, instead of
	// leaving it with a gap in the changes. Each job changes status, so none is limited.
	for i := range 200 {
		b.Publish(view(domain.JobID(fmt.Sprintf("job-%d", i)), domain.StatusUploading, 0, ""))
	}
	closed := false
	for range 200 {
		if _, ok := <-slow; !ok {
			closed = true
			break
		}
	}
	if !closed {
		t.Fatal("a subscriber that fell behind was not disconnected")
	}
	unsubscribe() // closing it again after the broker did is harmless
}

func TestOtherSubscribersKeepReceivingWhileOneFallsBehind(t *testing.T) {
	t.Parallel()

	c := &clock{now: start}
	b := broker.New(c.Now)
	_, unsubSlow := b.Subscribe()
	defer unsubSlow()
	fast, unsubFast := b.Subscribe()
	defer unsubFast()

	for i := range 100 {
		b.Publish(view(domain.JobID(fmt.Sprintf("job-%d", i)), domain.StatusUploading, 0, ""))
		drain(fast)
	}
	b.Publish(view("last", domain.StatusDone, 0, ""))
	if got := drain(fast); len(got) != 1 || got[0].Job.ID != "last" {
		t.Fatalf("fast subscriber got %v, want the last event", got)
	}
}
