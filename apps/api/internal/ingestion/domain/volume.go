package domain

import (
	"regexp"
	"strconv"
	"strings"
)

// Volume describes one part of a multi-volume archive, recognized by its name
// (RF-03). Gathering the parts of a set is not done yet: a job whose file is a
// volume is not extracted on its own.
type Volume struct {
	// Set identifies the archive the part belongs to (lower-case base name plus
	// naming style), so the parts of one archive find each other.
	Set string
	// Index is 1 for the first volume.
	Index int
}

var (
	// movie.part1.rar, movie.part01.rar (RAR 3+ naming).
	partStyle = regexp.MustCompile(`(?i)^(.+)\.part(\d{1,4})\.rar$`)
	// movie.7z.001, movie.zip.002 (split files).
	numberedStyle = regexp.MustCompile(`(?i)^(.+\.[a-z0-9]{1,4})\.(\d{3})$`)
)

// legacyVolume matches the old RAR naming (movie.r00, movie.r01…), whose first
// volume is a plain .rar that cannot be told from a single archive. It is not
// supported (RF-03): its job fails with legacy_volume.
var legacyVolume = regexp.MustCompile(`(?i)\.r\d{2}$`)

// IsLegacyVolume reports whether the name is an old-style RAR volume.
func IsLegacyVolume(fileName string) bool {
	return legacyVolume.MatchString(fileName)
}

// plainRAR matches a plain RAR name (movie.rar): the first volume of an
// old-style set when a movie.r00 of the same batch exists (RF-03).
var plainRAR = regexp.MustCompile(`(?i)^(.+)\.rar$`)

// legacyVolumeBase matches an old-style RAR volume and captures its base name.
var legacyVolumeBase = regexp.MustCompile(`(?i)^(.+)\.r\d{2}$`)

// LegacyVolumeBase returns the lower-case base name of an old-style RAR volume
// (movie.r00 gives movie), and whether the name is one.
func LegacyVolumeBase(fileName string) (string, bool) {
	m := legacyVolumeBase.FindStringSubmatch(fileName)
	if m == nil {
		return "", false
	}
	return strings.ToLower(m[1]), true
}

// PlainRARBase returns the lower-case base name of a plain .rar (movie.rar gives
// movie), and whether the name is one.
func PlainRARBase(fileName string) (string, bool) {
	m := plainRAR.FindStringSubmatch(fileName)
	if m == nil {
		return "", false
	}
	return strings.ToLower(m[1]), true
}

// ParseVolume recognizes multi-volume archive parts by name.
func ParseVolume(fileName string) (Volume, bool) {
	if m := partStyle.FindStringSubmatch(fileName); m != nil {
		n, _ := strconv.Atoi(m[2])
		return Volume{Set: strings.ToLower(m[1]) + "|rar-part", Index: n}, n > 0
	}
	if m := numberedStyle.FindStringSubmatch(fileName); m != nil {
		n, _ := strconv.Atoi(m[2])
		return Volume{Set: strings.ToLower(m[1]) + "|numbered", Index: n}, n > 0
	}
	return Volume{}, false
}
