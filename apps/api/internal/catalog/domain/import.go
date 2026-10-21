package domain

import (
	"path"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
)

// The import of the library (spec RF-29) decides what the registry holds after a scan of Movies/ and
// TV Shows/. What Jellyfin recognizes is registered where it is, without renaming or moving anything; what
// cannot be read goes to No asignados, keeping its path inside its folder; what is gone from the disk
// leaves the registry. The decision is a pure function of the disk, the registry and what TMDB answered,
// so the scan can run it twice: once to know which titles to look up, and once under the library lock to
// change anything (see ImportPlan).

// ImportRoot is a root of the library that the import scans (RF-29): MOVIES_DIR holds movies and
// SERIES_DIR holds series.
type ImportRoot struct {
	Name string
	Kind TitleKind
}

// DiskRoot is a root as the disk holds it now: its entries, as Entries groups them. A loose entry is a file
// that lies directly in the root.
type DiskRoot struct {
	Root    ImportRoot
	Entries []Folder
}

// MediaExts are the extensions, with their dot, of a video and of a subtitle (VIDEO_EXTENSIONS and
// SUBTITLE_EXTENSIONS). Any other file is media only when it is an archive (RF-27a).
type MediaExts struct {
	Video    []string
	Subtitle []string
}

// FileReading is what the name of a file says about it, read by the ingestion's analysis (RF-20, RF-23).
// A nil season or chapter is a name that gives none.
type FileReading struct {
	Season  *int
	Chapter *int
	Last    *int
	// Language, Forced and SDH describe a subtitle (RF-23); Language "" when the name gives none.
	Language string
	Forced   bool
	SDH      bool
	// TMDBID and IMDbID are the id tags of the file's own name (RF-29); 0 and "" when there are none.
	TMDBID int64
	IMDbID string
}

// ReadName reads what a file says about itself; rel is its path inside its title folder (or its name, for a
// loose file) and subtitle says whether it is a subtitle (RF-20).
type ReadName func(rel string, subtitle bool) FileReading

// RegisteredTitle is a title of the registry with its files, trashed ones included (RF-30).
type RegisteredTitle struct {
	Title LibraryTitle
	Files []LibraryFile
}

// TMDBHit is a title that TMDB answers for one lookup of the import (RF-29).
type TMDBHit struct {
	ID            int64
	Title         string
	OriginalTitle string
	Year          int
}

// Lookup is what TMDB said about a title folder that is not registered yet (RF-29). The application asks for
// it before the change, outside the library lock; a folder with no lookup stays free («Sin vincular»).
type Lookup struct {
	// ByID is the title of the folder's TMDB id; nil when the folder has none or TMDB does not know it.
	ByID *TMDBHit
	// ByIMDb holds the titles TMDB links to the folder's IMDb id, of the folder's kind.
	ByIMDb []TMDBHit
	// Search holds the titles TMDB answers for the folder's title (and year, when it has one).
	Search []TMDBHit
	// Unavailable says that TMDB could not answer: the title stays free (spec §6).
	Unavailable bool
}

// Wanted is a title folder that the import would register, and so the lookups it needs (RF-29).
type Wanted struct {
	// Folder is the title's folder inside the library, as the registry keeps it ("Movies/Alien Romulus (2024)").
	Folder string
	Kind   TitleKind
	Title  string
	Year   int
	// TMDBID and IMDbID are the folder's id tags; 0 and "" when it has none.
	TMDBID int64
	IMDbID string
}

// ImportInput is what the import reads: the disk, the registry, the readings of the files, what TMDB
// answered, and the readings the last scan kept of the files it waited for (RF-28).
type ImportInput struct {
	Roots  []DiskRoot
	Titles []RegisteredTitle
	Read   ReadName
	// Archives holds the non-media files that are archives, by library path in lower case (RF-27a).
	Archives map[string]bool
	// Seen holds the stamp each file had when the last scan read it, by library path in lower case.
	Seen map[string]Stamp
	// SettledFolders are folders whose files passed RF-28a's event-driven stability window.
	SettledFolders map[string]bool
	// Lookups holds what TMDB answered, by the folder's library path in lower case.
	Lookups map[string]Lookup
	Media   MediaExts
	Now     time.Time
}

