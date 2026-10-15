package domain

import "testing"

func TestCheckEditLayoutLetsVersionsCoexistAndRefusesPartsWithAWhole(t *testing.T) {
	t.Parallel()
	movie := func(file, folder string, part int) LayoutItem {
		return LayoutItem{File: file, Folder: folder, Kind: KindMovie, Place: Place{Part: part}}
	}
	series := func(file string, season, first, last int) LayoutItem {
		return LayoutItem{File: file, Folder: "TV Shows/Lizzie (2008)", Kind: KindSeries, Place: Place{Episode: true, Season: season, First: first, Last: last}}
	}
	tests := []struct {
		name  string
		items []LayoutItem
		want  map[string]string
	}{
		{"a whole movie and its versions", []LayoutItem{movie("a", "M", 0), movie("b", "M", 0)}, map[string]string{}},
		{"two parts with their own numbers", []LayoutItem{movie("p1", "M", 1), movie("p2", "M", 2)}, map[string]string{}},
		{"two parts with one number", []LayoutItem{movie("p1", "M", 2), movie("p2", "M", 2)}, map[string]string{"p2": CodeDuplicateTarget}},
		{"a part and a whole movie", []LayoutItem{movie("whole", "M", 0), movie("p1", "M", 1)}, map[string]string{"whole": CodePartsAndWhole, "p1": CodePartsAndWhole}},
		{"parts of other movies do not clash", []LayoutItem{movie("p1", "M", 1), movie("q1", "N", 1)}, map[string]string{}},
		{"chapters that do not overlap", []LayoutItem{series("c1", 1, 1, 1), series("c2", 1, 2, 2)}, map[string]string{}},
		{"chapters that overlap", []LayoutItem{series("c1", 1, 1, 1), series("c2", 1, 1, 2)}, map[string]string{"c2": CodeChaptersOverlap}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := CheckEditLayout(tt.items)
			if len(got) != len(tt.want) {
				t.Fatalf("CheckEditLayout = %v, want %v", got, tt.want)
			}
			for file, code := range tt.want {
				if got[file] != code {
					t.Errorf("file %s: code %q, want %q", file, got[file], code)
				}
			}
		})
	}
}

func TestSeasonEntryNameKeepsTheTitleWithoutItsTag(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		title  LibraryTitle
		season int
		want   string
	}{
		{"a season of a linked series", LibraryTitle{DiscTitle: "Breaking Bad", Year: 2008, TMDBID: 1396, IDTag: true}, 1, "Breaking Bad (2008) Season 01"},
		{"a season without a year", LibraryTitle{DiscTitle: "Lizzie"}, 0, "Lizzie Season 00"},
		{"a disc title with a colon", LibraryTitle{DiscTitle: "Mission - Impossible", Year: 1996}, 2, "Mission - Impossible (1996) Season 02"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := SeasonEntryName(tt.title, tt.season)
			if err != nil {
				t.Fatalf("SeasonEntryName: %v", err)
			}
			if got != tt.want {
				t.Errorf("SeasonEntryName = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestEntryNamesOfATitleAndOfAVideoWithSubtitles(t *testing.T) {
	t.Parallel()
	if got := TitleEntryName("Movies/Iron Man (2008) [tmdbid-1726]"); got != "Iron Man (2008) [tmdbid-1726]" {
		t.Errorf("TitleEntryName = %q", got)
	}
	if got := SubtitledEntryName("TV Shows/Lizzie (2008)/Season 01/Lizzie (2008) - S01E02 - Trial.mkv"); got != "Lizzie (2008) - S01E02 - Trial" {
		t.Errorf("SubtitledEntryName = %q", got)
	}
}
