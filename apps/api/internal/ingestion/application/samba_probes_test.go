package application_test

import (
	"context"
	"errors"
	"path/filepath"
	"sync"
	"testing"

	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/ingestion/application"
)

// probeEntry is one reading kept by the fake probes.
type probeEntry struct {
	path    string
	size    int64
	mtime   int64
	archive bool
}

// fakeProbes keeps the readings in memory and counts what the adoption asked of them.
type fakeProbes struct {
	mu      sync.Mutex
	entries map[string]probeEntry
	// lookupErr makes every Lookup fail.
	lookupErr error
	stored    []probeEntry
}

func (p *fakeProbes) Lookup(_ context.Context, path string, size, mtime int64) (bool, bool, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.lookupErr != nil {
		return false, false, p.lookupErr
	}
	e, ok := p.entries[path]
	if !ok || e.size != size || e.mtime != mtime {
		return false, false, nil
	}
	return e.archive, true, nil
}

func (p *fakeProbes) Store(_ context.Context, path string, size, mtime int64, archive bool) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	e := probeEntry{path: path, size: size, mtime: mtime, archive: archive}
	p.stored = append(p.stored, e)
	if p.entries == nil {
		p.entries = map[string]probeEntry{}
	}
	p.entries[path] = e
	return nil
}

// setMtime gives the copy called name a modification time, in Unix milliseconds, as the section reads it.
func (f *fakeSamba) setMtime(name string, mtime int64) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for i := range f.files {
		if f.files[i].Name == name {
			f.files[i].Mtime = mtime
		}
	}
}

func TestTheAdoptionReadsAnArchiveHeaderOnlyWhenTheReadingIsNotKept(t *testing.T) {
	t.Parallel()

	const mtime = 1_800_000_000_000
	zip := videoZip(t)
	video := []byte("not an archive, only a video")
	tests := []struct {
		name string
		data []byte
		// kept is the reading the probes hold for the copy before the adoption; nil keeps none.
		kept *probeEntry
		// lookupErr makes the probes fail: the header decides, as without probes.
		lookupErr error
		// wantTaken says the copy is taken as an archive by this adoption.
		wantTaken bool
		// wantStored is how many readings the adoption keeps, and wantArchive what they say.
		wantStored  int
		wantArchive bool
	}{
		{
			name: "no reading kept: the header decides, and the reading is kept",
			data: zip, wantTaken: true, wantStored: 1, wantArchive: true,
		},
		{
			name: "a reading with the same size and time decides, and the header is not opened",
			data: zip, kept: &probeEntry{size: int64(len(zip)), mtime: mtime, archive: false},
			wantTaken: false, wantStored: 0,
		},
		{
			name: "a reading with another size is not used: the header decides again",
			data: zip, kept: &probeEntry{size: int64(len(zip)) + 1, mtime: mtime, archive: false},
			wantTaken: true, wantStored: 1, wantArchive: true,
		},
		{
			name: "a reading with another modification time is not used: the header decides again",
			data: zip, kept: &probeEntry{size: int64(len(zip)), mtime: mtime - 1, archive: false},
			wantTaken: true, wantStored: 1, wantArchive: true,
		},
		{
			name: "a copy that is not an archive is kept as not an archive",
			data: video, wantTaken: false, wantStored: 1, wantArchive: false,
		},
		{
			name: "a probe that fails is only logged: the header decides",
			data: zip, lookupErr: errors.New("database busy"),
			wantTaken: true, wantStored: 1, wantArchive: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			r, f, lib := sambaRig(t)
			source := filepath.Join(lib, "movie.zip")
			probes := &fakeProbes{lookupErr: tt.lookupErr}
			if tt.kept != nil {
				kept := *tt.kept
				kept.path = source
				probes.entries = map[string]probeEntry{source: kept}
			}
			r.svc.WithArchiveProbes(probes)
			f.copy(t, lib, "movie.zip", "", tt.data, false)
			f.setMtime("movie.zip", mtime)

			n, err := r.svc.AdoptSamba(t.Context())
			if err != nil {
				t.Fatalf("AdoptSamba: %v", err)
			}
			if got := n == 1; got != tt.wantTaken {
				t.Fatalf("taken = %v (units %d), want %v", got, n, tt.wantTaken)
			}
			if len(probes.stored) != tt.wantStored {
				t.Fatalf("readings kept = %d, want %d", len(probes.stored), tt.wantStored)
			}
			if tt.wantStored == 1 {
				got := probes.stored[0]
				if got.path != source || got.size != int64(len(tt.data)) || got.mtime != mtime || got.archive != tt.wantArchive {
					t.Fatalf("reading kept = %+v, want the copy at %s with its size, time and archive %v", got, source, tt.wantArchive)
				}
			}
		})
	}
}

// Compile-time check: the fake answers the port the service takes.
var _ application.ArchiveProbes = (*fakeProbes)(nil)
