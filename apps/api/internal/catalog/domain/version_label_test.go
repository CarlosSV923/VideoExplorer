package domain

import (
	"encoding/json"
	"errors"
	"os"
	"strings"
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
			// Expected is a JSON array of labels, or an object {"error": field}.
			Expected json.RawMessage `json:"expected"`
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

			got, err := VersionLabels(versions)
			if strings.HasPrefix(string(c.Expected), "{") {
				var want struct {
					Error string `json:"error"`
				}
				if err := json.Unmarshal(c.Expected, &want); err != nil {
					t.Fatal(err)
				}
				var nameErr *NameError
				if !errors.As(err, &nameErr) || nameErr.Field != want.Error {
					t.Fatalf("VersionLabels error = %v, want a NameError for %q", err, want.Error)
				}
				return
			}
			var want []string
			if err := json.Unmarshal(c.Expected, &want); err != nil {
				t.Fatal(err)
			}
			if err != nil {
				t.Fatalf("VersionLabels: %v", err)
			}
			if len(got) != len(want) {
				t.Fatalf("labels = %q, want %q", got, want)
			}
			for i := range want {
				if got[i] != want[i] {
					t.Errorf("label %d = %q, want %q (all: %q)", i, got[i], want[i], got)
				}
			}
		})
	}
}
