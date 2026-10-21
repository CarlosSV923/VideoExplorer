package application

import (
	"context"
	"errors"
	"log/slog"
	"net/url"
	"path"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode"

	"golang.org/x/text/unicode/norm"

	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/catalog/domain"
)

// DefaultSubtitleLanguage is the language a subtitle whose name does not give one is preselected
// with: Latin American Spanish, the usual language of the library (RF-23).
const DefaultSubtitleLanguage = "es-419"

// Roots are the two library folders of the titles (MOVIES_DIR and SERIES_DIR, spec §5).
type Roots struct {
	Movies, Series string
}

// AssignDeps are what Asignar needs; the composition root wires them.
type AssignDeps struct {
	Unassigned UnassignedRepository
	Library    LibraryRepository
	Files      Files
	Engine     *Engine
	Finder     *DuplicateFinder
	Media      MediaReader
	TMDB       TMDB
	Names      NameReader
	Roots      Roots
	// VideoExts and SubtitleExts are the extensions with their dot (VIDEO_EXTENSIONS, SUBTITLE_EXTENSIONS).
	VideoExts    []string
	SubtitleExts []string
	// Now and NewToken may be nil (time.Now and a random token). Log may be nil.
	Now      func() time.Time
	NewToken func() string
	Log      *slog.Logger
}

// Assignments assigns the entries of No asignados to the library (spec RF-20 to RF-26): it holds
// the lock that keeps an entry for one tab, suggests what each file is, searches the titles, plans
// the result of the choices and commits it through the engine.
type Assignments struct {
	d        AssignDeps
	videoExt map[string]bool
	subExt   map[string]bool
	now      func() time.Time
	newToken func() string
	log      *slog.Logger
	// section reads No asignados as the disk holds it, so that an entry still being copied is refused
	// (RF-27) and the files that the commit moves are the ones on disk.
	section *Section
}

// NewAssignments builds the assignment use cases from their ports.
func NewAssignments(d AssignDeps) *Assignments {
	a := &Assignments{d: d, now: d.Now, newToken: d.NewToken, log: d.Log}
	if a.now == nil {
		a.now = time.Now
	}
	if a.newToken == nil {
		a.newToken = randomName
	}
	if a.log == nil {
		a.log = slog.New(slog.DiscardHandler)
	}
	a.videoExt = extSet(d.VideoExts)
	a.subExt = extSet(d.SubtitleExts)
	a.section = NewSection(SectionDeps{
		Unassigned: d.Unassigned, Files: d.Files, Engine: d.Engine,
		VideoExts: d.VideoExts, SubtitleExts: d.SubtitleExts, Now: a.now,
	})
	return a
}

// extSet returns the extensions as a set, in lower case.
func extSet(exts []string) map[string]bool {
	set := make(map[string]bool, len(exts))
	for _, e := range exts {
		set[strings.ToLower(e)] = true
	}
	return set
}

// Take opens an assignment of the entry for the tab that sends token (RF-26). An empty token makes
// a new one. It answers ErrAssigning when another tab holds the entry, and renews the lock of the
// tab that holds it.
// It answers ErrCopying when a file of the entry is still being copied (RF-27): nothing is assigned
// then. The reading of the section runs under the engine's lock, with the lock taken.
func (a *Assignments) Take(ctx context.Context, entryID int64, token string) (domain.AssignLock, error) {
	var lock domain.AssignLock
	err := a.d.Engine.Locked(func() error {
		entries, err := a.section.read(ctx)
		if err != nil {
			return err
		}
		e := findEntry(entries, entryID)
		if e == nil {
			return ErrEntryNotFound
		}
		if e.isCopying() {
			return ErrCopying
		}
		entry, err := a.entry(ctx, entryID)
		if err != nil {
			return err
		}
		now := a.now()
		if entry.Lock.Held(now) && entry.Lock.Token != token {
			return ErrAssigning
		}
		if token == "" {
			token = a.newToken()
		}
		until := now.Add(domain.AssignLockTTL)
		ok, err := a.d.Unassigned.Lock(ctx, entryID, token, now, until)
		if err != nil {
			return err
		}
		if !ok {
			return ErrAssigning
		}
		lock = domain.AssignLock{Token: token, Until: until}
		return nil
	})
	if err != nil {
		return domain.AssignLock{}, err
	}
	return lock, nil
}

