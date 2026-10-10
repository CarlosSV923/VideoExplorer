package tmdb

import (
	"net/http"
	"testing"
	"time"
)

func TestRetryAfterParsesSecondsAndDates(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, time.October, 10, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		name   string
		value  string
		want   time.Duration
		wantOK bool
	}{
		{"absent", "", 0, false},
		{"seconds", "7", 7 * time.Second, true},
		{"zero seconds", "0", 0, true},
		{"negative seconds are unreadable", "-3", 0, false},
		{"http date in the future", now.Add(30 * time.Second).Format(http.TimeFormat), 30 * time.Second, true},
		{"http date in the past", now.Add(-time.Minute).Format(http.TimeFormat), 0, true},
		{"garbage", "soon", 0, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, ok := retryAfter(tt.value, now)
			if ok != tt.wantOK || got != tt.want {
				t.Errorf("retryAfter(%q) = (%v, %v), want (%v, %v)", tt.value, got, ok, tt.want, tt.wantOK)
			}
		})
	}
}

func TestBackoffDoublesAndIsCapped(t *testing.T) {
	t.Parallel()

	c := &Client{cfg: Config{RetryDelay: 500 * time.Millisecond, MaxRetryAfter: 2 * time.Second}}
	tests := []struct {
		attempt int
		want    time.Duration
	}{
		{1, 500 * time.Millisecond},
		{2, time.Second},
		{3, 2 * time.Second},
		{4, 2 * time.Second},
	}
	for _, tt := range tests {
		if got := c.backoff(tt.attempt); got != tt.want {
			t.Errorf("backoff(%d) = %v, want %v", tt.attempt, got, tt.want)
		}
	}
}
