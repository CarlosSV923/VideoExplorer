package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"hash"
	"slices"
	"strconv"
)

// PartialHashChunk is the size of each range a partial hash reads (RF-24): 4 MiB.
const PartialHashChunk int64 = 4 << 20

// PartialHashVersion names the algorithm of a partial hash. Every hash starts with it, so
// that changing the ranges or the digest later never compares two different results (RF-24).
const PartialHashVersion = "ph1"

// ByteRange is a run of bytes of a file: [Offset, Offset+Length).
type ByteRange struct {
	Offset, Length int64
}

// PartialHashRanges returns the ranges a partial hash reads of a file of size bytes, in file
// order and without overlaps (RF-24): the first PartialHashChunk bytes, a chunk centred on the
// middle of the file, and the last chunk.
//
// Ranges that overlap or touch are merged, so every byte is read once: a file of 12 MiB or
// less is one range (the whole file), and the three ranges of a bigger file never overlap.
// A file of 12 MiB + 1 byte keeps the byte at offset 8 MiB out of its ranges (the middle
// chunk ends there and the last one starts one byte later). An empty file has no ranges.
func PartialHashRanges(size int64) []ByteRange {
	if size <= 0 {
		return nil
	}
	candidates := []ByteRange{
		clipRange(0, size),
		clipRange(size/2-PartialHashChunk/2, size),
		clipRange(size-PartialHashChunk, size),
	}
	slices.SortFunc(candidates, func(a, b ByteRange) int {
		switch {
		case a.Offset < b.Offset:
			return -1
		case a.Offset > b.Offset:
			return 1
		default:
			return 0
		}
	})

	merged := make([]ByteRange, 0, len(candidates))
	for _, r := range candidates {
		if r.Length <= 0 {
			continue
		}
		if n := len(merged); n > 0 && r.Offset <= merged[n-1].Offset+merged[n-1].Length {
			end := max(merged[n-1].Offset+merged[n-1].Length, r.Offset+r.Length)
			merged[n-1].Length = end - merged[n-1].Offset
			continue
		}
		merged = append(merged, r)
	}
	return merged
}

// clipRange is the PartialHashChunk bytes that start at start, kept inside a file of size bytes.
func clipRange(start, size int64) ByteRange {
	from := max(start, 0)
	to := min(start+PartialHashChunk, size)
	return ByteRange{Offset: from, Length: to - from}
}

// NewPartialDigest starts the digest of a partial hash of a file of size bytes. The size is
// written first, in decimal and followed by a newline, and then the bytes of the ranges of
// PartialHashRanges, in order. The caller writes those bytes and calls FormatPartialHash on the sum.
func NewPartialDigest(size int64) hash.Hash {
	h := sha256.New()
	_, _ = h.Write([]byte(strconv.FormatInt(size, 10) + "\n"))
	return h
}

// FormatPartialHash turns the sum of a NewPartialDigest into a partial hash:
// PartialHashVersion, a colon and the hex of the SHA-256 (RF-24).
func FormatPartialHash(sum []byte) string {
	return PartialHashVersion + ":" + hex.EncodeToString(sum)
}
