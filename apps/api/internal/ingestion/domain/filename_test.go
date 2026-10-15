package domain_test

import (
	"encoding/json"
	"os"
	"reflect"
	"testing"

	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/ingestion/domain"
)

// filenameVectors mirrors contracts/filename-cases.json (spec RF-20, RF-22, RF-23, §5).
type filenameVectors struct {
	Videos    filenameSection `json:"videos"`
	Folders   filenameSection `json:"folders"`
	Subtitles filenameSection `json:"subtitles"`
	Unknown   filenameSection `json:"unknown"`
}

type filenameSection struct {
	Cases []filenameCase `json:"cases"`
}

type filenameCase struct {
	Name  string `json:"name"`
	Input struct {
		Path string `json:"path"`
		Kind string `json:"kind"`
	} `json:"input"`
	Expected expectedAnalysis `json:"expected"`
}

// expectedAnalysis has the fields of domain.Analysis as the vectors write them. An
// absent key is the zero value, which is what the analysis leaves out.
type expectedAnalysis struct {
	Type        string `json:"type"`
	Title       string `json:"title"`
	Year        int    `json:"year"`
	Season      *int   `json:"season"`
	Chapter     *int   `json:"chapter"`
	LastChapter *int   `json:"lastChapter"`
	Part        int    `json:"part"`
	Language    string `json:"language"`
	Forced      bool   `json:"forced"`
	SDH         bool   `json:"sdh"`
	TMDBID      int64  `json:"tmdbId"`
	IMDbID      string `json:"imdbId"`
}

func loadFilenameVectors(t *testing.T) filenameVectors {
	t.Helper()
	raw, err := os.ReadFile("../../../../../contracts/filename-cases.json")
	if err != nil {
		t.Fatal(err)
	}
	var v filenameVectors
	if err := json.Unmarshal(raw, &v); err != nil {
		t.Fatal(err)
	}
	if len(v.Videos.Cases) == 0 || len(v.Folders.Cases) == 0 || len(v.Subtitles.Cases) == 0 || len(v.Unknown.Cases) == 0 {
		t.Fatal("no filename vectors loaded")
	}
	return v
}

func toExpected(a domain.Analysis) expectedAnalysis {
	return expectedAnalysis{
		Type:        string(a.Type),
		Title:       a.Title,
		Year:        a.Year,
		Season:      a.Season,
		Chapter:     a.Chapter,
		LastChapter: a.LastChapter,
		Part:        a.Part,
		Language:    a.Language,
		Forced:      a.Forced,
		SDH:         a.SDH,
		TMDBID:      a.TMDBID,
		IMDbID:      a.IMDbID,
	}
}

func kindOf(t *testing.T, kind string) domain.FileKind {
	t.Helper()
	switch kind {
	case "video":
		return domain.KindVideo
	case "subtitle":
		return domain.KindSubtitle
	}
	t.Fatalf("unknown kind %q in the vectors", kind)
	return ""
}

func TestAnalyzeFileGoldenVectors(t *testing.T) {
	t.Parallel()
	v := loadFilenameVectors(t)
	sections := map[string]filenameSection{
		"videos":    v.Videos,
		"folders":   v.Folders,
		"subtitles": v.Subtitles,
		"unknown":   v.Unknown,
	}
	for section, s := range sections {
		for _, c := range s.Cases {
			t.Run(section+"/"+c.Name, func(t *testing.T) {
				t.Parallel()
				got := toExpected(domain.AnalyzeFile(c.Input.Path, kindOf(t, c.Input.Kind)))
				if !reflect.DeepEqual(got, c.Expected) {
					gotJSON, _ := json.Marshal(got)
					wantJSON, _ := json.Marshal(c.Expected)
					t.Errorf("AnalyzeFile(%q, %s)\n got  %s\n want %s", c.Input.Path, c.Input.Kind, gotJSON, wantJSON)
				}
			})
		}
	}
}