// NewTitle is a title that the import registers in place, with its files in order (a video, then its
// subtitles).
type NewTitle struct {
	Title LibraryTitle
	Files []LibraryFile
}

// AddFiles adds files that the import registers to a title already in the registry.
type AddFiles struct {
	TitleID int64
	Files   []LibraryFile
}

// MoveFile is a file that the import moves into No asignados: From is its library path, Rel its place inside
// the entry ("" for a loose file, whose entry is the file itself).
type MoveFile struct {
	From string
	Rel  string
	Size int64
}

// UnassignedMove is what the import moves into one entry of No asignados: a folder of the library with the files that
// it cannot read (RF-29), or a loose file. Source is the key of the entry (see UnassignedEntry.Source).
type UnassignedMove struct {
	Source string
	Name   string
	Loose  bool
	Files  []MoveFile
}

// SeenFile is the stamp of a file that a scan waits for (RF-28): it is read again by the next scan.
type SeenFile struct {
	Rel   string
	Stamp Stamp
}

// ImportCounts says what a plan does, for the log.
type ImportCounts struct {
	Titles, Files, Moved, Gone, Waiting int
}

// ImportPlan is what a scan changes. The application performs it as one change of the library (RNF-04):
// first the registry forgets the files and titles that are gone (subtitles before their videos), then the
// titles and files are registered, then the moves into No asignados run and the folders they empty go, and
// the waiting files are stored. Wanted is the lookups the plan needs, for the first pass.
type ImportPlan struct {
	Wanted     []Wanted
	GoneFiles  []int64
	GoneTitles []int64
	NewTitles  []NewTitle
	AddFiles   []AddFiles
	Moves      []UnassignedMove
	// RemoveDirs are the folders of the library that the moves emptied, deepest first.
	RemoveDirs []string
	// Waiting holds the stamps of the files still waiting; it replaces the stamps of the last scan.
	Waiting []SeenFile
	Counts  ImportCounts
}

// PlanImport decides what the library must become (RF-29). It is deterministic: the same input gives the same
// plan, and a second run over a library the first one left in order gives an empty plan (idempotence).
func PlanImport(in ImportInput) ImportPlan {
	p := &importPlanner{
		in:      in,
		roots:   map[string]ImportRoot{},
		rows:    map[string][]LibraryFile{},
		titles:  map[string]RegisteredTitle{},
		liveRel: map[string]bool{},
		allRel:  map[string]bool{},
	}
	for _, r := range in.Roots {
		p.roots[strings.ToLower(r.Root.Name)] = r.Root
	}
	p.registry()
	for _, r := range in.Roots {
		for _, f := range r.Entries {
			if f.Loose {
				p.looseFile(r.Root, f)
				continue
			}
			p.folder(r.Root, f)
		}
	}
	return p.plan
}

// importPlanner holds the state of one PlanImport run.
type importPlanner struct {
	in   ImportInput
	plan ImportPlan
	// roots are the configured roots, by name in lower case.
	roots map[string]ImportRoot
	// rows are the rows of each title that stays, by its folder in lower case, trashed ones included.
	rows map[string][]LibraryFile
	// titles are the titles that stay, by folder in lower case.
	titles map[string]RegisteredTitle
	// liveRel holds the paths of the files in the library and not in the trash, and allRel every path the
	// registry holds (trashed files too), both in lower case.
	liveRel map[string]bool
	allRel  map[string]bool
}

// candidate is a media file of a folder that the import may register or move.
type candidate struct {
	rel     string // inside its folder; "" for a loose file
	lib     string // its path in the library
	size    int64
	mtime   time.Time
	kind    FileKind
	archive bool
	read    FileReading
	// ok is set when the file can be registered; the fields below describe it then.
	ok      bool
	place   Place
	version string
	// labelTitle and label are the chapter title and the version label that the name gives (§5).
	labelTitle string
	label      string
	key        string
	// parent is the video of a subtitle: its library path, "" for none.
	parent string
}

