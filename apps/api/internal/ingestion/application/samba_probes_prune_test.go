package application_test

import (
	"path/filepath"
	"testing"
)

// The readings of copies that left _unassigned/ (taken or removed) are forgotten at the next reading of the
// section (RF-27a); the readings of copies still there are kept.
func TestTheAdoptionForgetsTheReadingsOfCopiesThatLeftUnassigned(t *testing.T) {
	t.Parallel()

	r, f, lib := sambaRig(t)
	present := filepath.Join(lib, "video.mkv")
	gone := filepath.Join(lib, "gone.zip")
	probes := &fakeProbes{entries: map[string]probeEntry{
		present: {path: present, size: 1, mtime: 1},
		gone:    {path: gone, size: 1, mtime: 1, archive: true},
	}}
	r.svc.WithArchiveProbes(probes)
	f.copy(t, lib, "video.mkv", "", []byte("not an archive, only a video"), false)

	if _, err := r.svc.AdoptSamba(t.Context()); err != nil {
		t.Fatalf("AdoptSamba: %v", err)
	}
	if _, ok := probes.entries[gone]; ok {
		t.Error("the reading of a copy that left _unassigned/ is still kept")
	}
	if _, ok := probes.entries[present]; !ok {
		t.Error("the reading of a copy still in _unassigned/ was forgotten")
	}
}
