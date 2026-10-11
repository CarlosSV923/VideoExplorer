package domain

import (
	"regexp"
	"slices"
	"strings"
)

// Type is what a file is, as far as its name and its folders say (spec RF-20).
type Type string

// The types a file can get.
const (
	// TypeMovie: a movie, with its year and, for a part, its number (RF-22).
	TypeMovie Type = "movie"
	// TypeEpisode: a chapter of a series (§5). Season 0 holds the specials.
	TypeEpisode Type = "episode"
	// TypeUnknown: the name does not say what the file is, such as absolute numbering (§8).
	TypeUnknown Type = "unknown"
)

// Analysis is what the name of a file, and its folders, say about it (spec RF-20,
// RF-22, RF-23, §5). A zero field is absent: an empty title, a year 0, a part 0,
// an empty language. Season, Chapter and LastChapter are nil when the name does not
// give them; a season or chapter 0 is a real value (specials).
type Analysis struct {
	Type        Type
	Title       string
	Year        int
	Season      *int
	Chapter     *int
	LastChapter *int
	// Part is the number of a movie part (cd1, part2, disc1, pt2); movies only.
	Part int
	// Language is the subtitle language: ISO 639-1, with a region only where Jellyfin
	// recognizes it ("es-419"). Subtitles only.
	Language string
	// Forced is the .forced flag of a subtitle.
	Forced bool
	// SDH is the hearing-impaired flag of a subtitle (.sdh, and .cc or .hi).
	SDH bool
}

// AnalyzeFile reads what a file says about itself (RF-20). relPath is the path of the
// file inside its entry, folders included, with '/' or '\' as separator; kind says
// whether it is a video or a subtitle, which decides the extension to take off. The
// rules and the choices they fix are in contracts/filename-cases.json.
func AnalyzeFile(relPath string, kind FileKind) Analysis {
	dirs, file := splitRelPath(relPath)
	base := trimExtension(file, kind)
	var tags subtitleTags
	if kind == KindSubtitle {
		base, tags = cutSubtitleTags(base)
	}
	name := parseName(base)
	seasonAt, season, hasSeason := findSeasonFolder(dirs)
	folderTitle, folderYear := folderTitles(dirs, seasonAt, hasSeason)

	out := Analysis{Language: tags.language, Forced: tags.forced, SDH: tags.sdh}
	switch {
	case name.episode != nil && name.episode.invalid:
		out.Type = TypeUnknown
		out.Title, out.Year = pickTitle(name, folderTitle, folderYear)
	case name.episode != nil:
		ep := name.episode
		out.Type = TypeEpisode
		out.Season, out.Chapter = intPtr(ep.season), intPtr(ep.chapter)
		if ep.last > 0 {
			out.LastChapter = intPtr(ep.last)
		}
		out.Title, out.Year = pickTitle(name, folderTitle, folderYear)
	case hasSeason:
		// Inside a season folder the file name is a chapter title, unless it is a number.
		out.Type = TypeEpisode
		out.Season = intPtr(season)
		out.Chapter = name.bare
		out.Title, out.Year = folderTitle, folderYear
	case name.anime || name.bare != nil:
		out.Type = TypeUnknown
		out.Title, out.Year = pickTitle(name, folderTitle, folderYear)
	default:
		out.Type = TypeMovie
		out.Title, out.Year = pickTitle(name, folderTitle, folderYear)
		out.Part = name.part
	}
	if out.Type == TypeMovie && out.Title == "" {
		out.Type = TypeUnknown
	}
	return out
}

// subtitleTags are the tags a subtitle name ends with: <name>.<language>.<flags>.
type subtitleTags struct {
	language string
	forced   bool
	sdh      bool
}

// nameParts is what one name, without its extension and subtitle tags, says.
type nameParts struct {
	title   string
	year    int
	episode *episodeMark
	bare    *int // the name is only a number: "01", "01 - Pilot"
	anime   bool // "Title - 001": absolute numbering (§8)
	part    int
}

// episodeMark is a chapter marker: SxxEyy, SxxEyyEzz, SxxEy-Ezz, SxxEy-zz or NxNN.
type episodeMark struct {
	season  int
	chapter int
	last    int // 0 when the marker names one chapter
	invalid bool
}