// registry brings the registry in line with the disk for the titles of the roots (RF-29): the files that
// are gone leave, and so do their subtitles (the registry deletes them with their video), and a title that
// keeps no row leaves too.
func (p *importPlanner) registry() {
	disk := map[string]bool{}
	for _, r := range p.in.Roots {
		for _, f := range r.Entries {
			if f.Loose {
				disk[strings.ToLower(r.Root.Name+"/"+f.Name)] = true
				continue
			}
			for _, it := range f.Items {
				if !it.IsDir {
					disk[strings.ToLower(r.Root.Name+"/"+f.Name+"/"+it.Rel)] = true
				}
			}
		}
	}
	for _, rt := range p.in.Titles {
		root, _, ok := strings.Cut(rt.Title.Folder, "/")
		if !ok {
			continue
		}
		if _, ours := p.roots[strings.ToLower(root)]; !ours {
			continue
		}
		p.regTitle(rt, disk)
	}
}

// regTitle works out the rows of one title that the disk no longer holds.
func (p *importPlanner) regTitle(rt RegisteredTitle, disk map[string]bool) {
	gone := map[int64]bool{}
	goneVideo := map[string]bool{}
	for _, f := range rt.Files {
		if f.Kind == FileVideo && f.Live() && !disk[strings.ToLower(f.Rel)] {
			gone[f.ID] = true
			goneVideo[strings.ToLower(f.Rel)] = true
		}
	}
	for _, f := range rt.Files {
		if f.Kind == FileSubtitle && (goneVideo[strings.ToLower(f.VideoRel)] || (f.Live() && !disk[strings.ToLower(f.Rel)])) {
			gone[f.ID] = true
		}
	}
	// The subtitles go before their videos (the registry deletes them with the video anyway).
	for _, f := range rt.Files {
		if gone[f.ID] && f.Kind == FileSubtitle {
			p.plan.GoneFiles = append(p.plan.GoneFiles, f.ID)
		}
	}
	for _, f := range rt.Files {
		if gone[f.ID] && f.Kind == FileVideo {
			p.plan.GoneFiles = append(p.plan.GoneFiles, f.ID)
		}
	}
	p.plan.Counts.Gone += len(gone)

	var rows []LibraryFile
	for _, f := range rt.Files {
		if gone[f.ID] {
			continue
		}
		rows = append(rows, f)
		if f.Live() {
			p.liveRel[strings.ToLower(f.Rel)] = true
		}
		p.allRel[strings.ToLower(f.Rel)] = true
	}
	if len(rows) == 0 {
		p.plan.GoneTitles = append(p.plan.GoneTitles, rt.Title.ID)
		return
	}
	key := strings.ToLower(rt.Title.Folder)
	p.rows[key] = rows
	p.titles[key] = rt
}

// looseFile decides a file that lies directly in a root: it is no title, so an archive or a media file goes
// to No asignados as an entry of its own (RF-29).
func (p *importPlanner) looseFile(root ImportRoot, f Folder) {
	lib := root.Name + "/" + f.Name
	if p.liveRel[strings.ToLower(lib)] || len(f.Items) == 0 {
		return
	}
	c := p.candidateOf(f.Name, lib, f.Items[0])
	if c == nil {
		return
	}
	c.rel = ""
	if p.waiting([]*candidate{c}, lib) {
		return
	}
	p.plan.Moves = append(p.plan.Moves, UnassignedMove{
		Source: "library:" + lib, Name: f.Name, Loose: true,
		Files: []MoveFile{{From: lib, Size: c.size}},
	})
	p.plan.Counts.Moved++
}

// folder decides the media of one title folder (RF-29).
func (p *importPlanner) folder(root ImportRoot, f Folder) {
	folderLib := root.Name + "/" + f.Name
	var cands []*candidate
	for _, it := range f.Items {
		if it.IsDir || Ignored(it.Rel) {
			continue
		}
		lib := folderLib + "/" + it.Rel
		if p.liveRel[strings.ToLower(lib)] {
			continue
		}
		if c := p.candidateOf(it.Rel, lib, it); c != nil {
			cands = append(cands, c)
		}
	}
	if len(cands) == 0 {
		return
	}
	slices.SortFunc(cands, func(a, b *candidate) int { return strings.Compare(a.rel, b.rel) })
	if p.waiting(cands, folderLib) {
		return
	}
	p.place(root, f, folderLib, cands)
}

