package domain_test

import (
	"errors"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/catalog/domain"
	metadata "github.com/CarlosSV923/VideoExplorer/apps/api/internal/metadata/domain"
)

type pathVectors struct {
	Paths struct {
		Cases []struct {
			Name     string      `json:"name"`
			Input    pathInput   `json:"input"`
			Expected expectation `json:"expected"`
		} `json:"cases"`
	} `json:"paths"`
}

// pathInput is what a caller has for a case: TMDB's titles and the file's data, not the names.
type pathInput struct {
	Kind  string `json:"kind"`
	Roots *struct {
		Movies string `json:"movies"`
		Series string `json:"series"`
	} `json:"roots"`
	Original   string         `json:"original"`
	English    string         `json:"english"`
	Year       int            `json:"year"`
	TMDBID     int64          `json:"tmdbId"`
	IDTag      *bool          `json:"idTag"`
	Extension  string         `json:"extension"`
	Version    string         `json:"version"`
	Part       int            `json:"part"`
	Season     int            `json:"season"`
	Episode    int            `json:"episode"`
	EpisodeEnd *int           `json:"episodeEnd"`
	Chapters   []chapterInput `json:"chapters"`
	Video      *pathInput     `json:"video"`
	Language   string         `json:"language"`
	Forced     bool           `json:"forced"`
	SDH        bool           `json:"sdh"`
}

type chapterInput struct {
	Original string `json:"original"`
	English  string `json:"english"`
}

// build makes the path a case describes, as its caller does: the disc title and the chapter
// title come from metadata (TMDB's titles, spec §5), then the library names are built from them.
func build(t *testing.T, in pathInput) (string, error) {
	t.Helper()
	switch in.Kind {
	case "movie":
		return domain.MoviePath(movieOf(in))
	case "episode":
		return domain.EpisodePath(episodeOf(in))
	case "subtitle":
		if in.Video == nil {
			t.Fatal("a subtitle case needs its video")
		}
		video, err := build(t, *in.Video)
		if err != nil {
			return "", err
		}
		return domain.SubtitlePath(video, domain.Subtitle{
			Language:  in.Language,
			Forced:    in.Forced,
			SDH:       in.SDH,
			Extension: in.Extension,
		})
	}
	t.Fatalf("unknown kind %q", in.Kind)
	return "", nil
}

func movieOf(in pathInput) domain.Movie {
	return domain.Movie{
		Root:      rootOf(in, "movies", "Movies"),
		Title:     titleOf(in),
		Version:   in.Version,
		Part:      in.Part,
		Extension: in.Extension,
	}
}

func episodeOf(in pathInput) domain.Episode {
	last := in.Episode
	if in.EpisodeEnd != nil {
		last = *in.EpisodeEnd
	}
	return domain.Episode{
		Root:         rootOf(in, "series", "TV Shows"),
		Title:        titleOf(in),
		Season:       in.Season,
		First:        in.Episode,
		Last:         last,
		ChapterTitle: chapterTitleOf(in),
		Version:      in.Version,
		Extension:    in.Extension,
	}
}

// rootOf returns the folder of a kind: the case's own root when it has one, else the default (spec §5).
func rootOf(in pathInput, which, fallback string) string {
	if in.Roots == nil {
		return fallback
	}
	root := in.Roots.Movies
	if which == "series" {
		root = in.Roots.Series
	}
	if root == "" {
		return fallback
	}
	return root
}

// titleOf returns the title of a case. The tag is on by default for a linked title (RF-45).
func titleOf(in pathInput) domain.Title {
	idTag := domain.DefaultIDTag(in.TMDBID)
	if in.IDTag != nil {
		idTag = *in.IDTag
	}
	return domain.Title{
		Name:   metadata.DiscTitle(in.Original, in.English),
		Year:   in.Year,
		TMDBID: in.TMDBID,
		IDTag:  idTag,
	}
}

func chapterTitleOf(in pathInput) string {
	chapters := make([]metadata.ChapterNames, 0, len(in.Chapters))
	for _, c := range in.Chapters {
		chapters = append(chapters, metadata.ChapterNames{Original: c.Original, English: c.English})
	}
	return metadata.ChapterTitle(chapters)
}