// Release frees the lock of the entry when token holds it (RF-26: cancelling the form). Another
// token changes nothing.
func (a *Assignments) Release(ctx context.Context, entryID int64, token string) error {
	if _, err := a.entry(ctx, entryID); err != nil {
		return err
	}
	return a.d.Unassigned.Unlock(ctx, entryID, token)
}

// entry returns the entry with its id, or ErrEntryNotFound.
func (a *Assignments) entry(ctx context.Context, id int64) (domain.UnassignedEntry, error) {
	entry, ok, err := a.d.Unassigned.ByID(ctx, id)
	if err != nil {
		return domain.UnassignedEntry{}, err
	}
	if !ok {
		return domain.UnassignedEntry{}, ErrEntryNotFound
	}
	return entry, nil
}

// requireLock answers nil when token holds the entry's lock now. Another tab's lock is ErrAssigning
// (RF-26); no lock at all, or one that expired, is ErrLockLost.
func (a *Assignments) requireLock(entry domain.UnassignedEntry, token string) error {
	now := a.now()
	if entry.Lock.HeldBy(token, now) {
		return nil
	}
	if entry.Lock.Held(now) {
		return ErrAssigning
	}
	return ErrLockLost
}

// TitleHit is a title that the assignment can take (RF-21): one of the registry, or one that TMDB
// answers. LibraryID is set when a TMDB title is already in the library, so that it is assigned
// there and not created again.
type TitleHit struct {
	Source     string
	LibraryID  int64
	TMDBID     int64
	Kind       domain.TitleKind
	Title      string
	Year       int
	PosterFile string
}

// SearchResult is what the title search answers: the library titles that match, then the TMDB
// results (RF-21). TMDBAvailable is false when TMDB is not configured or did not answer.
type SearchResult struct {
	TMDBAvailable bool
	Library       []TitleHit
	TMDB          []TitleHit
}

// Search looks for titles by words or an identifier (RF-21). Library titles whose words all appear
// in the title come first, without regard to case or accents (RF-42). year, when positive, narrows
// the TMDB name search.
func (a *Assignments) Search(ctx context.Context, query string, year int) (SearchResult, error) {
	words := strings.Fields(foldText(query))
	kindFilter, tmdbID, imdbID := parseTitleSearchID(query)
	if tmdbID != 0 || imdbID != "" {
		words = nil
	}
	if len(words) == 0 && tmdbID == 0 && imdbID == "" {
		return SearchResult{}, ErrInvalidQuery
	}
	titles, _, err := classifyTitles(ctx, a.d.Library)
	if err != nil {
		return SearchResult{}, err
	}
	var out SearchResult
	for _, t := range titles {
		if len(words) == 0 {
			break
		}
		text := foldText(t.Title + " " + t.DiscTitle)
		if containsAllWords(text, words) {
			out.Library = append(out.Library, libraryHit(t))
		}
	}
	slices.SortStableFunc(out.Library, func(x, y TitleHit) int {
		return strings.Compare(foldText(x.Title), foldText(y.Title))
	})
	libraryIDMatches := make(map[domain.TitleKind]bool)
	if tmdbID > 0 {
		for _, title := range titles {
			if title.TMDBID == tmdbID && (kindFilter == "" || title.Kind == kindFilter) && !libraryIDMatches[title.Kind] {
				out.TMDB = append(out.TMDB, libraryHit(title))
				libraryIDMatches[title.Kind] = true
			}
		}
	}
	if !a.d.TMDB.Configured() {
		return out, nil
	}
	out.TMDBAvailable = true
	for _, kind := range []domain.TitleKind{domain.KindMovie, domain.KindSeries} {
		if kindFilter != "" && kind != kindFilter {
			continue
		}
		if libraryIDMatches[kind] {
			continue
		}
		if tmdbID > 0 {
			t, err := a.d.TMDB.Title(ctx, kind, tmdbID)
			if err == nil {
				hit := TitleHit{Source: "tmdb", TMDBID: t.ID, Kind: kind, Title: t.Title, Year: t.Year, PosterFile: t.PosterFile}
				hit.LibraryID = libraryIDOf(titles, kind, t.ID)
				if hit.LibraryID != 0 {
					hit.Source = "library"
				}
				out.TMDB = append(out.TMDB, hit)
			} else if !errors.Is(err, ErrTMDBNotFound) {
				out.TMDBAvailable = false
			}
			continue
		}
		if imdbID != "" {
			finder, ok := a.d.TMDB.(interface {
				FindIMDb(context.Context, domain.TitleKind, string) ([]TMDBTitle, error)
			})
			if !ok {
				out.TMDBAvailable = false
				continue
			}
			hits, err := finder.FindIMDb(ctx, kind, imdbID)
			if err != nil {
				if !errors.Is(err, ErrTMDBNotFound) {
					out.TMDBAvailable = false
				}
				continue
			}
			for _, t := range hits {
				hit := TitleHit{Source: "tmdb", TMDBID: t.ID, Kind: kind, Title: t.Title, Year: t.Year, PosterFile: t.PosterFile}
				hit.LibraryID = libraryIDOf(titles, kind, t.ID)
				if hit.LibraryID != 0 {
					hit.Source = "library"
				}
				out.TMDB = append(out.TMDB, hit)
			}
			continue
		}
		hits, err := a.d.TMDB.SearchTitles(ctx, kind, query, year)
		if err != nil {
			a.log.WarnContext(ctx, "TMDB search failed", "kind", kind, "error", err)
			out.TMDBAvailable = false
			continue
		}
		for _, h := range hits {
			hit := TitleHit{Source: "tmdb", TMDBID: h.ID, Kind: kind, Title: h.Title, Year: h.Year, PosterFile: h.PosterFile}
			hit.LibraryID = libraryIDOf(titles, kind, h.ID)
			if hit.LibraryID != 0 {
				hit.Source = "library"
			}
			out.TMDB = append(out.TMDB, hit)
		}
	}
	return out, nil
}

