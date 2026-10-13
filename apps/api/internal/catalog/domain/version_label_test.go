package domain

import (
	"encoding/json"
	"os"
	"slices"
	"testing"
)

// versionLabelVectors is contracts/version-labels.json, the golden vectors of the version labels.
type versionLabelVectors struct {
	VersionLabel struct {
		Cases []struct {
			Name  string `json:"name"`
			Input struct {
				Versions []struct {
					Width  int    `json:"width"`
					Height int    `json:"height"`
					Codec  string `json:"codec"`
					Audio  []struct {
						Language string `json:"language"`
						Title    string `json:"title"`
					} `json:"audio"`
				} `json:"versions"`
			} `json:"input"`
			Expected struct {
				Labels     []string `json:"labels"`
				Incomplete []bool   `json:"incomplete"`
			} `json:"expected"`
		} `json:"cases"`
	} `json:"versionLabel"`
}

func TestVersionLabelsFollowTheGoldenVectors(t *testing.T) {
	t.Parallel()

	raw, err := os.ReadFile("../../../../../contracts/version-labels.json")
	if err != nil {
		t.Fatal(err)
	}
	var vectors versionLabelVectors
	if err := json.Unmarshal(raw, &vectors); err != nil {
		t.Fatal(err)
	}
	if len(vectors.VersionLabel.Cases) == 0 {
		t.Fatal("the vectors have no cases")
	}
	for _, c := range vectors.VersionLabel.Cases {
		t.Run(c.Name, func(t *testing.T) {
			t.Parallel()

			versions := make([]VersionMedia, 0, len(c.Input.Versions))
			for _, v := range c.Input.Versions {
				m := VersionMedia{Width: v.Width, Height: v.Height, Codec: v.Codec}
				for _, a := range v.Audio {
					m.Audio = append(m.Audio, AudioMedia{Language: a.Language, Title: a.Title})
				}
				versions = append(versions, m)
			}

			got := VersionLabels(versions)
			gotLabels := make([]string, len(got))
			gotIncomplete := make([]bool, len(got))
			for i, l := range got {
				gotLabels[i] = l.Text
				gotIncomplete[i] = l.Incomplete
			}
			if !slices.Equal(gotLabels, c.Expected.Labels) {
				t.Errorf("labels = %q, want %q", gotLabels, c.Expected.Labels)
			}
			if !slices.Equal(gotIncomplete, c.Expected.Incomplete) {
				t.Errorf("incomplete = %v, want %v", gotIncomplete, c.Expected.Incomplete)
			}
		})
	}
}
