package application

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/catalog/domain"
)

// The library scan (spec RF-28, RF-29) reads Movies/ and TV Shows/ as the disk holds them now, decides what
// the registry must hold (domain.PlanImport) and performs it as one change of the library (RNF-04): the
// titles and files that are gone leave the registry, what Jellyfin recognizes is registered in place, what
// cannot be read goes to No asignados, and the files that are still being copied wait for the next scan.

// SeenStore reads the readings that the last library scan kept of the files it waits for (RF-28).
type SeenStore interface {
	Seen(ctx context.Context) (map[string]domain.Stamp, error)
}

// ArchiveProbe tells whether a file of the library is a compressed archive, from its first bytes (RF-27a).
// The path is the file's path on the disk; a link is never followed.
type ArchiveProbe interface {
	IsArchive(path string) (bool, error)
}

// ImportTMDB is what the library scan asks of TMDB (RF-29): what Assign asks, and the titles that link to an
// IMDb id (TMDB's /find).
type ImportTMDB interface {
	TMDB
	// FindIMDb returns the titles of kind that TMDB links to an IMDb id. An id TMDB does not know gives none.
	FindIMDb(ctx context.Context, kind domain.TitleKind, imdbID string) ([]TMDBTitle, error)
}

// Asider plans a move into No asignados inside a change that the caller performs (see Unassigned.MoveInto).
type Asider interface {
	MoveInto(ctx context.Context, b *Builder, r AsideRequest) (string, error)
}

// ImportDeps are what the library scan needs (RF-29).
type ImportDeps struct {
	Files   Files
	Engine  *Engine
	Library LibraryRepository
	Seen    SeenStore
	Aside   Asider
	// Archives tells the archives among the files that are not media (RF-27a).
	Archives ArchiveProbe
	TMDB     ImportTMDB
	// Names reads what a file says about itself (RF-20, RF-23): rel is inside its title folder.
	Names func(rel string, subtitle bool) domain.FileReading
	// Roots are MOVIES_DIR and SERIES_DIR.
	Roots Roots
	Media domain.MediaExts
	// Now and Log may be nil.
	Now func() time.Time
	Log *slog.Logger
}

// LibraryImport scans Movies/ and TV Shows/ (RF-29). It is the library scan of the scanner (LibraryScan).
type LibraryImport struct {
	d  ImportDeps
	mu sync.Mutex
}

var _ LibraryScan = (*LibraryImport)(nil)

// NewLibraryImport checks the dependencies and builds the scan.
func NewLibraryImport(d ImportDeps) (*LibraryImport, error) {
	switch {
	case d.Files == nil || d.Engine == nil || d.Library == nil || d.Seen == nil || d.Aside == nil:
		return nil, errors.New("catalog: the library scan needs files, engine, registry, waiting files and No asignados")
	case d.Archives == nil || d.TMDB == nil || d.Names == nil:
		return nil, errors.New("catalog: the library scan needs archives, TMDB and the names")
	case d.Roots.Movies == "" || d.Roots.Series == "" || d.Roots.Movies == d.Roots.Series:
		return nil, errors.New("catalog: the library scan needs two different roots")
	}
	if d.Now == nil {
		d.Now = time.Now
	}
	if d.Log == nil {
		d.Log = slog.New(slog.DiscardHandler)
	}
	return &LibraryImport{d: d}, nil
}

// Scan implements LibraryScan. The TMDB lookups happen before the change, outside the library lock: the
// first pass says which folders need them, the second decides under the lock with the answers in hand.
func (im *LibraryImport) Scan(ctx context.Context) error {
	im.mu.Lock()
	defer im.mu.Unlock()
	return im.scan(ctx, "")
}

// ScanTitle imports or forgets only one title folder, reusing the RF-29 planner and operation engine.
func (im *LibraryImport) ScanTitle(ctx context.Context, folder string) error {
	if folder == "" || strings.Contains(folder, "..") || strings.HasPrefix(folder, "/") {
		return errors.New("catalog: invalid title folder")
	}
	im.mu.Lock()
	defer im.mu.Unlock()
	return im.scan(ctx, folder)
}

