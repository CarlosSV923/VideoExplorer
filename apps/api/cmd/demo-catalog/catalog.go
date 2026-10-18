package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/metadata/domain"
)

// catalog holds TMDB structure and image paths, plus the project's own
// synopses. Image bytes and TMDB overviews never enter the file (RF-71).
type catalog struct {
	Source string         `json:"source"`
	Titles []catalogTitle `json:"titles"`
}

type catalogTitle struct {
	TMDBID           int64              `json:"tmdbId"`
	Type             string             `json:"type"` // "movie" or "series"
	Year             int                `json:"year"`
	OriginalTitle    string             `json:"originalTitle"`
	OriginalLanguage string             `json:"originalLanguage"`
	Title            localized          `json:"title"`
	DiscTitle        string             `json:"discTitle"`
	PosterPath       string             `json:"posterPath,omitempty"`
	BackdropPath     string             `json:"backdropPath,omitempty"`
	LogoPath         string             `json:"logoPath,omitempty"`
	Synopsis         localized          `json:"synopsis"`
	Runtime          int                `json:"runtime,omitempty"`
	Genres           []catalogGenre     `json:"genres"`
	Collection       *catalogCollection `json:"collection,omitempty"`
	Seasons          []catalogSeason    `json:"seasons,omitempty"`
}

// localized is a text in the two languages of the demo.
type localized struct {
	ES string `json:"es"`
	EN string `json:"en"`
}

type catalogGenre struct {
	ID   int64     `json:"id"`
	Name localized `json:"name"`
}

type catalogCollection struct {
	ID         int64     `json:"id"`
	Name       localized `json:"name"`
	PosterPath string    `json:"posterPath,omitempty"`
}

type catalogSeason struct {
	Number   int              `json:"number"` // 0 holds the specials
	Episodes []catalogEpisode `json:"episodes"`
}

type catalogEpisode struct {
	Number    int       `json:"number"`
	Title     localized `json:"title"`
	DiscTitle string    `json:"discTitle"` // the <título cap.> of spec §5 for this chapter
	AirDate   string    `json:"airDate,omitempty"`
	StillPath string    `json:"stillPath,omitempty"`
}

const (
	typeMovie  = "movie"
	typeSeries = "series"

	// maxSynopsisRunes keeps a synopsis to a couple of sentences.
	maxSynopsisRunes = 400
)

// forbiddenKey catches TMDB text and binary image fields. Image path fields
// are separately allowlisted for RF-71.
var (
	forbiddenKey   = regexp.MustCompile(`(?i)overview|image|poster|backdrop|logo|still`)
	imagePathKey   = regexp.MustCompile(`(?i)(posterPath|backdropPath|logoPath|stillPath)`)
	validImagePath = regexp.MustCompile(`^/[A-Za-z0-9_-]+\.(jpg|jpeg|png)$`)
)

// validate checks the rules of the demo catalog. The same checks run in the
// generator before it writes the file and in the test over the committed file.
func validate(c catalog) error {
	var errs []error
	if strings.TrimSpace(c.Source) == "" {
		errs = append(errs, errors.New("source is empty"))
	}
	if len(c.Titles) == 0 {
		errs = append(errs, errors.New("no titles"))
	}
	seen := map[string]bool{}
	for _, t := range c.Titles {
		key := fmt.Sprintf("%s:%d", t.Type, t.TMDBID)
		if seen[key] {
			errs = append(errs, fmt.Errorf("%s is repeated", key))
		}
		seen[key] = true
		errs = append(errs, validateTitle(t)...)
	}
	return errors.Join(errs...)
}

