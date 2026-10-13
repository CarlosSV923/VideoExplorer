package domain

import (
	"errors"
	"slices"
	"testing"
)

// The movie and series of the duplicate tests.
const (
	movieFolder  = "Movies/Iron Man (2008) [tmdbid-1726]"
	seriesFolder = "TV Shows/Breaking Bad (2008) [tmdbid-1396]"
)

var (
	wholeMovie = Target{Kind: KindMovie}
	partsMovie = Target{Kind: KindMovie, Parts: true}
	chapter2   = Target{Kind: KindSeries, Place: Place{Episode: true, Season: 1, First: 2, Last: 2}}
	double12   = Target{Kind: KindSeries, Place: Place{Episode: true, Season: 1, First: 1, Last: 2}}
	special1   = Target{Kind: KindSeries, Place: Place{Episode: true, Season: 0, First: 1, Last: 1}}
)

func wholeAt(rel string, size int64, hash string) Existing {
	return Existing{Rel: movieFolder + "/" + rel, Size: size, PartialHash: hash}
}

func chapterAt(season, first, last int, rel string) Existing {
	return Existing{
		Rel:   seriesFolder + "/Season 01/" + rel,
		Place: Place{Episode: true, Season: season, First: first, Last: last},
		Size:  10,
	}
}

func TestFindDuplicatesOfAMovie(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		target        Target
		existing      []Existing
		incoming      Fingerprint
		wantFiles     int
		wantExclusive bool
		wantExact     bool
	}{
		{
			name:      "an empty folder has no duplicate",
			target:    wholeMovie,
			wantFiles: 0,
		},
		{
			name:      "a registered whole movie is a duplicate, and a version may be kept",
			target:    wholeMovie,
			existing:  []Existing{wholeAt("Iron Man (2008) [tmdbid-1726].mkv", 100, "")},
			wantFiles: 1,
		},
		{
			name:      "a video the registry does not know, found on the disk, is a duplicate too",
			target:    wholeMovie,
			existing:  []Existing{wholeAt("Iron Man (2008) [tmdbid-1726] - 2160p.mkv", 100, "")},
			wantFiles: 1,
		},
		{
			name:      "a trashed file is not a duplicate",
			target:    wholeMovie,
			existing:  []Existing{{Rel: movieFolder + "/Iron Man (2008).mkv", Trashed: true}},
			wantFiles: 0,
		},
		{
			name:          "a whole movie against a part is exclusive",
			target:        wholeMovie,
			existing:      []Existing{{Rel: movieFolder + "/Iron Man (2008) - part1.mkv", Place: Place{Part: 1}}},
			wantFiles:     1,
			wantExclusive: true,
		},
		{
			name:          "parts against the whole movie are exclusive",
			target:        partsMovie,
			existing:      []Existing{wholeAt("Iron Man (2008).mkv", 100, "")},
			wantFiles:     1,
			wantExclusive: true,
		},
		{
			name:          "parts against parts are exclusive: the duplicate is the movie",
			target:        partsMovie,
			existing:      []Existing{{Rel: movieFolder + "/Iron Man (2008) - part1.mkv", Place: Place{Part: 1}}},
			wantFiles:     1,
			wantExclusive: true,
		},
		{
			name:      "parts into an empty movie are not a duplicate",
			target:    partsMovie,
			existing:  nil,
			wantFiles: 0,
		},
		{
			name:      "an exact copy: same size and partial hash",
			target:    wholeMovie,
			existing:  []Existing{wholeAt("Iron Man (2008).mkv", 100, "ph1:aa")},
			incoming:  Fingerprint{Size: 100, PartialHash: "ph1:aa"},
			wantFiles: 1,
			wantExact: true,
		},
		{
			name:      "the same size with another partial hash is not an exact copy",
			target:    wholeMovie,
			existing:  []Existing{wholeAt("Iron Man (2008).mkv", 100, "ph1:bb")},
			incoming:  Fingerprint{Size: 100, PartialHash: "ph1:aa"},
			wantFiles: 1,
		},
		{
			name:      "the same partial hash with another size is not an exact copy",
			target:    wholeMovie,
			existing:  []Existing{wholeAt("Iron Man (2008).mkv", 101, "ph1:aa")},
			incoming:  Fingerprint{Size: 100, PartialHash: "ph1:aa"},
			wantFiles: 1,
		},
		{
			name:      "a file without a partial hash is never an exact copy",
			target:    wholeMovie,
			existing:  []Existing{wholeAt("Iron Man (2008).mkv", 100, "")},
			incoming:  Fingerprint{Size: 100},
			wantFiles: 1,
		},
		{
			name:   "a file listed by the registry and by the disk counts once, the registry's listing first",
			target: wholeMovie,
			existing: []Existing{
				wholeAt("Iron Man (2008).mkv", 100, "ph1:aa"),
				wholeAt("iron man (2008).MKV", 100, ""),
			},
			wantFiles: 1,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := FindDuplicates(tt.target, tt.existing, tt.incoming)
			if err != nil {
				t.Fatalf("FindDuplicates: %v", err)
			}
			if len(got.Files) != tt.wantFiles {
				t.Errorf("files = %d, want %d: %+v", len(got.Files), tt.wantFiles, got.Files)
			}
			if got.Exclusive != tt.wantExclusive {
				t.Errorf("exclusive = %v, want %v", got.Exclusive, tt.wantExclusive)
			}
			if got.ExactCopy != tt.wantExact {
				t.Errorf("exact copy = %v, want %v", got.ExactCopy, tt.wantExact)
			}
		})
	}
}