var (
	tmdbURLID = regexp.MustCompile(`^(\d+)(?:-|$)`)
	imdbURLID = regexp.MustCompile(`^tt\d+$`)
)

// parseTitleSearchID recognizes the public identifiers accepted by RF-21. A blank kindFilter means
// the numeric or IMDb identifier may refer to either kind; regular text remains a title search.
func parseTitleSearchID(input string) (kindFilter domain.TitleKind, tmdbID int64, imdbID string) {
	input = strings.TrimSpace(input)
	if id, err := strconv.ParseInt(input, 10, 64); err == nil && id > 0 {
		return "", id, ""
	}
	if imdbURLID.MatchString(strings.ToLower(input)) {
		return "", 0, strings.ToLower(input)
	}
	_, taggedTMDBID, taggedIMDbID := domain.IDTagsOf(input)
	if taggedTMDBID > 0 || taggedIMDbID != "" {
		return "", taggedTMDBID, taggedIMDbID
	}

	urlInput := input
	if !strings.Contains(urlInput, "://") && (strings.HasPrefix(strings.ToLower(urlInput), "www.") || strings.HasPrefix(strings.ToLower(urlInput), "themoviedb.org/") || strings.HasPrefix(strings.ToLower(urlInput), "imdb.com/")) {
		urlInput = "https://" + urlInput
	}
	u, err := url.Parse(urlInput)
	if err != nil || u.Hostname() == "" {
		return "", 0, ""
	}
	host := strings.ToLower(strings.TrimSuffix(u.Hostname(), "."))
	segments := strings.Split(strings.Trim(u.EscapedPath(), "/"), "/")
	switch host {
	case "themoviedb.org", "www.themoviedb.org":
		for i := 0; i+1 < len(segments); i++ {
			var kind domain.TitleKind
			switch segments[i] {
			case "movie":
				kind = domain.KindMovie
			case "tv":
				kind = domain.KindSeries
			default:
				continue
			}
			match := tmdbURLID.FindStringSubmatch(segments[i+1])
			if len(match) != 2 {
				return "", 0, ""
			}
			id, err := strconv.ParseInt(match[1], 10, 64)
			if err == nil && id > 0 {
				return kind, id, ""
			}
		}
	case "imdb.com", "www.imdb.com":
		for _, segment := range segments {
			if imdbURLID.MatchString(strings.ToLower(segment)) {
				return "", 0, strings.ToLower(segment)
			}
		}
	}
	return "", 0, ""
}