func validateTitle(t catalogTitle) []error {
	key := fmt.Sprintf("%s %d", t.Type, t.TMDBID)
	var errs []error
	add := func(format string, args ...any) {
		errs = append(errs, fmt.Errorf("%s: %s", key, fmt.Sprintf(format, args...)))
	}
	if t.TMDBID <= 0 {
		add("tmdbId must be positive")
	}
	if t.Type != typeMovie && t.Type != typeSeries {
		add("unknown type %q", t.Type)
	}
	if t.Year <= 0 {
		add("year is missing")
	}
	if t.OriginalTitle == "" || t.OriginalLanguage == "" {
		add("original title or language is missing")
	}
	if !filled(t.Title) {
		add("title needs es and en")
	}
	if !filled(t.Synopsis) {
		add("synopsis needs es and en")
	}
	for _, s := range []string{t.Synopsis.ES, t.Synopsis.EN} {
		if utf8.RuneCountInString(s) > maxSynopsisRunes {
			add("synopsis is longer than %d characters", maxSynopsisRunes)
		}
	}
	if want := domain.DiscTitle(t.OriginalTitle, t.Title.EN); t.DiscTitle != want {
		add("discTitle is %q, want %q", t.DiscTitle, want)
	}
	if len(t.Genres) == 0 {
		add("no genres")
	}
	for _, g := range t.Genres {
		if g.ID <= 0 || !filled(g.Name) {
			add("genre %d needs an id and a name in es and en", g.ID)
		}
	}
	if c := t.Collection; c != nil && (c.ID <= 0 || !filled(c.Name)) {
		add("collection %d needs an id and a name in es and en", c.ID)
	}
	for _, path := range []string{t.PosterPath, t.BackdropPath, t.LogoPath} {
		if path != "" && !validImagePath.MatchString(path) {
			add("invalid TMDB image path %q", path)
		}
	}
	if t.Collection != nil && t.Collection.PosterPath != "" && !validImagePath.MatchString(t.Collection.PosterPath) {
		add("invalid collection poster path %q", t.Collection.PosterPath)
	}
	switch t.Type {
	case typeMovie:
		if len(t.Seasons) > 0 {
			add("a movie has no seasons")
		}
	case typeSeries:
		errs = append(errs, validateSeasons(key, t.Seasons)...)
	}
	return errs
}

func validateSeasons(key string, seasons []catalogSeason) []error {
	var errs []error
	if len(seasons) == 0 {
		return []error{fmt.Errorf("%s: a series has no seasons", key)}
	}
	last := -1
	for _, s := range seasons {
		if s.Number <= last {
			errs = append(errs, fmt.Errorf("%s: season %d is out of order or repeated", key, s.Number))
		}
		last = s.Number
		if len(s.Episodes) == 0 {
			errs = append(errs, fmt.Errorf("%s: season %d has no episodes", key, s.Number))
		}
		lastEp := -1
		for _, e := range s.Episodes {
			where := fmt.Sprintf("%s S%02dE%02d", key, s.Number, e.Number)
			if e.Number <= lastEp {
				errs = append(errs, fmt.Errorf("%s: episode is out of order or repeated", where))
			}
			lastEp = e.Number
			if !filled(e.Title) {
				errs = append(errs, fmt.Errorf("%s: chapter title needs es and en", where))
			}
			if e.AirDate != "" {
				if _, ok := domain.ParseDate(e.AirDate); !ok {
					errs = append(errs, fmt.Errorf("%s: airDate %q is not YYYY-MM-DD", where, e.AirDate))
				}
			}
			if e.StillPath != "" && !validImagePath.MatchString(e.StillPath) {
				errs = append(errs, fmt.Errorf("%s: invalid TMDB still path %q", where, e.StillPath))
			}
		}
	}
	return errs
}

// filled reports whether both languages have text.
func filled(l localized) bool {
	return strings.TrimSpace(l.ES) != "" && strings.TrimSpace(l.EN) != ""
}

// checkNoTMDBTexts walks the raw JSON and reports any key that names an
// overview or an image. It runs over the bytes, so it also catches fields the
// typed struct would drop.
func checkNoTMDBTexts(raw []byte) error {
	var tree any
	if err := json.Unmarshal(raw, &tree); err != nil {
		return err
	}
	var found []string
	var walk func(v any)
	walk = func(v any) {
		switch x := v.(type) {
		case map[string]any:
			for k, child := range x {
				if (forbiddenKey.MatchString(k) && !imagePathKey.MatchString(k)) || (strings.Contains(strings.ToLower(k), "path") && !imagePathKey.MatchString(k)) {
					found = append(found, k)
				}
				walk(child)
			}
		case []any:
			for _, child := range x {
				walk(child)
			}
		}
	}
	walk(tree)
	if len(found) > 0 {
		return fmt.Errorf("catalog holds TMDB texts or image fields: %v", found)
	}
	return nil
}

// marshal renders the catalog as indented JSON with a final newline. The order
// comes from build, so the output is the same on every run.
func marshal(c catalog) ([]byte, error) {
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(data, '\n'), nil
}