// candidateOf returns the file as a candidate when it is a video, a subtitle or an archive; nil otherwise.
func (p *importPlanner) candidateOf(rel, lib string, it Item) *candidate {
	c := &candidate{rel: rel, lib: lib, size: it.Size, mtime: it.ModTime}
	switch {
	case hasExtension(rel, p.in.Media.Video):
		c.kind = FileVideo
	case hasExtension(rel, p.in.Media.Subtitle):
		c.kind = FileSubtitle
	case p.in.Archives[strings.ToLower(lib)]:
		c.archive = true
	default:
		return nil
	}
	return c
}

// waiting reports whether the files of a folder are still being copied (RF-28): when one of them changed
// since the last scan or was modified less than a minute ago, the whole folder waits for a later scan, and
// its stamps are kept.

func (p *importPlanner) waiting(cands []*candidate, folder string) bool {
	if p.in.SettledFolders[strings.ToLower(folder)] {
		return false
	}
	copying := false
	for _, c := range cands {
		var prev *Stamp
		if s, ok := p.in.Seen[strings.ToLower(c.lib)]; ok {
			prev = &s
		}
		if Copying(prev, Stamp{Size: c.size, ModTime: c.mtime}, p.in.Now) {
			copying = true
		}
	}
	if !copying {
		return false
	}
	for _, c := range cands {
		p.plan.Waiting = append(p.plan.Waiting, SeenFile{Rel: c.lib, Stamp: Stamp{Size: c.size, ModTime: c.mtime}})
	}
	p.plan.Counts.Waiting++
	return true
}

// place registers the files of a title folder that can be read and moves the rest (RF-29).
func (p *importPlanner) place(root ImportRoot, f Folder, folderLib string, cands []*candidate) {
	fn := ParseFolderName(f.Name)
	reg, registered := p.titles[strings.ToLower(folderLib)]
	if fn.Title == "" || (registered && reg.Title.Kind != root.Kind) {
		p.moveUnaccepted(f, folderLib, cands)
		return
	}
	rows := p.rows[strings.ToLower(folderLib)]

	var videos, subs []*candidate
	for _, c := range cands {
		switch {
		case c.archive:
		case c.kind == FileVideo:
			videos = append(videos, c)
		case c.kind == FileSubtitle:
			subs = append(subs, c)
		}
	}
	for _, v := range videos {
		v.read = p.in.Read(v.rel, false)
		if fn.TMDBID == 0 {
			fn.TMDBID = v.read.TMDBID
		}
		if fn.IMDbID == "" {
			fn.IMDbID = v.read.IMDbID
		}
		v.ok = p.videoPlacement(root.Kind, fn, v) && !p.allRel[strings.ToLower(v.lib)]
	}
	if root.Kind == KindMovie {
		resolveMovies(videos, rows)
	} else {
		resolveEpisodes(videos, rows)
	}
	var accepted []*candidate
	for _, v := range videos {
		if v.ok {
			accepted = append(accepted, v)
		}
	}
	for _, s := range subs {
		s.read = p.in.Read(s.rel, true)
	}
	assignSubtitles(subs, accepted, rows)

	if !registered && len(accepted) == 0 {
		p.moveUnaccepted(f, folderLib, cands)
		return
	}
	files := p.filesOf(root, accepted, subs)
	if registered {
		if len(files) > 0 {
			p.plan.AddFiles = append(p.plan.AddFiles, AddFiles{TitleID: reg.Title.ID, Files: files})
			p.plan.Counts.Files += len(files)
		}
		p.moveUnaccepted(f, folderLib, cands)
		return
	}
	p.wantTitle(root, fn, folderLib)
	t, ok := p.newTitle(root, fn, folderLib)
	if !ok {
		p.moveAll(f, folderLib, cands)
		return
	}
	p.plan.NewTitles = append(p.plan.NewTitles, NewTitle{Title: t, Files: files})
	p.plan.Counts.Titles++
	p.plan.Counts.Files += len(files)
	p.moveUnaccepted(f, folderLib, cands)
}

// videoPlacement reads the place and the version of a video of a title of kind (RF-22, RF-24, §5). It
// returns false when the name does not say where the video goes, and the file is then not registered.
func (p *importPlanner) videoPlacement(kind TitleKind, fn FolderName, c *candidate) bool {
	if kind == KindMovie {
		return movieVideo(fn, c)
	}
	return episodeVideo(c)
}