func TestFindDuplicatesOfChapters(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		target        Target
		existing      []Existing
		wantFiles     []string
		wantExclusive bool
	}{
		{
			name:      "the same chapter is a duplicate, and a version may be kept",
			target:    chapter2,
			existing:  []Existing{chapterAt(1, 2, 2, "Breaking Bad (2008) - S01E02.mkv")},
			wantFiles: []string{"Breaking Bad (2008) - S01E02.mkv"},
		},
		{
			name:      "another chapter of the season is not a duplicate",
			target:    chapter2,
			existing:  []Existing{chapterAt(1, 1, 1, "Breaking Bad (2008) - S01E01.mkv")},
			wantFiles: nil,
		},
		{
			name:      "the same number in another season is not a duplicate",
			target:    chapter2,
			existing:  []Existing{chapterAt(2, 2, 2, "Breaking Bad (2008) - S02E02.mkv")},
			wantFiles: nil,
		},
		{
			name:      "a special is not a duplicate of the first chapter of season 1",
			target:    special1,
			existing:  []Existing{chapterAt(1, 1, 1, "Breaking Bad (2008) - S01E01.mkv")},
			wantFiles: nil,
		},
		{
			name:      "a file of two chapters overlaps the chapter it holds",
			target:    chapter2,
			existing:  []Existing{chapterAt(1, 1, 2, "Breaking Bad (2008) - S01E01-E02.mkv")},
			wantFiles: []string{"Breaking Bad (2008) - S01E01-E02.mkv"},
		},
		{
			name:   "two chapters overlap every chapter they span, once each",
			target: double12,
			existing: []Existing{
				chapterAt(1, 1, 2, "Breaking Bad (2008) - S01E01-E02.mkv"),
				chapterAt(1, 2, 2, "Breaking Bad (2008) - S01E02.mkv"),
				chapterAt(1, 3, 3, "Breaking Bad (2008) - S01E03.mkv"),
			},
			wantFiles: []string{"Breaking Bad (2008) - S01E01-E02.mkv", "Breaking Bad (2008) - S01E02.mkv"},
		},
		{
			name:      "a trashed chapter is not a duplicate",
			target:    chapter2,
			existing:  []Existing{{Rel: "TV Shows/x", Place: Place{Episode: true, Season: 1, First: 2, Last: 2}, Trashed: true}},
			wantFiles: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := FindDuplicates(tt.target, tt.existing, Fingerprint{})
			if err != nil {
				t.Fatalf("FindDuplicates: %v", err)
			}
			var rels []string
			for _, f := range got.Files {
				rels = append(rels, f.Rel[len(seriesFolder)+len("/Season 01/"):])
			}
			if !slices.Equal(rels, tt.wantFiles) {
				t.Errorf("files = %v, want %v", rels, tt.wantFiles)
			}
			if got.Exclusive != tt.wantExclusive {
				t.Errorf("exclusive = %v, want %v", got.Exclusive, tt.wantExclusive)
			}
		})
	}
}

