package domain

import (
	"path"
	"slices"
	"strings"
)

// FileKind says what filtering does with a file of the content it receives (RF-07).
type FileKind string

// The kinds a file can get.
const (
	// KindVideo: a video, kept for the next steps (VIDEO_EXTENSIONS).
	KindVideo FileKind = "video"
	// KindSubtitle: an external subtitle, kept and associated later (SUBTITLE_EXTENSIONS).
	KindSubtitle FileKind = "subtitle"
	// KindDiscard: sent to the trash, not deleted (samples and everything else).
	KindDiscard FileKind = "discard"
)

// Kept reports whether the kind stays for the next steps.
func (k FileKind) Kept() bool {
	return k == KindVideo || k == KindSubtitle
}

// SampleMaxSize is the size under which a file is a sample when its path says so (RF-07).
// The spec writes "150 MB", and it writes binary units (MiB, GiB) when it means them,
// so this is 150 × 10⁶ bytes.
const SampleMaxSize int64 = 150 * 1000 * 1000

// sampleWord is the text that marks a sample. It is matched in any letter case and
// anywhere in the path, so "Movie-SAMPLE.mkv", "Samples/clip.mkv" and "Resampled.mkv"
// all match: the spec says «sample» in the name or the folder, not as a whole word.
const sampleWord = "sample"

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

// Classify says what happens to a file (RF-07). A sample (under SampleMaxSize, with
// "sample" in its path) is discarded first, even when it is a video or a subtitle.
// Then the extension decides: a video or a subtitle is kept, anything else is discarded
// (.nfo, .txt, images, .url, .exe, files without an extension…). Extensions are
// compared in any letter case.
func (r FilterRules) Classify(c Candidate) FileKind {
	if c.Size < SampleMaxSize && strings.Contains(strings.ToLower(c.Path), sampleWord) {
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

func lowerAll(items []string) []string {
	out := make([]string, 0, len(items))
	for _, item := range items {
		out = append(out, strings.ToLower(item))
	}
	return out
}
