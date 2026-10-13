package domain

import (
	"fmt"
	"path"
	"regexp"
	"strconv"
	"strings"
)

// The library paths of spec §5, in Jellyfin's convention. Each function returns a path
// relative to LIBRARY_PATH with "/" separators. The disc title and the chapter title are
// chosen by the caller (metadata's DiscTitle and ChapterTitle); this package sanitizes them
// with the rules of spec §5 and builds the names. Pure: no I/O.

// extensionPattern is a file extension with its dot: letters and digits only.
var extensionPattern = regexp.MustCompile(`^\.[A-Za-z0-9]+$`)

// Title is a movie or a series as its folder names it (spec §5, <T>, <A> and <ids>).
type Title struct {
	// Name is the disc title <T>, already chosen by the caller. For a free title, the name the
	// user typed. Empty is an error; a name that sanitizes to nothing becomes FallbackName.
	Name string
	// Year is <A>: the year of the premiere or of the first broadcast. 0 when unknown, and then
	// the " (<A>)" part is left out.
	Year int
	// TMDBID is the TMDB id of a linked title; 0 for a free title, which has no tag.
	TMDBID int64
	// IDTag says whether the tag [tmdbid-ID] is on for a linked title (RF-45).
	IDTag bool
}

// DefaultIDTag returns whether the tag [tmdbid-ID] is on when nothing else was chosen (RF-45):
// a title linked to TMDB has it, a free title never does.
func DefaultIDTag(tmdbID int64) bool {
	return tmdbID > 0
}

// Movie is a movie file (spec §5, Movies/ rows).
type Movie struct {
	// Root is MOVIES_DIR, the folder of the movies inside the library.
	Root  string
	Title Title
	// Version is the <versión> label of a second file of the same movie (RF-24); "" for none.
	Version string
	// Part is the number of a part of a movie split into files (RF-22); 0 for none. A part and a
	// version are exclusive.
	Part int
	// Extension is the file's extension with its dot, in any letter case; it is written in lower case.
	Extension string
}

// Episode is a chapter file, or one file with several consecutive chapters (spec §5, TV Shows/ rows).
type Episode struct {
	// Root is SERIES_DIR, the folder of the series inside the library.
	Root  string
	Title Title
	// Season is the season; 0 holds the specials (Season 00).
	Season int
	// First is the number of the first chapter of the file and Last of the last one; Last equals
	// First for a single chapter.
	First, Last int
	// ChapterTitle is the <título cap.> of the file's chapters, already chosen by the caller; ""
	// when no chapter has a title, and then its segment is left out.
	ChapterTitle string
	// Version is the <versión> label of a second file of the same chapter; "" for none.
	Version   string
	Extension string
}

// Subtitle is a subtitle file that goes next to its video (spec §5, last row of the table).
type Subtitle struct {
	// Language is the <idioma> of the subtitle; NormalizeLanguage writes it in the file name.
	Language string
	// Forced and SDH are the .forced and .sdh flags (RF-23).
	Forced bool
	SDH    bool
	// Extension is the subtitle file's extension with its dot; it is written in lower case.
	Extension string
}

// MoviePath returns the path of a movie file (spec §5):
// <root>/<T> (<A>)<ids>/<T> (<A>)<ids>[ - <versión> | - part<N>].<ext>. Its folder and file share
// the same disc title, cut when the names are longer than 255 bytes.
func MoviePath(m Movie) (string, error) {
	root, err := rootOf(m.Root)
	if err != nil {
		return "", err
	}
	head, year, tag, err := titleParts(m.Title)
	if err != nil {
		return "", err
	}
	ext, err := extensionOf(m.Extension)
	if err != nil {
		return "", err
	}
	suffix, err := movieSuffix(m.Version, m.Part)
	if err != nil {
		return "", err
	}
	// The file is the longest name: the disc title is cut for it, so the folder keeps the same head.
	head = shortenStem(head, MaxNameBytes-len(year+tag+suffix)-len(ext))
	folder := fitHead(head, year+tag, "")
	file := fitHead(head, year+tag+suffix, ext) + ext
	return path.Join(root, folder, file), nil
}

// EpisodePath returns the path of a chapter file (spec §5):
// <root>/<T> (<A>)<ids>/Season <SS>/<T> (<A>) - S<SS>E<EE>[-E<FF>][ - <título cap.>][ - <versión>].<ext>.
// The chapter file has no tag: only the series folder has it. When the name is longer than 255
// bytes, the chapter title is cut first and then the disc title.
func EpisodePath(e Episode) (string, error) {
	root, err := rootOf(e.Root)
	if err != nil {
		return "", err
	}
	head, year, tag, err := titleParts(e.Title)
	if err != nil {
		return "", err
	}
	ext, err := extensionOf(e.Extension)
	if err != nil {
		return "", err
	}
	code, err := episodeCode(e.Season, e.First, e.Last)
	if err != nil {
		return "", err
	}
	versionSeg := ""
	if e.Version != "" {
		version := cleanChars(e.Version)
		if finishName(version) == "" {
			return "", &NameError{Field: "version"}
		}
		versionSeg = " - " + version
	}
	chapter := cleanChars(e.ChapterTitle)
	if finishName(chapter) == "" {
		chapter = ""
	}
	rest := year + " - " + code
	chapterSeg := segment(chapter)
	if len(head)+len(rest)+len(chapterSeg)+len(versionSeg)+len(ext) > MaxNameBytes && chapter != "" {
		room := MaxNameBytes - len(head) - len(rest) - len(versionSeg) - len(ext) - len(" - ")
		chapterSeg = segment(shortenStem(chapter, room))
	}
	folder := fitHead(head, year+tag, "")
	file := fitHead(head, rest+chapterSeg+versionSeg, ext) + ext
	return path.Join(root, folder, seasonFolder(e.Season), file), nil
}