func (im *LibraryImport) scan(ctx context.Context, folder string) error {
	var wanted []domain.Wanted
	err := im.d.Engine.Locked(func() error {
		in, err := im.snapshot(ctx)
		if err != nil {
			return err
		}
		if folder != "" {
			in = scopeImport(in, folder)
		}
		wanted = domain.PlanImport(in).Wanted
		return nil
	})
	if err != nil {
		return err
	}
	lookups := im.lookups(ctx, wanted)

	return im.d.Engine.Do(ctx, "library.scan", "library", func(b *Builder) error {
		in, err := im.snapshot(ctx)
		if err != nil {
			return err
		}
		allSeen := in.Seen
		if folder != "" {
			in = scopeImport(in, folder)
		}
		in.Lookups = lookups
		plan := domain.PlanImport(in)
		if folder != "" {
			plan.Waiting = mergeScopedWaiting(allSeen, plan.Waiting, folder)
		}
		return im.apply(ctx, b, in, plan)
	})
}

func mergeScopedWaiting(previous map[string]domain.Stamp, current []domain.SeenFile, folder string) []domain.SeenFile {
	prefix := strings.ToLower(strings.Trim(folder, "/")) + "/"
	out := make([]domain.SeenFile, 0, len(previous)+len(current))
	for rel, stamp := range previous {
		if !strings.HasPrefix(strings.ToLower(rel), prefix) {
			out = append(out, domain.SeenFile{Rel: rel, Stamp: stamp})
		}
	}
	out = append(out, current...)
	slices.SortFunc(out, func(a, b domain.SeenFile) int { return strings.Compare(strings.ToLower(a.Rel), strings.ToLower(b.Rel)) })
	return out
}

func scopeImport(in domain.ImportInput, folder string) domain.ImportInput {
	key := strings.ToLower(strings.Trim(folder, "/"))
	in.SettledFolders = map[string]bool{key: true}
	for i := range in.Roots {
		root := &in.Roots[i]
		root.Entries = slices.DeleteFunc(root.Entries, func(entry domain.Folder) bool {
			return entry.Loose || !strings.EqualFold(root.Root.Name+"/"+entry.Name, key)
		})
	}
	in.Titles = slices.DeleteFunc(in.Titles, func(title domain.RegisteredTitle) bool {
		return !strings.EqualFold(title.Title.Folder, key)
	})
	seen := make(map[string]domain.Stamp)
	for rel, stamp := range in.Seen {
		if strings.HasPrefix(strings.ToLower(rel), key+"/") {
			seen[rel] = stamp
		}
	}
	in.Seen = seen
	return in
}

// snapshot reads the library as it is now: the two roots, the registry, the waiting files and the archives
// among the files that are not media. It must run under the engine's lock.
func (im *LibraryImport) snapshot(ctx context.Context) (domain.ImportInput, error) {
	in := domain.ImportInput{Read: im.d.Names, Media: im.d.Media, Now: im.d.Now()}
	in.Archives = map[string]bool{}
	roots := []domain.ImportRoot{
		{Name: im.d.Roots.Movies, Kind: domain.KindMovie},
		{Name: im.d.Roots.Series, Kind: domain.KindSeries},
	}
	for _, root := range roots {
		tree, err := im.d.Files.Tree(im.d.Files.LibraryPath(root.Name))
		if err != nil {
			return in, fmt.Errorf("catalog: read %s: %w", root.Name, err)
		}
		entries := domain.Entries(tree)
		in.Roots = append(in.Roots, domain.DiskRoot{Root: root, Entries: entries})
		im.archivesIn(root, entries, in.Archives)
	}

	titles, err := im.d.Library.Titles(ctx)
	if err != nil {
		return in, err
	}
	for _, t := range titles {
		files, err := im.d.Library.Files(ctx, t.ID)
		if err != nil {
			return in, err
		}
		// A title whose only files are in the trash is not part of the disk's
		// import state. Keep the registry row itself: Assign uses it to answer
		// `in_trash` until the entry is restored or purged (RF-30, RF-31a).
		liveFiles := make([]domain.LibraryFile, 0, len(files))
		for _, f := range files {
			if f.TrashEntryID == 0 {
				liveFiles = append(liveFiles, f)
			}
		}
		if len(files) > 0 && len(liveFiles) == 0 {
			continue
		}
		in.Titles = append(in.Titles, domain.RegisteredTitle{Title: t, Files: liveFiles})
	}
	in.Seen, err = im.d.Seen.Seen(ctx)
	if err != nil {
		return in, err
	}
	return in, nil
}