func TestFindDuplicatesRefusesAnImpossibleTarget(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		target Target
	}{
		{name: "a movie with a place of its own", target: Target{Kind: KindMovie, Place: Place{Episode: true, Season: 1, First: 1, Last: 1}}},
		{name: "a series with parts", target: Target{Kind: KindSeries, Parts: true, Place: chapter2.Place}},
		{name: "a series with no chapter", target: Target{Kind: KindSeries}},
		{name: "a series whose chapters end before they start", target: Target{Kind: KindSeries, Place: Place{Episode: true, Season: 1, First: 3, Last: 2}}},
		{name: "a kind that is not known", target: Target{Kind: "book"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if _, err := FindDuplicates(tt.target, nil, Fingerprint{}); !errors.Is(err, ErrInvalidLibrary) {
				t.Errorf("FindDuplicates error = %v, want ErrInvalidLibrary", err)
			}
		})
	}
}

func TestPlaceOfNameReadsTheDiskNames(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		kind   TitleKind
		rel    string
		want   Place
		wantOK bool
	}{
		{name: "the whole movie", kind: KindMovie, rel: "Movies/Iron Man (2008)/Iron Man (2008).mkv", want: Place{}, wantOK: true},
		{name: "a version of the whole movie", kind: KindMovie, rel: "Movies/Iron Man (2008)/Iron Man (2008) - 2160p.mkv", want: Place{}, wantOK: true},
		{name: "a part", kind: KindMovie, rel: "Movies/Iron Man (2008)/Iron Man (2008) - part2.mkv", want: Place{Part: 2}, wantOK: true},
		{name: "a chapter", kind: KindSeries, rel: "TV Shows/A/Season 01/A (2008) - S01E02 - Cat.mkv", want: Place{Episode: true, Season: 1, First: 2, Last: 2}, wantOK: true},
		{name: "two chapters", kind: KindSeries, rel: "TV Shows/A/Season 01/A (2008) - S01E01-E02 - Pilot + Cat.mkv", want: Place{Episode: true, Season: 1, First: 1, Last: 2}, wantOK: true},
		{name: "a special", kind: KindSeries, rel: "TV Shows/A/Season 00/A (2008) - S00E03.mkv", want: Place{Episode: true, Season: 0, First: 3, Last: 3}, wantOK: true},
		{name: "a series file without a chapter code", kind: KindSeries, rel: "TV Shows/A/Season 01/notes.mkv", wantOK: false},
		{name: "a chapter code that ends before it starts", kind: KindSeries, rel: "TV Shows/A/Season 01/A - S01E03-E02.mkv", wantOK: false},
		{name: "an unknown kind", kind: "book", rel: "Books/x.mkv", wantOK: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, ok := PlaceOfName(tt.kind, tt.rel)
			if ok != tt.wantOK || got != tt.want {
				t.Errorf("PlaceOfName = %+v, %v; want %+v, %v", got, ok, tt.want, tt.wantOK)
			}
		})
	}
}

func TestSameContentNeedsTheSizeAndAHash(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		a, b Fingerprint
		want bool
	}{
		{name: "the same size and hash", a: Fingerprint{Size: 1, PartialHash: "ph1:x"}, b: Fingerprint{Size: 1, PartialHash: "ph1:x"}, want: true},
		{name: "another size", a: Fingerprint{Size: 1, PartialHash: "ph1:x"}, b: Fingerprint{Size: 2, PartialHash: "ph1:x"}},
		{name: "another hash", a: Fingerprint{Size: 1, PartialHash: "ph1:x"}, b: Fingerprint{Size: 1, PartialHash: "ph1:y"}},
		{name: "no hash on either side", a: Fingerprint{Size: 1}, b: Fingerprint{Size: 1}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := SameContent(tt.a, tt.b); got != tt.want {
				t.Errorf("SameContent = %v, want %v", got, tt.want)
			}
		})
	}
}
