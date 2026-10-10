package domain_test

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/metadata/domain"
)

type titleVectors struct {
	DiscTitle struct {
		Cases []struct {
			Name  string `json:"name"`
			Input struct {
				Original string `json:"original"`
				English  string `json:"english"`
			} `json:"input"`
			Expected string `json:"expected"`
		} `json:"cases"`
	} `json:"discTitle"`
	ChapterTitle struct {
		Cases []struct {
			Name  string `json:"name"`
			Input struct {
				Chapters []struct {
					Original string `json:"original"`
					English  string `json:"english"`
				} `json:"chapters"`
			} `json:"input"`
			Expected string `json:"expected"`
		} `json:"cases"`
	} `json:"chapterTitle"`
}

func loadTitleVectors(t *testing.T) titleVectors {
	t.Helper()
	raw, err := os.ReadFile("../../../../../contracts/naming-cases.json")
	if err != nil {
		t.Fatal(err)
	}
	var v titleVectors
	if err := json.Unmarshal(raw, &v); err != nil {
		t.Fatal(err)
	}
	if len(v.DiscTitle.Cases) == 0 || len(v.ChapterTitle.Cases) == 0 {
		t.Fatal("no title vectors loaded")
	}
	return v
}

func TestDiscTitleGoldenVectors(t *testing.T) {
	t.Parallel()
	for _, c := range loadTitleVectors(t).DiscTitle.Cases {
		t.Run(c.Name, func(t *testing.T) {
			t.Parallel()
			if got := domain.DiscTitle(c.Input.Original, c.Input.English); got != c.Expected {
				t.Errorf("DiscTitle = %q, want %q", got, c.Expected)
			}
		})
	}
}

func TestChapterTitleGoldenVectors(t *testing.T) {
	t.Parallel()
	for _, c := range loadTitleVectors(t).ChapterTitle.Cases {
		t.Run(c.Name, func(t *testing.T) {
			t.Parallel()
			chapters := make([]domain.ChapterNames, 0, len(c.Input.Chapters))
			for _, ch := range c.Input.Chapters {
				chapters = append(chapters, domain.ChapterNames{Original: ch.Original, English: ch.English})
			}
			if got := domain.ChapterTitle(chapters); got != c.Expected {
				t.Errorf("ChapterTitle = %q, want %q", got, c.Expected)
			}
		})
	}
}