// archivesIn marks the archives among the files of a root that are not media (RF-27a). A file that cannot be
// read is not an archive for this scan: it stays where it is, and the failure is logged.
func (im *LibraryImport) archivesIn(root domain.ImportRoot, entries []domain.Folder, out map[string]bool) {
	for _, f := range entries {
		if f.Loose {
			im.archiveAt(root, f.Name, out)
			continue
		}
		for _, it := range f.Items {
			if it.IsDir || domain.Ignored(it.Rel) || hasMedia(it.Rel, im.d.Media) {
				continue
			}
			im.archiveAt(root, f.Name+"/"+it.Rel, out)
		}
	}
}

// archiveAt probes one file, given its path inside the root.
func (im *LibraryImport) archiveAt(root domain.ImportRoot, rel string, out map[string]bool) {
	lib := root.Name + "/" + rel
	archive, err := im.d.Archives.IsArchive(im.d.Files.LibraryPath(strings.Split(lib, "/")...))
	if err != nil {
		im.d.Log.Warn("library file not read for the scan", "path", lib, "error", err)
		return
	}
	if archive {
		out[strings.ToLower(lib)] = true
	}
}

// hasMedia reports whether a file is a video or a subtitle by its extension.
func hasMedia(rel string, m domain.MediaExts) bool {
	lower := strings.ToLower(rel)
	for _, exts := range [][]string{m.Video, m.Subtitle} {
		for _, ext := range exts {
			if strings.HasSuffix(lower, ext) {
				return true
			}
		}
	}
	return false
}

// lookups asks TMDB about the folders that the plan would register (RF-29), in the order given. TMDB not
// configured gives no lookups: the titles stay free. An answer that fails leaves its folder free too.
func (im *LibraryImport) lookups(ctx context.Context, wanted []domain.Wanted) map[string]domain.Lookup {
	out := map[string]domain.Lookup{}
	if len(wanted) == 0 || !im.d.TMDB.Configured() {
		return out
	}
	for _, w := range wanted {
		out[strings.ToLower(w.Folder)] = im.lookupOf(ctx, w)
	}
	return out
}

// lookupOf asks TMDB about one folder: its TMDB id, its IMDb id, and else its title and year (RF-29).
func (im *LibraryImport) lookupOf(ctx context.Context, w domain.Wanted) domain.Lookup {
	var look domain.Lookup
	if w.TMDBID != 0 {
		t, err := im.d.TMDB.Title(ctx, w.Kind, w.TMDBID)
		switch {
		case err == nil:
			look.ByID = &domain.TMDBHit{ID: t.ID, Title: t.Title, OriginalTitle: t.OriginalTitle, Year: t.Year}
		case !errors.Is(err, ErrTMDBNotFound):
			look.Unavailable = true
			im.d.Log.WarnContext(ctx, "TMDB could not give a folder's title", "folder", w.Folder, "error", err)
		}
	}
	if w.IMDbID != "" && look.ByID == nil && !look.Unavailable {
		hits, err := im.d.TMDB.FindIMDb(ctx, w.Kind, w.IMDbID)
		switch {
		case err == nil:
			look.ByIMDb = hitsOf(hits)
		case !errors.Is(err, ErrTMDBNotFound):
			look.Unavailable = true
			im.d.Log.WarnContext(ctx, "TMDB could not find a folder's IMDb id", "folder", w.Folder, "error", err)
		}
	}
	// A folder with an id tag is not searched by its name (RF-29): the tag decides, and a tag TMDB does not know
	// leaves the title free.
	tagged := w.TMDBID != 0 || w.IMDbID != ""
	if look.ByID == nil && len(look.ByIMDb) == 0 && !look.Unavailable && !tagged {
		hits, err := im.d.TMDB.SearchTitles(ctx, w.Kind, w.Title, w.Year)
		switch {
		case err == nil:
			look.Search = hitsOf(hits)
		case !errors.Is(err, ErrTMDBNotFound):
			look.Unavailable = true
			im.d.Log.WarnContext(ctx, "TMDB search failed for a folder", "folder", w.Folder, "error", err)
		}
	}
	return look
}