// SubtitlePath returns the path of a subtitle that goes with the video at videoPath (spec §5,
// RF-23): the same folder, named after the video without its extension, then the language, the
// .forced and .sdh flags and the extension: <video>.<idioma>[.forced][.sdh].<ext>. When that name
// is longer than 255 bytes, the video's name is cut so the language and the flags stay.
func SubtitlePath(videoPath string, s Subtitle) (string, error) {
	if videoPath == "" {
		return "", &NameError{Field: "video"}
	}
	lang, err := NormalizeLanguage(s.Language)
	if err != nil {
		return "", err
	}
	ext, err := extensionOf(s.Extension)
	if err != nil {
		return "", err
	}
	suffix := "." + lang
	if s.Forced {
		suffix += ".forced"
	}
	if s.SDH {
		suffix += ".sdh"
	}
	dir, file := path.Split(videoPath)
	stem := fitStem(strings.TrimSuffix(file, path.Ext(file)), suffix+ext)
	return path.Join(dir, stem+suffix+ext), nil
}

// TitleFolder returns the folder of a title under root (spec §5): <root>/<T> (<A>)<ids>. It is the
// folder that the movie and chapter paths of the title share, so it names the title in the registry.
func TitleFolder(root string, t Title) (string, error) {
	r, err := rootOf(root)
	if err != nil {
		return "", err
	}
	head, year, tag, err := titleParts(t)
	if err != nil {
		return "", err
	}
	return path.Join(r, fitHead(head, year+tag, "")), nil
}

// titleParts checks a title and returns its disc title, ready to be joined (steps 1 to 5 of
// spec §5), the year part " (<A>)" and the tag part " [tmdbid-ID]", each "" when it is left out.
func titleParts(t Title) (head, year, tag string, err error) {
	switch {
	case t.Name == "":
		return "", "", "", &NameError{Field: "title"}
	case t.Year < 0:
		return "", "", "", &NameError{Field: "year"}
	case t.TMDBID < 0:
		return "", "", "", &NameError{Field: "tmdbId"}
	}
	head = cleanChars(t.Name)
	if finishName(head) == "" {
		head = FallbackName
	}
	if t.Year > 0 {
		year = " (" + strconv.Itoa(t.Year) + ")"
	}
	if t.TMDBID > 0 && t.IDTag {
		tag = " [tmdbid-" + strconv.FormatInt(t.TMDBID, 10) + "]"
	}
	return head, year, tag, nil
}

// movieSuffix returns the part of a movie file's name that tells it from the others: a version
// or a part number (RF-22: exclusive). It is "" for the only file of the movie.
func movieSuffix(version string, part int) (string, error) {
	switch {
	case part < 0:
		return "", &NameError{Field: "part"}
	case part > 0 && version != "":
		return "", &NameError{Field: "part"}
	case part > 0:
		return " - part" + strconv.Itoa(part), nil
	case version != "":
		v := cleanChars(version)
		if finishName(v) == "" {
			return "", &NameError{Field: "version"}
		}
		return " - " + v, nil
	}
	return "", nil
}

// episodeCode returns the code S<SS>E<EE> or S<SS>E<EE>-E<FF> of a chapter file (spec §5).
// Numbers have at least two digits.
func episodeCode(season, first, last int) (string, error) {
	switch {
	case season < 0:
		return "", &NameError{Field: "season"}
	case first < 1:
		return "", &NameError{Field: "episode"}
	case last < first:
		return "", &NameError{Field: "episodeEnd"}
	}
	code := "S" + pad2(season) + "E" + pad2(first)
	if last > first {
		code += "-E" + pad2(last)
	}
	return code, nil
}

// seasonFolder returns the folder of a season: "Season 01", "Season 00" for the specials.
func seasonFolder(season int) string {
	return "Season " + pad2(season)
}

// rootOf checks the library folder a file goes in (MOVIES_DIR or SERIES_DIR).
func rootOf(root string) (string, error) {
	if root == "" {
		return "", &NameError{Field: "root"}
	}
	return root, nil
}

// extensionOf checks a file's extension and returns it in lower case (spec §5, <ext>).
func extensionOf(ext string) (string, error) {
	if !extensionPattern.MatchString(ext) {
		return "", &NameError{Field: "extension"}
	}
	return strings.ToLower(ext), nil
}

// segment returns the " - " segment of a name for a piece of text, or "" when the text is empty.
func segment(text string) string {
	if text == "" {
		return ""
	}
	return " - " + text
}

// fitHead returns the stem of a name made of head and rest, shortened so that the stem and ext
// fit in MaxNameBytes: the head is cut first, so the rest (the year, the tag, the codes, the
// version) stays (spec §5). The result is sanitized as a whole component (step 6).
func fitHead(head, rest, ext string) string {
	head = shortenStem(head, MaxNameBytes-len(rest)-len(ext))
	return fitStem(finishName(head+rest), ext)
}

// fitStem shortens stem so that stem and ext fit in MaxNameBytes, at a character boundary.
func fitStem(stem, ext string) string {
	if len(stem)+len(ext) <= MaxNameBytes {
		return stem
	}
	return shortenStem(stem, MaxNameBytes-len(ext))
}

// pad2 writes a number with at least two digits.
func pad2(n int) string {
	return fmt.Sprintf("%02d", n)
}