// movieVideo places a movie video. A video directly in the title folder is the movie, or one of its parts or
// versions when its name carries the folder's name and a «- …» suffix (RF-22, RF-29).
func movieVideo(fn FolderName, c *candidate) bool {
	if strings.Contains(c.rel, "/") {
		return false
	}
	clean := strings.TrimSpace(idTagPattern.ReplaceAllString(fileStem(c.rel), " "))
	suffix, _ := movieNameSuffix(fn, clean)
	part, version := 0, ""
	if suffix != "" {
		if n := partNumberOf(suffix); n > 0 {
			part = n
		} else {
			version = suffix
		}
	}
	if version != "" && !plainName(version) {
		return false
	}
	c.place = Place{Part: part}
	c.version = version
	c.key = movieKey(part, version)
	return true
}

// movieNameSuffix returns the suffix that follows the folder's title in a movie's name, after " - ", and whether
// the name starts with that title at all. A name that is the title alone has an empty suffix.
func movieNameSuffix(fn FolderName, clean string) (string, bool) {
	heads := []string{fn.Title}
	if fn.Year > 0 {
		heads = []string{fn.Title + " (" + strconv.Itoa(fn.Year) + ")", fn.Title}
	}
	for _, head := range heads {
		if strings.EqualFold(clean, head) {
			return "", true
		}
		if len(clean) > len(head)+3 && strings.EqualFold(clean[:len(head)], head) && clean[len(head):len(head)+3] == " - " {
			return strings.TrimSpace(clean[len(head)+3:]), true
		}
	}
	return "", false
}

// movieKey is the place of a movie file: the whole movie, one of its parts, or one of its versions (RF-22).
func movieKey(part int, version string) string {
	switch {
	case part > 0:
		return "part:" + strconv.Itoa(part)
	case version != "":
		return "version:" + strings.ToLower(version)
	default:
		return "whole"
	}
}

// resolveMovies decides which movie videos are registered (RF-22, RF-24, RF-29): two files with the same place
// are not told apart, so both stay out; a part and a whole movie or a version do not mix; and a place the
// registry already holds is taken.
func resolveMovies(videos []*candidate, rows []LibraryFile) {
	occupied := map[string]bool{}
	occPart, occWhole := false, false
	for _, r := range rows {
		if r.Kind != FileVideo {
			continue
		}
		occupied[movieKey(r.Place.Part, r.Version)] = true
		if r.Place.Part > 0 {
			occPart = true
		} else {
			occWhole = true
		}
	}
	counts := map[string]int{}
	candPart, candWhole := false, false
	for _, v := range videos {
		if !v.ok {
			continue
		}
		counts[v.key]++
		if v.place.Part > 0 {
			candPart = true
		} else {
			candWhole = true
		}
	}
	if candPart && candWhole {
		// A part and a whole movie in one folder: which one is the movie is not told, so none is kept (RF-22).
		for _, v := range videos {
			v.ok = false
		}
		return
	}
	accPart, accWhole := false, false
	accepted := map[string]bool{}
	for _, v := range videos {
		if !v.ok {
			continue
		}
		isPart := v.place.Part > 0
		switch {
		case counts[v.key] > 1, occupied[v.key], accepted[v.key]:
			v.ok = false
		case isPart && (occWhole || accWhole), !isPart && (occPart || accPart):
			v.ok = false
		}
		if v.ok {
			accepted[v.key] = true
			if isPart {
				accPart = true
			} else {
				accWhole = true
			}
		}
	}
}

// episodeVideo places a chapter: a file directly in the series folder or in its season folder, with the season
// and the chapters its name gives (§5, RF-22). A name with no chapter is not a chapter.
func episodeVideo(c *candidate) bool {
	parts := strings.Split(c.rel, "/")
	switch {
	case len(parts) == 1:
	case len(parts) == 2 && isSeasonFolder(parts[0]):
	default:
		return false
	}
	r := c.read
	if r.Season == nil || r.Chapter == nil {
		return false
	}
	first := *r.Chapter
	last := first
	if r.Last != nil {
		last = *r.Last
	}
	if first < 1 || last < first || *r.Season < 0 {
		return false
	}
	c.place = Place{Episode: true, Season: *r.Season, First: first, Last: last}
	c.labelTitle, c.label = chapterLabelOf(fileStem(path.Base(c.rel)))
	return true
}

