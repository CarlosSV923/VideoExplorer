package domain

import (
	"regexp"
	"strings"
	"unicode"
)

// ChapterNames holds the TMDB titles of one chapter: Original in the language
// of the series, English in en-US. An empty field means TMDB has no such title.
type ChapterNames struct {
	Original string
	English  string
}

// genericTitle matches the titles TMDB gives to chapters with no real title,
// such as "Episode 3" or "Episodio 3" (spec §5). Any number counts.
var genericTitle = regexp.MustCompile(`(?i)^(?:episode|episodio|cap[ií]tulo|chapter)[ ]+\d+$`)

// numberedSuffix matches the suffixes that two chapters of one file may differ
// by: " (2)", ", Part 2", ", Parte 2" or a final " 2". The group before it is the base.
var numberedSuffix = regexp.MustCompile(`(?i)^(.*?)(?:[ ]*\(\d+\)|,[ ]*(?:part|parte)[ ]+\d+|[ ]+\d+)$`)

// DiscTitle returns the disc title <T> of a movie or series (spec §5): the
// original title when all its letters are Latin, otherwise the English title,
// and the original title when there is no English one. A title with no letters
// (digits and signs only) counts as Latin. Empty only when both titles are empty.
func DiscTitle(original, english string) string {
	original = strings.TrimSpace(original)
	english = strings.TrimSpace(english)
	if english == "" || (original != "" && isLatin(original)) {
		return original
	}
	return english
}

// ChapterTitle returns the chapter title <título cap.> of a file that holds the
// given chapters (spec §5). Each chapter takes its original title when that is
// Latin, otherwise its English title; a chapter with neither has no title, and
// neither has a generic title such as "Episode 3". Several titles are joined
// with " + ", unless they all share the same base (see titleBase), which is then
// the title. Empty when no chapter has a title.
func ChapterTitle(chapters []ChapterNames) string {
	titles := make([]string, 0, len(chapters))
	for _, c := range chapters {
		if t := chapterTitleOf(c); t != "" {
			titles = append(titles, t)
		}
	}
	switch len(titles) {
	case 0:
		return ""
	case 1:
		return titles[0]
	}
	if base, ok := commonBase(titles); ok {
		return base
	}
	return strings.Join(titles, " + ")
}

// chapterTitleOf picks the title of one chapter, or "" when it has none.
func chapterTitleOf(c ChapterNames) string {
	original := keepTitle(c.Original)
	if original != "" && isLatin(original) {
		return original
	}
	return keepTitle(c.English)
}

// keepTitle trims a TMDB title and drops it when it is empty or generic.
func keepTitle(t string) string {
	t = strings.TrimSpace(t)
	if genericTitle.MatchString(t) {
		return ""
	}
	return t
}

// commonBase returns the base shared by all titles, when they all have one.
func commonBase(titles []string) (string, bool) {
	base := titleBase(titles[0])
	for _, t := range titles[1:] {
		if titleBase(t) != base {
			return "", false
		}
	}
	return base, true
}

// titleBase returns the title without its numbered suffix. A title that would
// have an empty base keeps its own text.
func titleBase(t string) string {
	m := numberedSuffix.FindStringSubmatch(t)
	if m == nil || strings.TrimSpace(m[1]) == "" {
		return t
	}
	return m[1]
}

// isLatin reports whether every letter of s is in the Latin script. Digits,
// signs and combining marks are not letters and do not count.
func isLatin(s string) bool {
	for _, r := range s {
		if unicode.IsLetter(r) && !unicode.Is(unicode.Latin, r) {
			return false
		}
	}
	return true
}
