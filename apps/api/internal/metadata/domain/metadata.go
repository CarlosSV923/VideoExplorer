// Package domain is the metadata model: our own view of movies, series,
// seasons and collections, independent of TMDB's schema (this context is an
// anti-corruption layer). The language rules of spec §6 and RF-61 are pure
// functions here, so every adapter applies them the same way.
package domain

import (
	"errors"
	"strings"
	"time"
	"unicode/utf8"
)

var (
	// ErrNotConfigured means no TMDB credentials are set: the app works with free names (spec §6).
	ErrNotConfigured = errors.New("metadata provider not configured")
	// ErrUpstream means TMDB failed, could not be reached or answered something unexpected.
	ErrUpstream = errors.New("metadata provider error")
	// ErrNotFound means TMDB does not know the requested id.
	ErrNotFound = errors.New("not found")
	// ErrInvalidQuery is returned for empty or overlong search terms and for years that are not positive.
	ErrInvalidQuery = errors.New("invalid search query")
)

// maxQueryRunes bounds a search term; longer input is rejected, not truncated.
const maxQueryRunes = 200

// MediaKind is the type of a title in TMDB's catalogue.
type MediaKind string

// Media kinds.
const (
	KindMovie  MediaKind = "movie"
	KindSeries MediaKind = "series"
)

// ImageKind is a type of image the app shows (spec §6).
type ImageKind string

// Image kinds.
const (
	ImagePoster   ImageKind = "poster"
	ImageBackdrop ImageKind = "backdrop"
	ImageLogo     ImageKind = "logo"
	ImageStill    ImageKind = "still"
)

// Genre is a genre in one language.
type Genre struct {
	ID   int64
	Name string
}

// Logo is a title logo. Language is ISO 639-1, or "" for a logo without text.
// VoteAverage and VoteCount are TMDB's rating of the logo, used to choose one.
type Logo struct {
	Path        string
	Language    string
	VoteAverage float64
	VoteCount   int
}

// PickLogo chooses the logo of a title for language (RF-61, the hero). Groups
// are tried in order: the exact language, another variant of the same
// language, English, and logos without text. Inside a group the best rated
// logo wins; ties go to the one with more votes, then to the lower path so the
// choice is stable. Logos in other languages are never chosen. TMDB logos only
// carry ISO 639-1, so for a language with a region the exact group is empty and
// the same-language group does the work. ok is false when no logo qualifies.
func PickLogo(logos []Logo, language string) (logo Logo, ok bool) {
	groups := []func(Logo) bool{
		func(l Logo) bool { return l.Language != "" && strings.EqualFold(l.Language, language) },
		func(l Logo) bool { return l.Language != "" && primaryLanguage(l.Language) == primaryLanguage(language) },
		func(l Logo) bool { return l.Language != "" && primaryLanguage(l.Language) == "en" },
		func(l Logo) bool { return l.Language == "" },
	}
	for _, inGroup := range groups {
		var best Logo
		found := false
		for _, l := range logos {
			if l.Path == "" || !inGroup(l) {
				continue
			}
			if !found || betterLogo(l, best) {
				best, found = l, true
			}
		}
		if found {
			return best, true
		}
	}
	return Logo{}, false
}

// betterLogo reports whether a outranks b: higher average, then more votes, then a lower path.
func betterLogo(a, b Logo) bool {
	if a.VoteAverage != b.VoteAverage {
		return a.VoteAverage > b.VoteAverage
	}
	if a.VoteCount != b.VoteCount {
		return a.VoteCount > b.VoteCount
	}
	return a.Path < b.Path
}

// Images holds the TMDB path of each image type ("" when missing). The
// adapter turns paths into URLs at the sizes of spec §6.
type Images struct {
	Poster   string
	Backdrop string
	Logos    []Logo
}

// Translation is the title and overview that TMDB holds for one language.
// Language is BCP 47 as used by the app, for example "es-MX" or "en-US".
type Translation struct {
	Language string
	Title    string
	Overview string
}

// MovieSummary is a movie in a search result.
type MovieSummary struct {
	ID               int64
	Title            string
	OriginalTitle    string
	OriginalLanguage string
	Year             *int
	PosterPath       string
}

// SeriesSummary is a series in a search result.
type SeriesSummary struct {
	ID               int64
	Title            string
	OriginalTitle    string
	OriginalLanguage string
	Year             *int
	PosterPath       string
}

