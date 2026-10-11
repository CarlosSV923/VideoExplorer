package domain_test

import (
	"testing"
	"time"

	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/ingestion/domain"
)

func TestFileIdentityMatchesOnlyTheSameContentOfTheSameFile(t *testing.T) {
	t.Parallel()

	stored := domain.FileIdentity{Device: 7, Inode: 42, Size: 1000, ModTime: time.Unix(1_700_000_000, 123)}
	tests := []struct {
		name    string
		current domain.FileIdentity
		want    bool
	}{
		{name: "the same file", current: stored, want: true},
		{
			name:    "renamed inside its file system: same device and inode, size and mtime kept",
			current: stored, want: true,
		},
		{
			name:    "another inode",
			current: domain.FileIdentity{Device: 7, Inode: 43, Size: 1000, ModTime: stored.ModTime},
		},
		{
			name:    "another device",
			current: domain.FileIdentity{Device: 8, Inode: 42, Size: 1000, ModTime: stored.ModTime},
		},
		{
			name:    "size changed",
			current: domain.FileIdentity{Device: 7, Inode: 42, Size: 1001, ModTime: stored.ModTime},
		},
		{
			name:    "modification time changed by a nanosecond",
			current: domain.FileIdentity{Device: 7, Inode: 42, Size: 1000, ModTime: stored.ModTime.Add(time.Nanosecond)},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := stored.Matches(tt.current); got != tt.want {
				t.Fatalf("Matches = %v, want %v", got, tt.want)
			}
		})
	}
}