// resolveEpisodes decides which chapter videos are registered (RF-29). Files that overlap in a season form a
// group with the chapters the registry holds too. A group of one file is that chapter's file. A group of more
// is a set of versions only when every file of it carries a version label (§5, «Capítulo, versión»: the name
// after the marker is «<título> - <versión>»), all have the same chapter title and the labels differ; otherwise
// none of its candidates is registered and they go to No asignados.
func resolveEpisodes(videos []*candidate, rows []LibraryFile) {
	var items []chapterItem
	for _, v := range videos {
		if v.ok {
			items = append(items, chapterItem{place: v.place, title: v.labelTitle, version: v.label, cand: v})
		}
	}
	for _, r := range rows {
		if r.Kind == FileVideo && r.Place.Episode {
			title, _ := chapterLabelOf(fileStem(path.Base(r.Rel)))
			items = append(items, chapterItem{place: r.Place, title: title, version: r.Version})
		}
	}
	seen := make([]bool, len(items))
	for i := range items {
		if seen[i] {
			continue
		}
		group := []int{i}
		seen[i] = true
		for k := 0; k < len(group); k++ {
			for j := range items {
				if !seen[j] && overlaps(items[group[k]].place, items[j].place) {
					seen[j] = true
					group = append(group, j)
				}
			}
		}
		settleChapterGroup(items, group)
	}
}

// chapterItem is a file that competes for chapters: a candidate of this change, or a video the registry holds.
type chapterItem struct {
	place   Place
	title   string
	version string
	cand    *candidate // nil for a file the registry holds
}

// settleChapterGroup decides the candidates of one group of overlapping chapter files (see resolveEpisodes).
func settleChapterGroup(items []chapterItem, group []int) {
	if len(group) == 1 {
		if c := items[group[0]].cand; c != nil {
			c.version = ""
		}
		return
	}
	valid := true
	title := titleKey(items[group[0]].title)
	labels := map[string]bool{}
	for _, idx := range group {
		it := items[idx]
		key := strings.ToLower(it.version)
		if it.version == "" || titleKey(it.title) != title || labels[key] {
			valid = false
		}
		labels[key] = true
	}
	for _, idx := range group {
		it := items[idx]
		if it.cand == nil {
			continue
		}
		it.cand.ok = valid
		if valid {
			it.cand.version = it.version
		}
	}
}

// chapterMarker is the marker of a chapter in a file name: SxxEyy, SxxEyyEzz or SxxEy-Ezz.
var chapterMarker = regexp.MustCompile(`(?i)S\d{1,3}E\d{1,3}(?:-?E?\d{1,3})?`)

// chapterLabelOf reads the version label of a chapter file name (§5, «Capítulo, versión»): the name after the
// chapter marker is «<título> - <versión>», or just «- <versión>» when the chapter has no title. It returns the
// title part ("" when there is none) and the version; the version is "" when the name has no such suffix.
func chapterLabelOf(stem string) (title, version string) {
	loc := chapterMarker.FindStringIndex(stem)
	if loc == nil {
		return "", ""
	}
	rest := stem[loc[1]:]
	if !strings.HasPrefix(rest, " - ") {
		return "", ""
	}
	segments := strings.Split(rest[len(" - "):], " - ")
	version = strings.TrimSpace(segments[len(segments)-1])
	if version == "" || !plainName(version) {
		return "", ""
	}
	return strings.TrimSpace(strings.Join(segments[:len(segments)-1], " - ")), version
}

// overlaps reports whether two chapter places share a chapter of the same season.
func overlaps(a, b Place) bool {
	return a.Season == b.Season && a.First <= b.Last && b.First <= a.Last
}

// ImportedSubtitleLanguage is the language of a subtitle whose name gives none: the usual language of the
// library (RF-23, decision 2026-10-14). The user may change it later.
const ImportedSubtitleLanguage = "es-419"

// subtitleLanguageOf returns the language a subtitle is registered with (RF-29).
func subtitleLanguageOf(r FileReading) string {
	if r.Language == "" {
		return ImportedSubtitleLanguage
	}
	return r.Language
}

