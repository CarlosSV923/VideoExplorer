package domain_test

import (
	"testing"
	"time"

	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/identity/domain"
)

func TestSessionActiveAt(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 10, 10, 12, 0, 0, 0, time.UTC)
	s := domain.NewSession(now, time.Hour)

	tests := []struct {
		name string
		at   time.Time
		want bool
	}{
		{"at issue time", now, true},
		{"just before expiry", now.Add(59 * time.Minute), true},
		{"at expiry", now.Add(time.Hour), false},
		{"after expiry", now.Add(2 * time.Hour), false},
		{"small clock skew", now.Add(-30 * time.Second), true},
		{"issued in the future", now.Add(-2 * time.Minute), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := s.ActiveAt(tt.at); got != tt.want {
				t.Errorf("ActiveAt = %v, want %v", got, tt.want)
			}
		})
	}
}