// libraryHit is a registry title as a search result.
func libraryHit(t domain.LibraryTitle) TitleHit {
	return TitleHit{Source: "library", LibraryID: t.ID, TMDBID: t.TMDBID, Kind: t.Kind, Title: t.Title, Year: t.Year}
}

// libraryIDOf returns the id of the registry title of kind with TMDB id tmdbID, or 0.
func libraryIDOf(titles []domain.LibraryTitle, kind domain.TitleKind, tmdbID int64) int64 {
	for _, t := range titles {
		if t.Kind == kind && t.TMDBID == tmdbID && tmdbID > 0 {
			return t.ID
		}
	}
	return 0
}

// foldText returns s without accents and in lower case, the form in which titles are compared.
func foldText(s string) string {
	var b strings.Builder
	for _, r := range norm.NFD.String(strings.ToLower(s)) {
		if !unicode.Is(unicode.Mn, r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// containsAllWords reports whether every word is in text.
func containsAllWords(text string, words []string) bool {
	for _, w := range words {
		if !strings.Contains(text, w) {
			return false
		}
	}
	return true
}

// Suggestions is what Asignar starts from (RF-20, RF-23): what each file of the entry is, and the
// title that TMDB finds for it, with the relation of its batch already applied (RF-03).
type Suggestions struct {
	EntryID        int64
	Name           string
	Relation       string
	TMDBConfigured bool
	// Title is the title of the whole entry for a relation of chapters: the best match of its series.
	Title     *TitleHit
	Videos    []VideoSuggestion
	Subtitles []SubtitleSuggestion
}

// VideoSuggestion is a video of the entry, what its name says and its best TMDB match.
type VideoSuggestion struct {
	File    string
	Reading NameReading
	Match   *TitleHit
}

// SubtitleSuggestion is a subtitle of the entry: its language and flags, and the video it goes with
// when its name says so. Video is "" when no video is named (the user chooses, RF-23).
type SubtitleSuggestion struct {
	File     string
	Language string
	Forced   bool
	SDH      bool
	Video    string
}

// Suggest returns the suggestions of the entry, for the tab that holds its lock (RF-20).
func (a *Assignments) Suggest(ctx context.Context, entryID int64, token string) (Suggestions, error) {
	entry, err := a.entry(ctx, entryID)
	if err != nil {
		return Suggestions{}, err
	}
	if err := a.requireLock(entry, token); err != nil {
		return Suggestions{}, err
	}
	titles, _, err := classifyTitles(ctx, a.d.Library)
	if err != nil {
		return Suggestions{}, err
	}
	out := Suggestions{
		EntryID: entry.ID, Name: entry.Name, Relation: entry.Relation, TMDBConfigured: a.d.TMDB.Configured(),
	}
	for _, f := range entry.Files {
		rel := f.Rel
		switch {
		case a.isVideo(entryPath(entry, rel)):
			reading := a.d.Names.Read(entryPath(entry, rel), false)
			s := VideoSuggestion{File: rel, Reading: reading}
			if out.TMDBConfigured {
				_, tmdbID, imdbID := domain.IDTagsOf(rel)
				s.Match = a.bestMatch(ctx, reading, titles, tmdbID, imdbID)
			}
			out.Videos = append(out.Videos, s)
		case a.isSubtitle(entryPath(entry, rel)):
			out.Subtitles = append(out.Subtitles, a.subtitleSuggestion(entry, rel))
		}
	}
	if entry.Relation == domain.RelationEpisodes {
		for _, v := range out.Videos {
			if v.Reading.Kind == domain.KindSeries && v.Match != nil {
				out.Title = v.Match
				break
			}
		}
	}
	return out, nil
}

// bestMatch returns the first TMDB result for what the name says, of the kind it says (RF-20), or nil.
func (a *Assignments) bestMatch(ctx context.Context, r NameReading, titles []domain.LibraryTitle, tmdbID int64, imdbID string) *TitleHit {
	if tmdbID > 0 {
		hit, err := a.d.TMDB.Title(ctx, r.Kind, tmdbID)
		if err == nil {
			return a.titleHit(r.Kind, hit, titles)
		}
		if !errors.Is(err, ErrTMDBNotFound) {
			a.log.WarnContext(ctx, "TMDB id match failed", "kind", r.Kind, "error", err)
		}
		return nil
	}
	if imdbID != "" {
		if finder, ok := a.d.TMDB.(interface {
			FindIMDb(context.Context, domain.TitleKind, string) ([]TMDBTitle, error)
		}); ok {
			hits, err := finder.FindIMDb(ctx, r.Kind, imdbID)
			if err == nil && len(hits) > 0 {
				return a.titleHit(r.Kind, hits[0], titles)
			}
			if err != nil && !errors.Is(err, ErrTMDBNotFound) {
				a.log.WarnContext(ctx, "TMDB IMDb match failed", "kind", r.Kind, "error", err)
			}
		}
		return nil
	}
	if r.Kind == "" || r.Title == "" {
		return nil
	}
	hits, err := a.d.TMDB.SearchTitles(ctx, r.Kind, r.Title, r.Year)
	if err != nil {
		a.log.WarnContext(ctx, "TMDB match failed", "kind", r.Kind, "error", err)
		return nil
	}
	if len(hits) == 0 {
		return nil
	}
	return a.titleHit(r.Kind, hits[0], titles)
}

func (a *Assignments) titleHit(kind domain.TitleKind, title TMDBTitle, titles []domain.LibraryTitle) *TitleHit {
	h := TitleHit{Source: "tmdb", TMDBID: title.ID, Kind: kind, Title: title.Title, Year: title.Year, PosterFile: title.PosterFile}
	h.LibraryID = libraryIDOf(titles, kind, h.TMDBID)
	if h.LibraryID != 0 {
		h.Source = "library"
	}
	return &h
}

// subtitleSuggestion reads a subtitle's language and flags, and the one video its name shares.
func (a *Assignments) subtitleSuggestion(entry domain.UnassignedEntry, rel string) SubtitleSuggestion {
	r := a.d.Names.Read(entryPath(entry, rel), true)
	s := SubtitleSuggestion{File: rel, Language: r.Language, Forced: r.Forced, SDH: r.SDH}
	if s.Language == "" {
		s.Language = DefaultSubtitleLanguage
	}
	var named []string
	for _, f := range entry.Files {
		if a.isVideo(entryPath(entry, f.Rel)) && sameFolder(f.Rel, rel) && subtitleNamed(f.Rel, rel) {
			named = append(named, f.Rel)
		}
	}
	if len(named) == 1 {
		s.Video = named[0]
	}
	return s
}

// subtitleNamed reports whether the subtitle's name starts with the video's name and a dot (RF-23).
func subtitleNamed(videoRel, subtitleRel string) bool {
	_, ok := domain.SubtitleSuffix(videoRel, subtitleRel)
	return ok
}

// sameFolder reports whether two files of an entry are in the same folder.
func sameFolder(a, b string) bool {
	return path.Dir(a) == path.Dir(b)
}

// isVideo and isSubtitle tell the files of an entry by their extension (VIDEO_EXTENSIONS).
func (a *Assignments) isVideo(name string) bool {
	return a.videoExt[strings.ToLower(path.Ext(name))]
}

func (a *Assignments) isSubtitle(name string) bool {
	return a.subExt[strings.ToLower(path.Ext(name))]
}

// isMedia tells the videos and the subtitles of an entry: what a junk folder may hold and keep (RF-27).
func (a *Assignments) isMedia(name string) bool {
	return a.isVideo(name) || a.isSubtitle(name)
}

// entryPath is the path of a file of an entry as its name is read: the entry's name, then the
// file's place inside it. A loose entry's file is the entry itself.
func entryPath(entry domain.UnassignedEntry, rel string) string {
	if rel == "" {
		return entry.Name
	}
	return path.Join(entry.Name, rel)
}

// libraryPath is the path of a file of an entry inside the library, where it waits (_unassigned/).
func libraryPath(entry domain.UnassignedEntry, rel string) string {
	return path.Join(domain.UnassignedDir, entryPath(entry, rel))
}

// isNotFound reports whether err is TMDB saying that it does not know what was asked.
func isNotFound(err error) bool {
	return errors.Is(err, ErrTMDBNotFound)
}
