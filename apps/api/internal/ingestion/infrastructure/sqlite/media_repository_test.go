package sqlite_test

import (
	"math"
	"reflect"
	"testing"
	"time"

	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/ingestion/domain"
	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/ingestion/infrastructure/sqlite"
)

// probed is what a probe of a 1080p video with two audio tracks and a forced subtitle reads.
var probed = domain.MediaInfo{
	Container: "matroska,webm",
	Duration:  1023 * time.Millisecond,
	Size:      8147,
	Video:     domain.VideoTrack{Codec: "h264", Width: 1920, Height: 1080, HDR: false},
	Audio: []domain.AudioTrack{
		{Codec: "aac", Channels: 2, Language: "eng", Title: "English", Default: true},
		{Codec: "ac3", Channels: 6, Language: "spa", Title: "Latino"},
	},
	Subtitles: []domain.SubtitleTrack{
		{Codec: "subrip", Language: "spa", Title: "Forzados", Forced: true},
	},
}

func TestMediaProbeRoundTripKeepsEveryTrack(t *testing.T) {
	t.Parallel()

	conn := open(t, t.TempDir())
	probes := sqlite.NewMediaProbeRepository(conn)
	ctx := t.Context()
	id := domain.FileIdentity{Device: 2049, Inode: 1234567, Size: 8147, ModTime: time.Unix(1_700_000_000, 987654321)}

	if err := probes.Store(ctx, id, probed); err != nil {
		t.Fatalf("Store: %v", err)
	}
	stored, got, found, err := probes.Find(ctx, id.Device, id.Inode)
	if err != nil || !found {
		t.Fatalf("Find = found %v, err %v; want the stored probe", found, err)
	}
	if !reflect.DeepEqual(got, probed) {
		t.Fatalf("Find info = %+v\nwant %+v", got, probed)
	}
	if !id.Matches(stored) {
		t.Fatalf("stored identity = %+v, want it to match %+v", stored, id)
	}
}

func TestMediaProbeIsFoundByDeviceAndInodeOnly(t *testing.T) {
	t.Parallel()

	conn := open(t, t.TempDir())
	probes := sqlite.NewMediaProbeRepository(conn)
	ctx := t.Context()

	_, _, found, err := probes.Find(ctx, 1, 1)
	if err != nil || found {
		t.Fatalf("Find of an empty cache = found %v, err %v; want nothing and no error", found, err)
	}
	if err := probes.Store(ctx, domain.FileIdentity{Device: 1, Inode: 1, Size: 10, ModTime: time.Unix(1, 0)}, probed); err != nil {
		t.Fatal(err)
	}
	if _, _, found, _ := probes.Find(ctx, 1, 2); found {
		t.Error("another inode found the probe of this one")
	}
	if _, _, found, _ := probes.Find(ctx, 2, 1); found {
		t.Error("another device found the probe of this one")
	}
}

func TestMediaProbeStoreReplacesTheProbeOfTheSameFile(t *testing.T) {
	t.Parallel()

	conn := open(t, t.TempDir())
	probes := sqlite.NewMediaProbeRepository(conn)
	ctx := t.Context()
	first := domain.FileIdentity{Device: 3, Inode: 9, Size: 100, ModTime: time.Unix(10, 0)}
	second := domain.FileIdentity{Device: 3, Inode: 9, Size: 200, ModTime: time.Unix(20, 0)}

	if err := probes.Store(ctx, first, probed); err != nil {
		t.Fatal(err)
	}
	if err := probes.Store(ctx, second, domain.MediaInfo{Container: "mov,mp4"}); err != nil {
		t.Fatal(err)
	}
	stored, got, found, err := probes.Find(ctx, 3, 9)
	if err != nil || !found {
		t.Fatalf("Find = found %v, err %v", found, err)
	}
	if !second.Matches(stored) || first.Matches(stored) {
		t.Fatalf("stored identity = %+v, want the second file's", stored)
	}
	if got.Container != "mov,mp4" || len(got.Audio) != 0 {
		t.Fatalf("Find info = %+v, want the replacing probe", got)
	}
}

func TestMediaProbeKeepsDeviceAndInodeNumbersAboveTheSignedRange(t *testing.T) {
	t.Parallel()

	conn := open(t, t.TempDir())
	probes := sqlite.NewMediaProbeRepository(conn)
	ctx := t.Context()
	id := domain.FileIdentity{Device: math.MaxUint64, Inode: 1 << 63, Size: 1, ModTime: time.Unix(5, 0)}

	if err := probes.Store(ctx, id, probed); err != nil {
		t.Fatal(err)
	}
	stored, _, found, err := probes.Find(ctx, id.Device, id.Inode)
	if err != nil || !found {
		t.Fatalf("Find = found %v, err %v", found, err)
	}
	if stored.Device != id.Device || stored.Inode != id.Inode {
		t.Fatalf("stored = %+v, want device and inode %+v", stored, id)
	}
}