// hitsOf converts TMDB's titles to the ones the plan reads.
func hitsOf(titles []TMDBTitle) []domain.TMDBHit {
	out := make([]domain.TMDBHit, 0, len(titles))
	for _, t := range titles {
		out = append(out, domain.TMDBHit{ID: t.ID, Title: t.Title, OriginalTitle: t.OriginalTitle, Year: t.Year})
	}
	return out
}

// apply records and performs a plan inside the change b: the registry forgets what is gone, registers what
// is recognized, the moves into No asignados run, the folders they empty go, and the waiting files are stored.
func (im *LibraryImport) apply(ctx context.Context, b *Builder, in domain.ImportInput, plan domain.ImportPlan) error {
	if len(plan.GoneFiles) > 0 || len(plan.GoneTitles) > 0 {
		b.Record(func(rec Records) error {
			for _, id := range plan.GoneFiles {
				if err := rec.DeleteLibraryFile(ctx, id); err != nil {
					return err
				}
			}
			for _, id := range plan.GoneTitles {
				if err := rec.DeleteLibraryTitle(ctx, id); err != nil {
					return err
				}
			}
			return nil
		})
	}
	for _, nt := range plan.NewTitles {
		b.Record(func(rec Records) error {
			id, err := rec.InsertLibraryTitle(ctx, nt.Title)
			if err != nil {
				return err
			}
			return rec.InsertLibraryFiles(ctx, id, nt.Files)
		})
	}
	for _, af := range plan.AddFiles {
		b.Record(func(rec Records) error {
			return rec.InsertLibraryFiles(ctx, af.TitleID, af.Files)
		})
	}

	planned := domain.NameSet{}
	for _, m := range plan.Moves {
		name, err := im.move(ctx, b, m, planned)
		if err != nil {
			return err
		}
		planned.Add(name)
	}
	for _, dir := range plan.RemoveDirs {
		b.RemoveDir(im.d.Files.LibraryPath(strings.Split(dir, "/")...))
	}

	if !sameWaiting(in.Seen, plan.Waiting) {
		seen := plan.Waiting
		b.Record(func(rec Records) error {
			return rec.ReplaceLibrarySeen(ctx, seen)
		})
	}
	im.logPlan(ctx, plan)
	return nil
}

// move plans one entry of No asignados for what the scan cannot read (RF-29). The names that earlier entries
// of the same change took are not free.
func (im *LibraryImport) move(ctx context.Context, b *Builder, m domain.UnassignedMove, planned domain.NameSet) (string, error) {
	req := AsideRequest{Source: m.Source, Name: m.Name, Loose: m.Loose, Origin: domain.OriginSamba, Taken: planned}
	for _, f := range m.Files {
		req.Files = append(req.Files, AsideFile{
			Source: im.d.Files.LibraryPath(strings.Split(f.From, "/")...), Rel: f.Rel, Size: f.Size,
		})
	}
	name, err := im.d.Aside.MoveInto(ctx, b, req)
	if err != nil {
		return "", fmt.Errorf("catalog: move %s to %s: %w", m.Source, domain.UnassignedDir, err)
	}
	return name, nil
}

// sameWaiting reports whether the waiting files are the ones the last scan stored, so that nothing is written.
func sameWaiting(prev map[string]domain.Stamp, now []domain.SeenFile) bool {
	if len(prev) != len(now) {
		return false
	}
	for _, f := range now {
		p, ok := prev[strings.ToLower(f.Rel)]
		if !ok || p.Size != f.Stamp.Size || p.ModTime.UnixMilli() != f.Stamp.ModTime.UnixMilli() {
			return false
		}
	}
	return true
}

// logPlan says what the scan did, when it did something.
func (im *LibraryImport) logPlan(ctx context.Context, p domain.ImportPlan) {
	c := p.Counts
	changed := c.Titles+c.Files+c.Moved+c.Gone > 0
	level := slog.LevelDebug
	if changed {
		level = slog.LevelInfo
	}
	im.d.Log.Log(ctx, level, "library scanned", "titles", c.Titles, "files", c.Files, "unassigned", c.Moved,
		"gone", c.Gone, "waiting", c.Waiting)
}
