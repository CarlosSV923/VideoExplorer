package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
)

// Paths relative to this package, where `go test` runs.
const (
	committedCatalog  = "../../../web/src/modules/metadata/infrastructure/demo/catalog.json"
	committedSynopses = "synopses.json"
)

// decodeCatalog reads a catalog and rejects fields the struct does not know.
func decodeCatalog(raw []byte) (catalog, error) {
	var c catalog
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	err := dec.Decode(&c)
	return c, err
}

// TestCommittedCatalogIsValid checks the file the demo ships: it loads, every
// title has its synopses in es and en, it holds no TMDB texts or image fields,
// its disc titles equal domain.DiscTitle, and it is in the generator's format
// (a round trip gives the same bytes). No network: it reads the committed file.
func TestCommittedCatalogIsValid(t *testing.T) {
	t.Parallel()
	raw, err := os.ReadFile(committedCatalog) //nolint:gosec // committed file
	if err != nil {
		t.Fatalf("read catalog: %v", err)
	}
	if err := checkNoTMDBTexts(raw); err != nil {
		t.Fatal(err)
	}
	c, err := decodeCatalog(raw)
	if err != nil {
		t.Fatalf("decode catalog: %v", err)
	}
	if err := validate(c); err != nil {
		t.Fatalf("catalog is not valid: %v", err)
	}
	again, err := marshal(c)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !bytes.Equal(raw, again) {
		t.Error("the committed catalog is not in the generator's format; run task demo-catalog")
	}

	syn, err := loadSynopses(committedSynopses)
	if err != nil {
		t.Fatalf("synopses: %v", err)
	}
	if err := matchSynopses(c, syn); err != nil {
		t.Fatal(err)
	}
}

// matchSynopses checks that the synopses file and the catalog name the same titles.
func matchSynopses(c catalog, syn synopsesFile) error {
	movies, series := map[int64]bool{}, map[int64]bool{}
	for _, t := range c.Titles {
		if t.Type == typeMovie {
			movies[t.TMDBID] = true
			if _, ok := syn.Movies[t.TMDBID]; !ok {
				return fmt.Errorf("no synopsis for movie %d", t.TMDBID)
			}
		} else {
			series[t.TMDBID] = true
			if _, ok := syn.Series[t.TMDBID]; !ok {
				return fmt.Errorf("no synopsis for series %d", t.TMDBID)
			}
		}
	}
	for id := range syn.Movies {
		if !movies[id] {
			return fmt.Errorf("synopsis for movie %d matches no title", id)
		}
	}
	for id := range syn.Series {
		if !series[id] {
			return fmt.Errorf("synopsis for series %d matches no title", id)
		}
	}
	return nil
}

// TestCommittedCatalogCoversTheDemo keeps the selection able to show what the
// demo needs (spec RF-40, RF-41, RF-43, RF-70): several genres and collections,
// specials, non-Latin and non-English originals, and remakes with one title.
func TestCommittedCatalogCoversTheDemo(t *testing.T) {
	t.Parallel()
	raw, err := os.ReadFile(committedCatalog) //nolint:gosec // committed file
	if err != nil {
		t.Fatalf("read catalog: %v", err)
	}
	c, err := decodeCatalog(raw)
	if err != nil {
		t.Fatalf("decode catalog: %v", err)
	}
	var movies, series, specials, nonLatin, latinNotEnglish int
	genres := map[int64]bool{}
	collections := map[int64]bool{}
	discs := map[string]int{}
	for _, title := range c.Titles {
		discs[title.DiscTitle]++
		for _, g := range title.Genres {
			genres[g.ID] = true
		}
		if title.Collection != nil {
			collections[title.Collection.ID] = true
		}
		if title.Type == typeMovie {
			movies++
		} else {
			series++
			for _, s := range title.Seasons {
				if s.Number == 0 {
					specials++
				}
			}
		}
		// The disc title is the English one only when the original is not Latin (spec §5).
		if title.DiscTitle != title.OriginalTitle {
			nonLatin++
		} else if title.OriginalLanguage != "en" {
			latinNotEnglish++
		}
	}
	remakes := 0
	for _, n := range discs {
		if n > 1 {
			remakes++
		}
	}
	checks := []struct {
		name string
		got  int
		min  int
	}{
		{"movies", movies, 20},
		{"series", series, 6},
		{"genres", len(genres), 5},
		{"collections", len(collections), 2},
		{"specials (season 0)", specials, 1},
		{"non-Latin originals", nonLatin, 3},
		{"Latin originals that are not English", latinNotEnglish, 1},
		{"remakes sharing a disc title", remakes, 1},
	}
	for _, ch := range checks {
		if ch.got < ch.min {
			t.Errorf("%s = %d, want at least %d", ch.name, ch.got, ch.min)
		}
	}
}