func TestLibraryPathsGoldenVectors(t *testing.T) {
	t.Parallel()

	var v pathVectors
	readVectors(t, &v)
	if len(v.Paths.Cases) == 0 {
		t.Fatal("no path vectors loaded")
	}
	for _, c := range v.Paths.Cases {
		t.Run(c.Name, func(t *testing.T) {
			t.Parallel()
			got, err := build(t, c.Input)
			if c.Expected.Error != "" {
				var nameErr *domain.NameError
				if !errors.As(err, &nameErr) || nameErr.Field != c.Expected.Error {
					t.Fatalf("error = %v, want a NameError for %q", err, c.Expected.Error)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != c.Expected.Value {
				t.Errorf("path = %q, want %q", got, c.Expected.Value)
			}
		})
	}
}

func TestDefaultIDTagIsOnlyForLinkedTitles(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		tmdbID int64
		want   bool
	}{
		{name: "a linked title has the tag by default", tmdbID: 1726, want: true},
		{name: "a free title has no tag", tmdbID: 0, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := domain.DefaultIDTag(tt.tmdbID); got != tt.want {
				t.Errorf("DefaultIDTag(%d) = %v, want %v", tt.tmdbID, got, tt.want)
			}
		})
	}
}

// TestLongNamesKeepTheirParts checks the 255-byte rule (spec §5) on paths whose names need a
// cut: every component fits, the cut is at a character, and what must stay is still there.
func TestLongNamesKeepTheirParts(t *testing.T) {
	t.Parallel()

	long := strings.Repeat("é", 200)
	tests := []struct {
		name      string
		make      func() (string, error)
		mustHave  []string
		mustStart []string
	}{
		{
			name: "a movie folder and file share a cut disc title and keep the tag",
			make: func() (string, error) {
				return domain.MoviePath(domain.Movie{
					Root:      "Movies",
					Title:     domain.Title{Name: long, Year: 2008, TMDBID: 1, IDTag: true},
					Extension: ".mkv",
				})
			},
			mustHave: []string{" (2008) [tmdbid-1]/", " (2008) [tmdbid-1].mkv"},
		},
		{
			name: "a chapter keeps its codes when its title is cut",
			make: func() (string, error) {
				return domain.EpisodePath(domain.Episode{
					Root:         "TV Shows",
					Title:        domain.Title{Name: "Breaking Bad", Year: 2008, TMDBID: 1396, IDTag: true},
					Season:       1,
					First:        1,
					Last:         1,
					ChapterTitle: long,
					Extension:    ".mkv",
				})
			},
			mustHave:  []string{" - S01E01 - ", ".mkv"},
			mustStart: []string{"TV Shows/Breaking Bad (2008) [tmdbid-1396]/Season 01/Breaking Bad (2008) - S01E01 - é"},
		},
		{
			name: "a subtitle keeps its language and flags when the video name is cut",
			make: func() (string, error) {
				video := "Movies/" + strings.Repeat("a", 250) + "/" + strings.Repeat("a", 250) + ".mkv"
				return domain.SubtitlePath(video, domain.Subtitle{Language: "es-MX", Forced: true, Extension: ".srt"})
			},
			mustHave: []string{".es-419.forced.srt"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := tt.make()
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			for _, component := range strings.Split(got, "/") {
				if n := len(component); n > domain.MaxNameBytes {
					t.Errorf("component %q is %d bytes, want at most %d", component, n, domain.MaxNameBytes)
				}
			}
			if !utf8.ValidString(got) {
				t.Errorf("path %q is not valid UTF-8", got)
			}
			for _, want := range tt.mustHave {
				if !strings.Contains(got, want) {
					t.Errorf("path %q lacks %q", got, want)
				}
			}
			for _, want := range tt.mustStart {
				if !strings.HasPrefix(got, want) {
					t.Errorf("path %q does not start with %q", got, want)
				}
			}
		})
	}
}
