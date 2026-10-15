package domain

import (
	"regexp"
	"strconv"
	"strings"
	"unicode"
)

// The import of the library (spec RF-29) reads the names of the title folders and of the files in them. A
// title folder is «Título (Año)» with optional id tags, in any order; the tags are not part of the title.
// Everything here is pure: no I/O.

var (
	// idTagPattern matches the id tags of a title (RF-29, §5): [tmdbid-N] (Jellyfin), {tmdb-N} and [imdbid-ttN].
	idTagPattern = regexp.MustCompile(`(?i)\[tmdbid-(\d+)\]|\{tmdb-(\d+)\}|\[imdbid-(tt\d+)\]`)
	// yearSuffix is the year that ends a name: " (2024)".
	yearSuffix = regexp.MustCompile(`\((19\d{2}|20\d{2})\)$`)
	// seasonFolderName is a season folder: "Season 01", "Temporada 1", "S01"; or the specials, season 0 (§5).
	seasonFolderName = regexp.MustCompile(`(?i)^(?:(?:season|temporada|s)[ ._-]*\d{1,3}|specials?|especiales)$`)
	// partWord is the word of a movie part in a file name: part1, cd2, disc1, pt3 (RF-22, RF-29).
	partWord = regexp.MustCompile(`(?i)^(?:cd|disc|part|pt)-?(\d{1,2})$`)
)

// FolderName is what the name of a title folder says (RF-29).
type FolderName struct {
	// Title is the name without its year and its id tags: the disc title <T> of the folder (§5).
	// "" when nothing is left, and then the folder is no title.
	Title string
	// Year is the year in parentheses at the end of the name; 0 when there is none.
	Year int
	// JellyfinID is the id of the tag [tmdbid-N] (RF-45); 0 when the folder has none.
	JellyfinID int64
	// TMDBID is the id of the tag [tmdbid-N] or {tmdb-N}, the Jellyfin tag first; 0 when there is none.
	TMDBID int64
	// IMDbID is the id of the tag [imdbid-ttN]; "" when there is none.
	IMDbID string
}

// ParseFolderName reads the name of a title folder (RF-29). The year is the one in parentheses at the end,
// once the id tags are left out, so both «Título (2024) [tmdbid-1]» and «Título [tmdbid-1] (2024)» read it.
func ParseFolderName(name string) FolderName {
	fn := FolderName{}
	fn.JellyfinID, fn.TMDBID, fn.IMDbID = IDTagsOf(name)
	rest := strings.TrimSpace(idTagPattern.ReplaceAllString(name, " "))
	if m := yearSuffix.FindStringSubmatchIndex(rest); m != nil {
		fn.Year, _ = strconv.Atoi(rest[m[2]:m[3]])
		rest = strings.TrimSpace(rest[:m[0]])
	}
	fn.Title = rest
	return fn
}

// IDTagsOf returns the Jellyfin id (tag [tmdbid-N]), the TMDB id of the tags [tmdbid-N] or {tmdb-N} (the
// Jellyfin one first) and the IMDb id of the tag [imdbid-ttN] that a name carries (RF-29). Each is 0 or ""
// when the name has none. A tag whose number does not read as an id is ignored.
func IDTagsOf(name string) (jellyfin, tmdb int64, imdb string) {
	for _, m := range idTagPattern.FindAllStringSubmatch(name, -1) {
		if id := parseID(m[1]); id > 0 && jellyfin == 0 && m[1] != "" {
			jellyfin = id
		}
		if id := parseID(m[1] + m[2]); id > 0 && tmdb == 0 {
			tmdb = id
		}
		if m[3] != "" && imdb == "" {
			imdb = strings.ToLower(m[3])
		}
	}
	if jellyfin != 0 {
		tmdb = jellyfin
	}
	return jellyfin, tmdb, imdb
}

// parseID reads a positive TMDB id; 0 when s is empty or not a positive number.
func parseID(s string) int64 {
	if s == "" {
		return 0
	}
	id, err := strconv.ParseInt(s, 10, 64)
	if err != nil || id <= 0 {
		return 0
	}
	return id
}

// titleKey is the form in which two titles are compared when a folder is looked up in TMDB (RF-29): lower
// case, with every sign and every run of spaces read as one space, so «Monstruo - La historia» and
// «Monstruo: La historia» are the same title.
func titleKey(s string) string {
	var b strings.Builder
	space := true
	for _, r := range strings.ToLower(s) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
			space = false
			continue
		}
		if !space {
			b.WriteByte(' ')
			space = true
		}
	}
	return strings.TrimSpace(b.String())
}

// isSeasonFolder reports whether a folder inside a series is a season folder (§5), specials included.
func isSeasonFolder(name string) bool {
	return seasonFolderName.MatchString(name)
}

// partNumberOf reads the number of a movie part from a word: cd1, part2, disc1, pt3; 0 when it is not one.
func partNumberOf(word string) int {
	m := partWord.FindStringSubmatch(word)
	if m == nil {
		return 0
	}
	n, err := strconv.Atoi(m[1])
	if err != nil || n < 1 {
		return 0
	}
	return n
}