// assignSubtitles pairs each subtitle with the video whose name its own name starts with (the longest one,
// RF-23, RF-29): an accepted video of this change, or a video the registry holds. A subtitle with no video is
// not registered; one with no language in its name is registered as ImportedSubtitleLanguage (RF-29).
func assignSubtitles(subs, accepted []*candidate, rows []LibraryFile) {
	type parent struct {
		stem string
		lib  string
	}
	var parents []parent
	for _, v := range accepted {
		parents = append(parents, parent{stem: strings.ToLower(fileStem(v.rel)), lib: v.lib})
	}
	for _, r := range rows {
		if r.Kind == FileVideo && r.Live() {
			parents = append(parents, parent{stem: strings.ToLower(fileStem(path.Base(r.Rel))), lib: r.Rel})
		}
	}
	for _, s := range subs {
		s.ok = false
		name := strings.ToLower(fileStem(s.rel))
		best := -1
		for i, pa := range parents {
			if (name == pa.stem || strings.HasPrefix(name, pa.stem+".")) && (best < 0 || len(pa.stem) > len(parents[best].stem)) {
				best = i
			}
		}
		if best >= 0 {
			s.ok = true
			s.parent = parents[best].lib
		}
	}
}

// filesOf returns the registry rows of the accepted videos, each followed by its accepted subtitles, in the
// order they are stored (a subtitle's video comes first, RF-23). A subtitle of a video the registry already
// holds comes last.
func (p *importPlanner) filesOf(root ImportRoot, videos, subs []*candidate) []LibraryFile {
	var files []LibraryFile
	candidates := map[string]bool{}
	stored := map[string]bool{}
	for _, v := range videos {
		candidates[v.lib] = true
		if !v.ok {
			continue
		}
		vf, err := NewVideo(root.Kind, VideoSpec{Rel: v.lib, Place: v.place, Version: v.version, Size: v.size}, p.in.Now)
		if err != nil {
			v.ok = false
			continue
		}
		files = append(files, vf)
		stored[v.lib] = true
		files = p.appendSubtitles(files, v.lib, subs)
	}
	// A subtitle of a video that this change does not store stays out with it (it moves to No asignados).
	for _, s := range subs {
		if s.ok && candidates[s.parent] && !stored[s.parent] {
			s.ok = false
		}
	}
	// The subtitles of videos the registry already holds follow, once per video.
	for _, s := range subs {
		if s.ok && !candidates[s.parent] && !stored[s.parent] {
			files = p.appendSubtitles(files, s.parent, subs)
			stored[s.parent] = true
		}
	}
	return files
}

// appendSubtitles adds the accepted subtitles that go with the video at videoLib.
func (p *importPlanner) appendSubtitles(files []LibraryFile, videoLib string, subs []*candidate) []LibraryFile {
	for _, s := range subs {
		if !s.ok || s.parent != videoLib {
			continue
		}
		sf, err := NewSubtitle(SubtitleSpec{
			Rel: s.lib, VideoRel: videoLib, Language: subtitleLanguageOf(s.read), Forced: s.read.Forced, SDH: s.read.SDH, Size: s.size,
		}, p.in.Now)
		if err == nil {
			files = append(files, sf)
		}
	}
	return files
}

// wantTitle records that a folder that is not registered needs its lookups (RF-29).
func (p *importPlanner) wantTitle(root ImportRoot, fn FolderName, folderLib string) {
	p.plan.Wanted = append(p.plan.Wanted, Wanted{
		Folder: folderLib, Kind: root.Kind, Title: fn.Title, Year: fn.Year, TMDBID: fn.TMDBID, IMDbID: fn.IMDbID,
	})
}

// newTitle builds the registry title of a folder that is not registered, linked to TMDB when a lookup gives a
// single title (RF-29). A title with no link is free («Sin vincular»): it keeps the folder's name and year.
func (p *importPlanner) newTitle(root ImportRoot, fn FolderName, folderLib string) (LibraryTitle, bool) {
	look, has := p.in.Lookups[strings.ToLower(folderLib)]
	l := linkOf(fn, look, has)
	title := l.title
	if title == "" {
		title = fn.Title
	}
	idTag := fn.JellyfinID != 0 && l.tmdbID == fn.JellyfinID
	t, err := NewLibraryTitle(root.Kind, title, fn.Title, fn.Year, l.tmdbID, idTag, folderLib, p.in.Now)
	return t, err == nil
}

// link is the TMDB title that a folder is linked to.
type link struct {
	tmdbID int64
	title  string
}

