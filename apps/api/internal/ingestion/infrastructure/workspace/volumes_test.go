package workspace_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/ingestion/domain"
	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/ingestion/infrastructure/workspace"
)

func TestClaimVolumeMovesTheStagedVolumeIntoItsSetsFolder(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	area := workspace.New(root, nil)
	const id = domain.JobID("vol1")
	const group = "vol1"
	if err := os.WriteFile(area.ArchivePath(id), []byte("volume one"), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := area.ClaimVolume(id, group, "Movie.7z.001"); err != nil {
		t.Fatalf("ClaimVolume: %v", err)
	}
	if area.ArchiveExists(id) {
		t.Error("the staged file is still there after the claim")
	}
	if !area.VolumeExists(group, "Movie.7z.001") {
		t.Fatal("the volume is not in its set's folder")
	}
	if got, want := area.VolumePath(group, "Movie.7z.001"), filepath.Join(root, "volumes", group, "Movie.7z.001"); got != want {
		t.Errorf("VolumePath = %q, want %q", got, want)
	}

	// After a restart the volume is already in place: claiming it again changes nothing.
	if err := area.ClaimVolume(id, group, "Movie.7z.001"); err != nil {
		t.Fatalf("second claim: %v", err)
	}
	if !area.VolumeExists(group, "Movie.7z.001") {
		t.Error("the second claim lost the volume")
	}
}

func TestClaimVolumeRefusesAVolumeThatIsNowhere(t *testing.T) {
	t.Parallel()

	area := workspace.New(t.TempDir(), nil)
	if err := area.ClaimVolume("gone", "gone", "Movie.7z.001"); err == nil {
		t.Fatal("a volume that is nowhere was claimed")
	}
}

func TestRemoveVolumesDeletesTheSetFolderOnly(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	area := workspace.New(root, nil)
	if err := os.MkdirAll(area.VolumesDir("g1"), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(area.VolumePath("g1", "a.7z.001"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	other := area.VolumesDir("g2")
	if err := os.MkdirAll(other, 0o750); err != nil {
		t.Fatal(err)
	}

	if err := area.RemoveVolumes("g1"); err != nil {
		t.Fatalf("RemoveVolumes: %v", err)
	}
	if _, err := os.Stat(area.VolumesDir("g1")); !os.IsNotExist(err) {
		t.Errorf("the set folder is still there: %v", err)
	}
	if _, err := os.Stat(other); err != nil {
		t.Errorf("another set's folder was removed: %v", err)
	}
	if err := area.RemoveVolumes("g1"); err != nil {
		t.Errorf("removing a folder that is not there: %v", err)
	}
}