// Movie is a movie with its localized text (RF-61), original title and images.
// Title falls back to the English title and then to the original one; Overview
// falls back to English. EnglishTitle is empty when TMDB has no English title,
// which the naming rule of spec §5 needs.
type Movie struct {
	ID               int64
	Title            string
	EnglishTitle     string
	OriginalTitle    string
	OriginalLanguage string
	Overview         string
	Year             *int
	Runtime          int // minutes; 0 when TMDB does not know it
	Genres           []Genre
	Collection       *CollectionRef
	Images           Images
}

// CollectionRef is the collection a movie belongs to.
type CollectionRef struct {
	ID         int64
	Name       string
	PosterPath string
}

// Series is a series with its seasons. Titles follow the same rules as Movie.
type Series struct {
	ID               int64
	Title            string
	EnglishTitle     string
	OriginalTitle    string
	OriginalLanguage string
	Overview         string
	Year             *int // year of the first air date
	Genres           []Genre
	Seasons          []SeasonSummary
	EpisodeCount     int
	Images           Images
}

// SeasonSummary is one season listed in a series. Number 0 holds the specials.
type SeasonSummary struct {
	Number       int
	EpisodeCount int
	AirDate      time.Time // zero when unknown
	PosterPath   string
}

// Season is one season of a series with its episodes.
type Season struct {
	SeriesID int64
	Number   int
	Episodes []Episode
}

// Episode is one chapter. Title and Overview may be empty when TMDB has no
// text for them in any language the app asks for.
type Episode struct {
	ID           int64
	SeasonNumber int
	Number       int
	Title        string
	Overview     string
	AirDate      time.Time // zero when unknown
	Runtime      int       // minutes; 0 when unknown
	StillPath    string
}

// Collection is a TMDB collection (a saga of movies).
type Collection struct {
	ID           int64
	Name         string
	Overview     string
	PosterPath   string
	BackdropPath string
	Parts        []CollectionPart
}

// CollectionPart is a movie of a collection.
type CollectionPart struct {
	ID            int64
	Title         string
	OriginalTitle string
	Year          *int
	PosterPath    string
}

// Localize returns the title and overview to show in language (RF-61). Each
// field takes the first of: the translation in exactly that language, another
// variant of the same language (the first one TMDB lists), English (en-US, or
// another English variant), and for the title the original title. Missing text
// stays empty when no translation has it.
func Localize(original string, translations []Translation, language string) (title, overview string) {
	title = firstNonEmpty(textIn(translations, language, titleOf), original)
	overview = textIn(translations, language, overviewOf)
	return title, overview
}

// textIn applies the RF-61 order to one field of the translations.
func textIn(translations []Translation, language string, field func(Translation) string) string {
	if v := field(translationFor(translations, language)); v != "" {
		return v
	}
	for _, t := range translations {
		if primaryLanguage(t.Language) == primaryLanguage(language) && field(t) != "" {
			return field(t)
		}
	}
	return englishField(translations, field)
}

// EnglishTitle returns the English title (en-US, or another English variant),
// or "" when TMDB has none.
func EnglishTitle(translations []Translation) string {
	return englishField(translations, titleOf)
}

// ParseDate reads a TMDB date (YYYY-MM-DD). It reports false for empty or malformed values.
func ParseDate(s string) (time.Time, bool) {
	t, err := time.Parse(time.DateOnly, s)
	return t, err == nil
}

// YearOf returns the year of a date, or nil when the date is unknown.
func YearOf(date time.Time) *int {
	if date.IsZero() {
		return nil
	}
	y := date.Year()
	return &y
}

// NormalizeQuery trims a search term, collapses its spaces and rejects an
// empty or overlong one.
func NormalizeQuery(q string) (string, error) {
	q = strings.Join(strings.Fields(q), " ")
	if q == "" || utf8.RuneCountInString(q) > maxQueryRunes {
		return "", ErrInvalidQuery
	}
	return q, nil
}

// translationFor returns the translation whose language is exactly language
// (case-insensitive), or the zero value.
func translationFor(ts []Translation, language string) Translation {
	for _, t := range ts {
		if strings.EqualFold(t.Language, language) {
			return t
		}
	}
	return Translation{}
}

// englishField returns the field from the en-US translation when it has it,
// else from the first English variant that has it.
func englishField(ts []Translation, field func(Translation) string) string {
	if v := field(translationFor(ts, "en-US")); v != "" {
		return v
	}
	for _, t := range ts {
		if primaryLanguage(t.Language) == "en" && field(t) != "" {
			return field(t)
		}
	}
	return ""
}

func titleOf(t Translation) string    { return t.Title }
func overviewOf(t Translation) string { return t.Overview }

func primaryLanguage(language string) string {
	lang, _, _ := strings.Cut(language, "-")
	return strings.ToLower(lang)
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}