var (
	videoExtensions    = []string{"mkv", "mp4", "avi", "m4v", "ts", "m2ts", "mov", "wmv", "webm", "mpg", "mpeg"}
	subtitleExtensions = []string{"srt", "ass", "ssa", "sub", "idx", "vtt"}

	// seasonFolder is "Season 1", "Temporada 01", "S1"; specialFolder is season 0.
	seasonFolder  = regexp.MustCompile(`(?i)^(?:season|temporada|s)[ ._-]*(\d{1,3})$`)
	specialFolder = regexp.MustCompile(`(?i)^(?:specials?|especiales)$`)

	episodeSxxEyy = regexp.MustCompile(`(?i)^s(\d{1,3})e(\d{1,3})(?:-?e?(\d{1,3}))?$`)
	episodeNxNN   = regexp.MustCompile(`(?i)^(\d{1,2})x(\d{1,3})(?:-(\d{1,3}))?$`)
	partPattern   = regexp.MustCompile(`(?i)^(?:cd|disc|part|pt)-?(\d{1,2})$`)
	yearPattern   = regexp.MustCompile(`^((?:19|20)\d{2})(?:-[A-Za-z][A-Za-z0-9]*)?$`)
	bareNumber    = regexp.MustCompile(`^\d{1,3}$`)
	regionPattern = regexp.MustCompile(`^(?:[A-Za-z]{2}|\d{3})$`)
	bracketGroup  = regexp.MustCompile(`\[[^\]]*\]`)

	// tagPattern matches the technical tags that end a title: resolution, source,
	// streaming service, codec, HDR, bit depth, audio and their scene group.
	tagPattern = regexp.MustCompile(`(?i)^(?:\d{3,4}[pi]|[248]k|uhd|hdr(?:10)?\+?|\d{1,2}bit|` +
		`bluray|blu-ray|bdrip|brrip|bd|dvdrip|dvdscr|hdrip|hdtv|webrip|web-dl|webdl|web|remux|hdcam|telesync|` +
		`amzn|nf|dsnp|hmax|atvp|hulu|pcok|` +
		`(?:x26[45]|h26[45]|hevc|avc|av1|vp9|xvid|divx)(?:-[a-z0-9]+)?|` +
		`(?:aac|ac3|eac3|ddp?|dd|dts(?:-hd)?|truehd|atmos|flac|mp3|opus)\d*(?:-[a-z0-9]+)?)$`)

	separators = strings.NewReplacer("(", " ", ")", " ", ".", " ", "_", " ")
)

// keptRegions are the regions Jellyfin recognizes, which the names keep (§5, spike 12.2).
var keptRegions = map[string]bool{"pt-BR": true, "pt-PT": true, "fr-CA": true, "zh-TW": true}

// latinAmericanRegions turn es-XX into es-419 (§5).
var latinAmericanRegions = map[string]bool{
	"AR": true, "BO": true, "CL": true, "CO": true, "CR": true, "CU": true, "DO": true,
	"EC": true, "GT": true, "HN": true, "MX": true, "NI": true, "PA": true, "PE": true,
	"PR": true, "PY": true, "SV": true, "UY": true, "VE": true,
}

// isoCodes maps the ISO 639-1 and 639-2 codes of the common languages to ISO 639-1.
var isoCodes = map[string]string{
	"es": "es", "spa": "es",
	"en": "en", "eng": "en",
	"fr": "fr", "fra": "fr", "fre": "fr",
	"de": "de", "deu": "de", "ger": "de",
	"it": "it", "ita": "it",
	"pt": "pt", "por": "pt",
	"ja": "ja", "jpn": "ja",
	"ko": "ko", "kor": "ko",
	"zh": "zh", "zho": "zh", "chi": "zh",
	"ru": "ru", "rus": "ru",
	"pl": "pl", "pol": "pl",
	"nl": "nl", "nld": "nl", "dut": "nl",
	"sv": "sv", "swe": "sv",
	"da": "da", "dan": "da",
	"fi": "fi", "fin": "fi",
	"cs": "cs", "ces": "cs", "cze": "cs",
	"hu": "hu", "hun": "hu",
	"tr": "tr", "tur": "tr",
	"el": "el", "ell": "el", "gre": "el",
	"he": "he", "heb": "he",
	"ar": "ar", "ara": "ar",
	"hi": "hi", "hin": "hi",
	"th": "th", "tha": "th",
	"uk": "uk", "ukr": "uk",
	"ro": "ro", "ron": "ro", "rum": "ro",
	"id": "id", "ind": "id",
	"vi": "vi", "vie": "vi",
	"no": "no", "nor": "no",
}

// languageNames maps the names of the common languages, in lowercase, to ISO 639-1.
var languageNames = map[string]string{
	"spanish": "es", "español": "es", "espanol": "es", "castellano": "es", "latino": "es-419",
	"english": "en", "inglés": "en", "ingles": "en",
	"french": "fr", "français": "fr", "francais": "fr",
	"german": "de", "deutsch": "de", "alemán": "de", "aleman": "de",
	"italian": "it", "italiano": "it",
	"portuguese": "pt", "português": "pt",
	"japanese": "ja", "japonés": "ja", "japones": "ja",
	"korean": "ko", "coreano": "ko",
	"chinese": "zh", "chino": "zh",
	"russian": "ru", "ruso": "ru",
}