func TestCheckNoTMDBTexts(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		json    string
		wantErr bool
	}{
		{name: "clean catalog", json: `{"titles":[{"title":{"es":"a","en":"b"},"synopsis":{"es":"c","en":"d"}}]}`},
		{name: "overview is a TMDB text", json: `{"titles":[{"overview":"x"}]}`, wantErr: true},
		{name: "poster path is an image", json: `{"titles":[{"posterPath":"/x.jpg"}]}`, wantErr: true},
		{name: "nested backdrop path", json: `{"titles":[{"seasons":[{"backdrop_path":"/y.jpg"}]}]}`, wantErr: true},
		{name: "logo in a list", json: `{"titles":[{"logos":[]}]}`, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := checkNoTMDBTexts([]byte(tt.json))
			if (err != nil) != tt.wantErr {
				t.Errorf("error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidateRejectsBrokenCatalogs(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		mutate func(*catalog)
		want   string
	}{
		{"missing source", func(c *catalog) { c.Source = "" }, "source is empty"},
		{"empty synopsis in English", func(c *catalog) { c.Titles[0].Synopsis.EN = "" }, "synopsis needs es and en"},
		{"disc title not the one of spec §5", func(c *catalog) { c.Titles[0].DiscTitle = "Other" }, "discTitle is"},
		{"repeated title", func(c *catalog) { c.Titles = append(c.Titles, c.Titles[0]) }, "is repeated"},
		{"season out of order", func(c *catalog) {
			s := c.Titles[3]
			s.Seasons[0], s.Seasons[1] = s.Seasons[1], s.Seasons[0]
			c.Titles[3] = s
		}, "out of order"},
		{"chapter without English title", func(c *catalog) {
			c.Titles[3].Seasons[1].Episodes[0].Title.EN = ""
		}, "chapter title needs es and en"},
		{"airDate in another format", func(c *catalog) {
			c.Titles[3].Seasons[1].Episodes[0].AirDate = "04/03/2019"
		}, "is not YYYY-MM-DD"},
		{"a movie with seasons", func(c *catalog) {
			c.Titles[0].Seasons = []catalogSeason{{Number: 1, Episodes: []catalogEpisode{{Number: 1, Title: localized{ES: "a", EN: "b"}}}}}
		}, "a movie has no seasons"},
		{"synopsis longer than the limit", func(c *catalog) {
			c.Titles[0].Synopsis.ES = strings.Repeat("a", maxSynopsisRunes+1)
		}, "longer than"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			c := fixtureCatalog(t)
			tt.mutate(&c)
			err := validate(c)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %v, want one containing %q", err, tt.want)
			}
		})
	}
}

// fixtureCatalog is the valid catalog of the fixture world. Its titles are
// sorted as build sorts them: movies, then series.
func fixtureCatalog(t *testing.T) catalog {
	t.Helper()
	c, err := build(t.Context(), fixtureTMDB(), fixtureCurated(), fixtureSynopses())
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	return c
}

func TestParseSynopsesRejectsUnknownFields(t *testing.T) {
	t.Parallel()
	_, err := parseSynopses([]byte(`{"movies":{"1":{"es":"a","en":"b"}},"series":{},"sinopsis":{}}`))
	if err == nil {
		t.Fatal("expected an error for an unknown field")
	}
}
