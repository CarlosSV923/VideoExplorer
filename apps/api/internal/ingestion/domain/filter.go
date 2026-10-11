package domain

import (
	"path"
	"slices"
	"strings"
	"unicode"
)

// FileKind says what filtering does with a file of the content it receives (RF-07).
type FileKind string

// The kinds a file can get.
const (
	// KindVideo: a video, kept for the next steps (VIDEO_EXTENSIONS).
	KindVideo FileKind = "video"
	// KindSubtitle: an external subtitle, kept and associated later (SUBTITLE_EXTENSIONS).
	KindSubtitle FileKind = "subtitle"
	// KindDiscard: sent to the trash, not deleted (samples, macOS metadata and everything else).
	KindDiscard FileKind = "discard"
)

// Kept reports whether the kind stays for the next steps.
func (k FileKind) Kept() bool {
	return k == KindVideo || k == KindSubtitle
}

// SampleMaxSize is the size under which a file is a sample when its path says so (RF-07):
// 150 000 000 bytes, as the spec writes it.
const SampleMaxSize int64 = 150 * 1000 * 1000

// sampleWords are the words that mark a sample (RF-07): "sample" and its plural "samples".
// They are whole words of the path, in any letter case, and a word is what lies between
// characters that are not letters or digits: "Movie-sample.mkv", "Sample/clip.mkv",
// "movie.sample.mkv" and "Samples/clip.mkv" match, while "Resampled.mkv" does not.
var sampleWords = []string{"sample", "samples"}

// macOSFolder is the folder macOS adds to an archive with the metadata of its files (RF-07).
const macOSFolder = "__MACOSX"

// appleDoublePrefix starts the metadata files macOS writes next to each file (RF-07).
const appleDoublePrefix = "._"

// FilterRules are the extensions kept as video and as external subtitle (RF-07).
// The lists are compared in lower case, with the dot.
type FilterRules struct {
	video    []string
	subtitle []string
}

// NewFilterRules builds the rules from the configured extension lists.
func NewFilterRules(video, subtitle []string) FilterRules {
	return FilterRules{video: lowerAll(video), subtitle: lowerAll(subtitle)}
}

// Candidate is a file of a work unit: Path is its place inside the unit, with "/"
// separators and its folders (a folder dropped in the browser, or the folders of an
// extracted archive), and Size is its number of bytes.
type Candidate struct {
	Path string
	Size int64
}

// Classify says what happens to a file (RF-07). The macOS metadata (a file named "._*"
// or anything under a "__MACOSX" folder) is discarded first, whatever it is. Then a
// sample (under SampleMaxSize, with "sample" as a word in its path) is discarded, even
// when it is a video or a subtitle. Then the extension decides: a video or a subtitle is
// kept, anything else is discarded (.nfo, .txt, images, .url, .exe, files without an
// extension…). Extensions are compared in any letter case.
func (r FilterRules) Classify(c Candidate) FileKind {
	if isMacOSMetadata(c.Path) || (c.Size < SampleMaxSize && isSample(c.Path)) {
		return KindDiscard
	}
	ext := strings.ToLower(path.Ext(c.Path))
	switch {
	case ext == "":
		return KindDiscard
	case slices.Contains(r.video, ext):
		return KindVideo
	case slices.Contains(r.subtitle, ext):
		return KindSubtitle
	default:
		return KindDiscard
	}
}

// isSample reports whether a sample word is in the path: in the name or in any folder.
func isSample(p string) bool {
	for _, w := range words(p) {
		if slices.Contains(sampleWords, w) {
			return true
		}
	}
	return false
}

// words splits a path into its words, in lower case: the runs of letters and digits.
func words(p string) []string {
	return strings.FieldsFunc(strings.ToLower(p), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
}

// isMacOSMetadata reports whether the path is macOS metadata: a file whose name starts
// with "._", or anything inside a "__MACOSX" folder at any level.
func isMacOSMetadata(p string) bool {
	parts := strings.Split(p, "/")
	name := parts[len(parts)-1]
	if strings.HasPrefix(name, appleDoublePrefix) {
		return true
	}
	return slices.Contains(parts[:len(parts)-1], macOSFolder)
}

func lowerAll(items []string) []string {
	out := make([]string, 0, len(items))
	for _, item := range items {
		out = append(out, strings.ToLower(item))
	}
	return out
}

// FilterEntry is one file that filtering found in a work unit, with what it decided
// for it. Trashed is set once a discarded file went to the trash.
type FilterEntry struct {
	Path    string
	Size    int64
	Kind    FileKind
	Trashed bool
}
