package infrastructure

import (
	"sync"
	"time"

	"golang.org/x/time/rate"
)

// MemoryThrottle is a per-key token bucket kept in memory.
type MemoryThrottle struct {
	mu      sync.Mutex
	limit   rate.Limit
	burst   int
	idleTTL time.Duration
	now     func() time.Time
	clients map[string]*client
}

type client struct {
	limiter  *rate.Limiter
	lastSeen time.Time
}

// NewMemoryThrottle allows burst attempts at once, refilled at one per every.
func NewMemoryThrottle(every time.Duration, burst int, now func() time.Time) *MemoryThrottle {
	if now == nil {
		now = time.Now
	}
	return &MemoryThrottle{
		limit:   rate.Every(every),
		burst:   burst,
		idleTTL: 30 * time.Minute,
		now:     now,
		clients: map[string]*client{},
	}
}

// Allow implements application.Throttle.
func (t *MemoryThrottle) Allow(key string) bool {
	t.mu.Lock()
	defer t.mu.Unlock()

	now := t.now()
	for k, c := range t.clients { // tiny map on a LAN: lazy eviction is enough
		if now.Sub(c.lastSeen) > t.idleTTL {
			delete(t.clients, k)
		}
	}
	c, ok := t.clients[key]
	if !ok {
		c = &client{limiter: rate.NewLimiter(t.limit, t.burst)}
		t.clients[key] = c
	}
	c.lastSeen = now
	return c.limiter.AllowN(now, 1)
}
