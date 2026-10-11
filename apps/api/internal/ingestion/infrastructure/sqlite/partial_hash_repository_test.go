package sqlite_test

import (
	"math"
	"testing"
	"time"

	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/ingestion/domain"
	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/ingestion/infrastructure/sqlite"
)

func TestPartialHashRoundTripKeepsTheIdentityAndTheHash(t *testing.T) {
	t.Parallel()

	conn := open(t, t.TempDir())
	hashes := sqlite.NewPartialHashRepository(conn)
	ctx := t.Context()
	id := domain.FileIdentity{Device: 2049, Inode: 1234567, Size: 13 << 20, ModTime: time.Unix(1_700_000_000, 987654321)}
	const hash = "ph1:0a1b2c"

	if _, _, found, err := hashes.Find(ctx, id.Device, id.Inode); err != nil || found {
		t.Fatalf("Find of an empty cache = found %v, err %v; want nothing and no error", found, err)
	}
	if err := hashes.Store(ctx, id, hash); err != nil {
		t.Fatalf("Store: %v", err)
	}
	stored, got, found, err := hashes.Find(ctx, id.Device, id.Inode)
	if err != nil || !found {
		t.Fatalf("Find = found %v, err %v; want the stored hash", found, err)
	}
	if got != hash {
		t.Fatalf("Find hash = %q, want %q", got, hash)
	}
	if !id.Matches(stored) {
		t.Fatalf("stored identity = %+v, want it to match %+v", stored, id)
	}
}

func TestPartialHashIsFoundByDeviceAndInodeOnly(t *testing.T) {
	t.Parallel()

	conn := open(t, t.TempDir())
	hashes := sqlite.NewPartialHashRepository(conn)
	ctx := t.Context()
	if err := hashes.Store(ctx, domain.FileIdentity{Device: 1, Inode: 1, Size: 10, ModTime: time.Unix(1, 0)}, "ph1:x"); err != nil {
		t.Fatal(err)
	}
	if _, _, found, _ := hashes.Find(ctx, 1, 2); found {
		t.Error("another inode found the hash of this one")
	}
	if _, _, found, _ := hashes.Find(ctx, 2, 1); found {
		t.Error("another device found the hash of this one")
	}
}

func TestPartialHashStoreReplacesTheHashOfTheSameFile(t *testing.T) {
	t.Parallel()

	conn := open(t, t.TempDir())
	hashes := sqlite.NewPartialHashRepository(conn)
	ctx := t.Context()
	first := domain.FileIdentity{Device: 3, Inode: 9, Size: 100, ModTime: time.Unix(10, 0)}
	second := domain.FileIdentity{Device: 3, Inode: 9, Size: 200, ModTime: time.Unix(20, 0)}

	if err := hashes.Store(ctx, first, "ph1:first"); err != nil {
		t.Fatal(err)
	}
	if err := hashes.Store(ctx, second, "ph1:second"); err != nil {
		t.Fatal(err)
	}
	stored, got, found, err := hashes.Find(ctx, 3, 9)
	if err != nil || !found {
		t.Fatalf("Find = found %v, err %v", found, err)
	}
	if !second.Matches(stored) || first.Matches(stored) || got != "ph1:second" {
		t.Fatalf("stored = %+v, %q; want the second file's identity and hash", stored, got)
	}
}

func TestPartialHashKeepsDeviceAndInodeNumbersAboveTheSignedRange(t *testing.T) {
	t.Parallel()

	conn := open(t, t.TempDir())
	hashes := sqlite.NewPartialHashRepository(conn)
	ctx := t.Context()
	id := domain.FileIdentity{Device: math.MaxUint64, Inode: 1 << 63, Size: 1, ModTime: time.Unix(5, 0)}

	if err := hashes.Store(ctx, id, "ph1:big"); err != nil {
		t.Fatal(err)
	}
	stored, _, found, err := hashes.Find(ctx, id.Device, id.Inode)
	if err != nil || !found {
		t.Fatalf("Find = found %v, err %v", found, err)
	}
	if stored.Device != id.Device || stored.Inode != id.Inode {
		t.Fatalf("stored = %+v, want device and inode %+v", stored, id)
	}
}
