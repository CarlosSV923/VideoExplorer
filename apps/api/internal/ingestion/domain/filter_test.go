package domain_test

import (
	"testing"

	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/ingestion/domain"
)

// defaultRules are the defaults of VIDEO_EXTENSIONS and SUBTITLE_EXTENSIONS (spec §7).
func defaultRules() domain.FilterRules {
	return domain.NewFilterRules(
		[]string{".mkv", ".mp4", ".avi", ".m4v", ".ts", ".m2ts", ".mov", ".wmv", ".webm", ".mpg", ".mpeg"},
		[]string{".srt", ".ass", ".ssa", ".sub", ".idx", ".vtt"},
	)
}

const big = domain.SampleMaxSize

func TestClassifyKeepsVideosAndSubtitlesByExtensionInAnyCase(t *testing.T) {
	t.Parallel()

	tests := []struct {
		path string
		want domain.FileKind
	}{
		{path: "Movie.mkv", want: domain.KindVideo},
		{path: "Movie.MKV", want: domain.KindVideo},
		{path: "Movie.M2TS", want: domain.KindVideo},
		{path: "Folder/Movie.Mpeg", want: domain.KindVideo},
		{path: "Movie.es.SRT", want: domain.KindSubtitle},
		{path: "Movie.en.forced.ass", want: domain.KindSubtitle},
		{path: "Movie.idx", want: domain.KindSubtitle},
		{path: "Movie.nfo", want: domain.KindDiscard},
		{path: "Movie.txt", want: domain.KindDiscard},
		{path: "poster.jpg", want: domain.KindDiscard},
		{path: "Movie.url", want: domain.KindDiscard},
		{path: "setup.exe", want: domain.KindDiscard},
		{path: "Movie", want: domain.KindDiscard},
		{path: "Movie.mkv.txt", want: domain.KindDiscard},
		{path: "Movie.mkvx", want: domain.KindDiscard},
	}
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			t.Parallel()

			got := defaultRules().Classify(domain.Candidate{Path: tt.path, Size: big})
			if got != tt.want {
				t.Errorf("Classify(%q) = %s, want %s", tt.path, got, tt.want)
			}
		})
	}
}

func TestClassifyCompareExtensionsInAnyCaseWhateverTheConfiguredCase(t *testing.T) {
	t.Parallel()

	rules := domain.NewFilterRules([]string{".MKV"}, []string{".Srt"})
	if got := rules.Classify(domain.Candidate{Path: "a.mkv", Size: big}); got != domain.KindVideo {
		t.Errorf("a.mkv = %s, want video", got)
	}
	if got := rules.Classify(domain.Candidate{Path: "a.SRT", Size: big}); got != domain.KindSubtitle {
		t.Errorf("a.SRT = %s, want subtitle", got)
	}
}

func TestClassifyDiscardsSamplesByNameOrFolderInAnyCase(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		path string
		size int64
		want domain.FileKind
	}{
		{name: "sample name", path: "sample.mkv", size: 1 << 20, want: domain.KindDiscard},
		{name: "sample as a suffix", path: "Movie-sample.mkv", size: 1 << 20, want: domain.KindDiscard},
		{name: "upper case", path: "Movie.SAMPLE.mkv", size: 1 << 20, want: domain.KindDiscard},
		{name: "sample folder", path: "Samples/clip.mkv", size: 1 << 20, want: domain.KindDiscard},
		{name: "folder of a dropped folder", path: "Movie.Sample/Movie.mkv", size: 1 << 20, want: domain.KindDiscard},
		{name: "deep folder", path: "Movie/extras/sample/x.mkv", size: 1 << 20, want: domain.KindDiscard},
		{name: "sample as part of a word", path: "Resampled.mkv", size: 1 << 20, want: domain.KindDiscard},
		{name: "sample subtitle", path: "Movie.sample.srt", size: 10, want: domain.KindDiscard},
		{name: "zero-byte sample", path: "sample.mkv", size: 0, want: domain.KindDiscard},
		{name: "not a sample word", path: "Example.mkv", size: 1 << 20, want: domain.KindVideo},
		{name: "a sample name but too big", path: "sample.mkv", size: big, want: domain.KindVideo},
		{name: "a sample folder but too big", path: "Samples/Movie.mkv", size: big + 1, want: domain.KindVideo},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := defaultRules().Classify(domain.Candidate{Path: tt.path, Size: tt.size})
			if got != tt.want {
				t.Errorf("Classify(%q, %d bytes) = %s, want %s", tt.path, tt.size, got, tt.want)
			}
		})
	}
}

func TestClassifySampleLimitIsExactly150MB(t *testing.T) {
	t.Parallel()

	tests := []struct {
		size int64
		want domain.FileKind
	}{
		{size: 0, want: domain.KindDiscard},
		{size: big - 1, want: domain.KindDiscard},
		{size: big, want: domain.KindVideo},
		{size: big + 1, want: domain.KindVideo},
	}
	for _, tt := range tests {
		got := defaultRules().Classify(domain.Candidate{Path: "sample.mkv", Size: tt.size})
		if got != tt.want {
			t.Errorf("sample.mkv of %d bytes = %s, want %s", tt.size, got, tt.want)
		}
	}
}

func TestSampleLimitIsDecimal150MegabytesAndNotMiB(t *testing.T) {
	t.Parallel()

	if domain.SampleMaxSize != 150_000_000 {
		t.Fatalf("SampleMaxSize = %d, want 150 MB = 150000000 bytes (spec §3, RF-07)", domain.SampleMaxSize)
	}
}

func TestFileKindKeptOnlyForVideosAndSubtitles(t *testing.T) {
	t.Parallel()

	if !domain.KindVideo.Kept() || !domain.KindSubtitle.Kept() {
		t.Error("videos and subtitles must be kept")
	}
	if domain.KindDiscard.Kept() {
		t.Error("a discard must not be kept")
	}
}