// linkOf chooses the TMDB title of a folder (RF-29): the folder's TMDB id, else the one title its IMDb id
// links to. A folder with an id tag that TMDB does not know stays free («Sin vincular»): the tag states the
// intention, so the title is not searched by its name. A folder with no tag is linked to the one title that
// TMDB finds with the folder's title and year. Any other answer leaves the folder free.
func linkOf(fn FolderName, look Lookup, has bool) link {
	if !has || look.Unavailable {
		return link{}
	}
	if look.ByID != nil {
		return link{tmdbID: look.ByID.ID, title: look.ByID.Title}
	}
	if len(look.ByIMDb) == 1 {
		return link{tmdbID: look.ByIMDb[0].ID, title: look.ByIMDb[0].Title}
	}
	if fn.TMDBID != 0 || fn.IMDbID != "" {
		return link{}
	}
	var found []TMDBHit
	for _, h := range look.Search {
		if !sameTitleOf(fn.Title, h) || (fn.Year > 0 && h.Year != fn.Year) {
			continue
		}
		found = append(found, h)
	}
	if len(found) == 1 {
		return link{tmdbID: found[0].ID, title: found[0].Title}
	}
	return link{}
}

// sameTitleOf reports whether a TMDB title is the folder's title, in either of its names (RF-29).
func sameTitleOf(title string, h TMDBHit) bool {
	key := titleKey(title)
	return titleKey(h.Title) == key || (h.OriginalTitle != "" && titleKey(h.OriginalTitle) == key)
}

// moveAll moves every candidate of a folder into No asignados: the folder is no title, or its files are not
// registered after all.
func (p *importPlanner) moveAll(f Folder, folderLib string, cands []*candidate) {
	for _, c := range cands {
		c.ok = false
	}
	p.moveUnaccepted(f, folderLib, cands)
}

// moveUnaccepted moves into No asignados the candidates of a folder that are not registered: one entry per
// folder, with the paths inside it, and the folders that the moves empty (RF-29).
func (p *importPlanner) moveUnaccepted(f Folder, folderLib string, cands []*candidate) {
	var files []MoveFile
	moved := map[string]bool{}
	for _, c := range cands {
		if c.ok {
			continue
		}
		files = append(files, MoveFile{From: c.lib, Rel: c.rel, Size: c.size})
		moved[c.rel] = true
	}
	if len(files) == 0 {
		return
	}
	p.plan.Moves = append(p.plan.Moves, UnassignedMove{Source: "library:" + folderLib, Name: f.Name, Files: files})
	p.plan.Counts.Moved += len(files)
	p.plan.RemoveDirs = append(p.plan.RemoveDirs, emptied(folderLib, f.Items, moved)...)
}

// emptied returns the folders of a title folder that the moves empty, deepest first, and the folder itself
// when everything in it goes (RF-29). A folder with junk or a file that stays is kept, and so is one that
// nothing was moved from.
func emptied(folderLib string, items []Item, moved map[string]bool) []string {
	children := map[string][]Item{}
	for _, it := range items {
		parent := parentOf(it.Rel)
		children[parent] = append(children[parent], it)
	}
	removed := map[string]bool{}
	var dirs []string
	var visit func(rel string) bool
	visit = func(rel string) bool {
		movedBelow := false
		allGone := true
		for _, ch := range children[rel] {
			switch {
			case moved[ch.Rel]:
				movedBelow = true
				removed[ch.Rel] = true
			case ch.IsDir:
				if visit(ch.Rel) {
					movedBelow = true
				}
				if !removed[ch.Rel] {
					allGone = false
				}
			default:
				allGone = false
			}
		}
		if movedBelow && allGone {
			removed[rel] = true
			if rel != "" {
				dirs = append(dirs, folderLib+"/"+rel)
			}
		}
		return movedBelow
	}
	visit("")
	if removed[""] {
		dirs = append(dirs, folderLib)
	}
	return dirs
}

// fileStem returns the name of a file without its last extension.
func fileStem(rel string) string {
	name := path.Base(rel)
	if i := strings.LastIndexByte(name, '.'); i > 0 {
		return name[:i]
	}
	return name
}

// hasExtension reports whether the name ends with one of the extensions (with their dot, in any case).
func hasExtension(name string, exts []string) bool {
	ext := strings.ToLower(path.Ext(name))
	return ext != "" && slices.Contains(exts, ext)
}