// genericNames are file names that say nothing about the title (§5 folders, RF-20).
var genericNames = []string{"movie", "film", "video", "main", "feature", "index"}

// AnalyzeFile helpers, in the order they are used.

func splitRelPath(rel string) ([]string, string) {
	var names []string
	for _, p := range strings.FieldsFunc(rel, isSeparator) {
		if p != "." {
			names = append(names, p)
		}
	}
	if len(names) == 0 {
		return nil, ""
	}
	return names[:len(names)-1], names[len(names)-1]
}

func isSeparator(r rune) bool { return r == '/' || r == '\\' }

func trimExtension(file string, kind FileKind) string {
	i := strings.LastIndexByte(file, '.')
	if i <= 0 {
		return file
	}
	exts := videoExtensions
	if kind == KindSubtitle {
		exts = subtitleExtensions
	}
	if slices.Contains(exts, strings.ToLower(file[i+1:])) {
		return file[:i]
	}
	return file
}

// cutSubtitleTags takes the tags off the end of a subtitle name without its extension
// (RF-23): <name>.<language>[.forced|.sdh|.cc|.hi]. The language sits right before the
// flags. The name keeps its first part.
func cutSubtitleTags(name string) (string, subtitleTags) {
	parts := strings.Split(name, ".")
	var tags subtitleTags
	j := len(parts) - 1
	for j >= 1 && isSubtitleFlag(parts, j) {
		j--
	}
	flags := parts[j+1:]
	if j >= 1 {
		if code, ok := languageOf(parts[j], true); ok {
			tags.language = code
			j--
		}
	}
	for _, f := range flags {
		switch strings.ToLower(f) {
		case "forced":
			tags.forced = true
		case "sdh", "cc", "hi":
			tags.sdh = true
		}
	}
	return strings.Join(parts[:j+1], "."), tags
}

// isSubtitleFlag reports whether parts[j] is a flag: forced and sdh always; cc and hi
// only right after a language (RF-23), so a lone hi is Hindi and a lone cc stays in the
// name.
func isSubtitleFlag(parts []string, j int) bool {
	switch strings.ToLower(parts[j]) {
	case "forced", "sdh":
		return true
	case "cc", "hi":
		if j < 2 {
			return false
		}
		_, ok := languageOf(parts[j-1], true)
		return ok
	}
	return false
}

// languageOf reads a language tag: a name ("Spanish", "Español"), an ISO 639-1 or 639-2
// code, or one of them with a region ("es-MX"). anyCase lets codes in capitals count
// (subtitles); in a video name, a code must be lowercase so that a title word is not read.
func languageOf(seg string, anyCase bool) (string, bool) {
	if lang, region, ok := strings.Cut(seg, "-"); ok {
		code, known := isoCodes[strings.ToLower(lang)]
		if !known || !regionPattern.MatchString(region) {
			return "", false
		}
		return regionalCode(code, strings.ToUpper(region)), true
	}
	if code, ok := languageNames[strings.ToLower(seg)]; ok {
		return code, true
	}
	if !anyCase && seg != strings.ToLower(seg) {
		return "", false
	}
	code, ok := isoCodes[strings.ToLower(seg)]
	return code, ok
}

// regionalCode applies the region rules of §5: es-MX and the other Latin American
// regions are es-419, es-ES is es, en-* is en, the kept regions stay, and any other
// region is dropped.
func regionalCode(code, region string) string {
	switch code {
	case "es":
		if region == "ES" {
			return "es"
		}
		if region == "419" || latinAmericanRegions[region] {
			return "es-419"
		}
		return "es"
	case "en":
		return "en"
	}
	if kept := code + "-" + region; keptRegions[kept] {
		return kept
	}
	return code
}

// parseName reads a name without its extension and subtitle tags: the title up to the
// first marker, tag or year, the year, the chapter or the part.
func parseName(name string) nameParts {
	tokens := tokenize(name)
	if len(tokens) > 0 && bareNumber.MatchString(tokens[0]) && (len(tokens) == 1 || tokens[1] == "-") {
		n := atoi(tokens[0])
		return nameParts{bare: &n}
	}
	var np nameParts
	end := len(tokens)
	for i, tok := range tokens {
		if ep, ok := episodeToken(tok); ok {
			np.episode = &ep
			end = i
			break
		}
		if i > 0 && tagPattern.MatchString(tok) {
			end = i
			break
		}
		if n, ok := partToken(tok); ok {
			np.part = n
			end = i
			break
		}
		if i > 0 && tok == "-" && i+1 < len(tokens) && bareNumber.MatchString(tokens[i+1]) {
			np.anime = true
			end = i
			break
		}
	}
	// The last year before the marker is the year: "Blade Runner 2049 (2017)" is 2017.
	yearAt := -1
	for i := 1; i < end; i++ {
		if y, ok := yearToken(tokens[i]); ok {
			np.year = y
			yearAt = i
		}
	}
	titleEnd := end
	if yearAt >= 0 {
		titleEnd = yearAt
	}
	words := tokens[:titleEnd]
	for len(words) > 0 && (words[len(words)-1] == "-" || isLanguageTag(words[len(words)-1])) {
		words = words[:len(words)-1]
	}
	np.title = strings.Join(words, " ")
	return np
}

