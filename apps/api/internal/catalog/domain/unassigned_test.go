package domain_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/catalog/domain"
)

func TestFreeNameSuffixesATakenName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		given string
		loose bool
		taken []string
		want  string
	}{
		{name: "a free folder name is kept", given: "Movie", want: "Movie"},
		{name: "a taken folder name takes (2)", given: "Movie", taken: []string{"Movie"}, want: "Movie (2)"},
		{name: "the next free number is used", given: "Movie", taken: []string{"Movie", "Movie (2)"}, want: "Movie (3)"},
		{name: "a name taken in another letter case counts", given: "Movie", taken: []string{"movie"}, want: "Movie (2)"},
		{name: "a loose file keeps its extension after the suffix", given: "Movie.mkv", loose: true, taken: []string{"Movie.mkv"}, want: "Movie (2).mkv"},
		{name: "a loose file with a taken stem but free name is kept", given: "Movie.mkv", loose: true, taken: []string{"Movie.srt"}, want: "Movie.mkv"},
		{name: "a folder with a dot keeps the whole name", given: "Movie.2008", taken: []string{"Movie.2008"}, want: "Movie.2008 (2)"},
		{name: "a loose file without extension takes the suffix at the end", given: "Movie", loose: true, taken: []string{"Movie"}, want: "Movie (2)"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := domain.FreeName(tt.given, tt.loose, domain.NewNameSet(tt.taken...)); got != tt.want {
				t.Errorf("FreeName(%q) = %q, want %q", tt.given, got, tt.want)
			}
		})
	}
}

func TestFreeRelSuffixesOnlyTheFileName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		given string
		taken []string
		want  string
	}{
		{name: "a free place is kept", given: "Season 01/Pilot.mkv", want: "Season 01/Pilot.mkv"},
		{name: "a taken place takes (2) before the extension", given: "Season 01/Pilot.mkv", taken: []string{"Season 01/Pilot.mkv"}, want: "Season 01/Pilot (2).mkv"},
		{name: "a file at the root takes (2)", given: "a.mkv", taken: []string{"a.mkv"}, want: "a (2).mkv"},
		{name: "a taken place in another folder does not count", given: "Season 02/Pilot.mkv", taken: []string{"Season 01/Pilot.mkv"}, want: "Season 02/Pilot.mkv"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := domain.FreeRel(tt.given, domain.NewNameSet(tt.taken...)); got != tt.want {
				t.Errorf("FreeRel(%q) = %q, want %q", tt.given, got, tt.want)
			}
		})
	}
}

func TestFitNameKeepsEveryName255BytesOrLess(t *testing.T) {
	t.Parallel()

	// "é" takes two bytes: a cut in the middle of one would not be valid UTF-8.
	long := strings.Repeat("é", 200)
	tests := []struct {
		name  string
		given string
		loose bool
	}{
		{name: "a folder name longer than 255 bytes is cut", given: long + "xyz"},
		{name: "a loose file keeps its extension when its stem is cut", given: long + ".mkv", loose: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := domain.FitName(tt.given, tt.loose)
			if len(got) > domain.MaxNameBytes {
				t.Fatalf("FitName = %d bytes, want at most %d", len(got), domain.MaxNameBytes)
			}
			if !strings.HasPrefix(got, "é") {
				t.Errorf("FitName = %q, want the start of the name", got)
			}
			if tt.loose && !strings.HasSuffix(got, ".mkv") {
				t.Errorf("FitName = %q, want the extension kept", got)
			}
		})
	}
	if got := domain.FitName("Movie.mkv", true); got != "Movie.mkv" {
		t.Errorf("FitName of a short name = %q, want it unchanged", got)
	}
}

func TestEntryNameForKeepsTheExtensionOfALooseFile(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		raw   string
		loose bool
		want  string
	}{
		{name: "a folder name is sanitized", raw: "Mission: Impossible", want: "Mission - Impossible"},
		{name: "a loose file keeps its extension in the case it has", raw: "Movie: Film.MKV", loose: true, want: "Movie - Film.MKV"},
		{name: "a loose file with an empty stem gets the fallback", raw: "?.mkv", loose: true, want: "Sin nombre.mkv"},
		{name: "a name with a space before its dot is all stem", raw: "Movie. mkv", loose: true, want: "Movie. mkv"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := domain.EntryNameFor(tt.raw, tt.loose); got != tt.want {
				t.Errorf("EntryNameFor(%q, %v) = %q, want %q", tt.raw, tt.loose, got, tt.want)
			}
		})
	}
}

func TestNewUnassignedEntryAndFileRefuseWhatTheSectionCannotHold(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, time.October, 11, 9, 0, 0, 0, time.UTC)
	tests := []struct {
		name string
		make func() error
	}{
		{name: "an empty source", make: func() error {
			_, err := domain.NewUnassignedEntry("", "Movie", false, "b1", domain.RelationNone, now)
			return err
		}},
		{name: "a name with a separator", make: func() error {
			_, err := domain.NewUnassignedEntry("s", "a/b", false, "b1", domain.RelationNone, now)
			return err
		}},
		{name: "a name of dots", make: func() error {
			_, err := domain.NewUnassignedEntry("s", "..", false, "b1", domain.RelationNone, now)
			return err
		}},
		{name: "a name longer than 255 bytes", make: func() error {
			_, err := domain.NewUnassignedEntry("s", strings.Repeat("a", 256), false, "b1", domain.RelationNone, now)
			return err
		}},
		{name: "an unknown relation", make: func() error {
			_, err := domain.NewUnassignedEntry("s", "Movie", false, "b1", "maybe", now)
			return err
		}},
		{name: "a loose file with a place", make: func() error {
			_, err := domain.NewUnassignedFile("src", "a.mkv", 1, true)
			return err
		}},
		{name: "a folder file with an absolute place", make: func() error {
			_, err := domain.NewUnassignedFile("src", "/etc/a.mkv", 1, false)
			return err
		}},
		{name: "a folder file that climbs out", make: func() error {
			_, err := domain.NewUnassignedFile("src", "x/../a.mkv", 1, false)
			return err
		}},
		{name: "a negative size", make: func() error {
			_, err := domain.NewUnassignedFile("src", "a.mkv", -1, false)
			return err
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if err := tt.make(); !errors.Is(err, domain.ErrInvalidEntry) {
				t.Errorf("error = %v, want ErrInvalidEntry", err)
			}
		})
	}
}
