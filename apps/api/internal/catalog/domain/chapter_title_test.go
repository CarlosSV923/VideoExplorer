package domain

import "testing"

func TestChapterTitleFromNameReadsTheTitleOfTheFileAsSpecSays(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		rel     string
		season  int
		first   int
		last    int
		version string
		want    string
	}{
		{"plain title", "TV Shows/Lizzie (1975)/Season 01/Lizzie (1975) - S01E02 - The Test.mkv", 1, 2, 2, "", "The Test"},
		{"title and version", "TV Shows/Lizzie (1975)/Season 01/Lizzie (1975) - S01E02 - The Test - 1080p.mkv", 1, 2, 2, "1080p", "The Test"},
		{"no title, a version", "TV Shows/Lizzie (1975)/Season 01/Lizzie (1975) - S01E02 - 1080p.mkv", 1, 2, 2, "1080p", ""},
		{"no title at all", "TV Shows/Lizzie (1975)/Season 01/Lizzie (1975) - S01E02.mkv", 1, 2, 2, "", ""},
		{"several chapters", "TV Shows/Lizzie (1975)/Season 00/Lizzie (1975) - S00E01-E02 - Pilot + Two.mkv", 0, 1, 2, "", "Pilot + Two"},
		{"title with dots and dashes", "TV Shows/Lizzie (1975)/Season 01/Lizzie (1975) - S01E03 - Mr. Smith - Part 2.mkv", 1, 3, 3, "", "Mr. Smith - Part 2"},
		{"a name without the code", "TV Shows/Lizzie (1975)/Season 01/Pilot.mkv", 1, 1, 1, "", ""},
		{"a code of other chapters", "TV Shows/Lizzie (1975)/Season 01/Lizzie (1975) - S01E02 - The Test.mkv", 1, 3, 3, "", ""},
		{"text glued to the code", "TV Shows/Lizzie (1975)/Season 01/Lizzie (1975) - S01E02Test.mkv", 1, 2, 2, "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := ChapterTitleFromName(tt.rel, tt.season, tt.first, tt.last, tt.version); got != tt.want {
				t.Errorf("ChapterTitleFromName(%q) = %q, want %q", tt.rel, got, tt.want)
			}
		})
	}
}