// tokenize splits a name into words at dots, underscores and spaces. Brackets (site
// tags, TMDB ids) go, parentheses separate, and "H.264" becomes one codec token.
func tokenize(name string) []string {
	words := strings.Fields(separators.Replace(bracketGroup.ReplaceAllString(name, " ")))
	tokens := make([]string, 0, len(words))
	for i := 0; i < len(words); i++ {
		w := words[i]
		if strings.EqualFold(w, "h") && i+1 < len(words) && (words[i+1] == "264" || words[i+1] == "265") {
			w = "H" + words[i+1]
			i++
		}
		tokens = append(tokens, w)
	}
	return tokens
}

func episodeToken(tok string) (episodeMark, bool) {
	m := episodeSxxEyy.FindStringSubmatch(tok)
	if m == nil {
		m = episodeNxNN.FindStringSubmatch(tok)
	}
	if m == nil {
		return episodeMark{}, false
	}
	ep := episodeMark{season: atoi(m[1]), chapter: atoi(m[2])}
	if m[3] != "" {
		ep.last = atoi(m[3])
		ep.invalid = ep.last <= ep.chapter
	}
	return ep, true
}

func partToken(tok string) (int, bool) {
	m := partPattern.FindStringSubmatch(tok)
	if m == nil {
		return 0, false
	}
	n := atoi(m[1])
	return n, n >= 1
}

func yearToken(tok string) (int, bool) {
	m := yearPattern.FindStringSubmatch(tok)
	if m == nil {
		return 0, false
	}
	return atoi(m[1]), true
}

// isLanguageTag reports whether a word at the end of a video title is a language
// tag, which the title leaves out.
func isLanguageTag(word string) bool {
	_, ok := languageOf(word, false)
	return ok
}

func isGenericName(title string) bool {
	return slices.Contains(genericNames, strings.ToLower(title))
}

// findSeasonFolder returns the nearest season folder: its index, its season and true,
// or false when there is none.
func findSeasonFolder(dirs []string) (int, int, bool) {
	for i := len(dirs) - 1; i >= 0; i-- {
		if specialFolder.MatchString(dirs[i]) {
			return i, 0, true
		}
		if m := seasonFolder.FindStringSubmatch(dirs[i]); m != nil {
			return i, atoi(m[1]), true
		}
	}
	return -1, 0, false
}

// folderTitles is the title and year that the folders give to a file: the folder above
// its season folder when it is in one, else the file's own folder.
func folderTitles(dirs []string, seasonAt int, hasSeason bool) (string, int) {
	switch {
	case hasSeason:
		if seasonAt == 0 {
			return "", 0
		}
		n := parseName(dirs[seasonAt-1])
		return n.title, n.year
	case len(dirs) > 0:
		n := parseName(dirs[len(dirs)-1])
		return n.title, n.year
	}
	return "", 0
}

// pickTitle takes the title and year of the file when it has a title, else those of
// its folders. A generic name with no year (movie.mkv) is no title of its own, so the
// folders decide, and with no folder title there is none. The file's year wins; without
// one, the folder's year comes only when the file's title is the folder's title (RF-20).
func pickTitle(name nameParts, folderTitle string, folderYear int) (string, int) {
	generic := name.year == 0 && isGenericName(name.title)
	if name.title == "" || generic {
		return folderTitle, folderYear
	}
	year := name.year
	if year == 0 && folderTitle != "" && sameTitle(name.title, folderTitle) {
		year = folderYear
	}
	return name.title, year
}

// sameTitle compares two titles ignoring case, dots, underscores and spaces (RF-20).
func sameTitle(a, b string) bool {
	return normalizeTitle(a) == normalizeTitle(b)
}

var titleSeparators = strings.NewReplacer(".", " ", "_", " ")

func normalizeTitle(title string) string {
	return strings.Join(strings.Fields(titleSeparators.Replace(strings.ToLower(title))), " ")
}

func atoi(digits string) int {
	n := 0
	for _, c := range digits {
		n = n*10 + int(c-'0')
	}
	return n
}

func intPtr(n int) *int { return &n }
