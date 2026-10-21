package sqlite_test

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/catalog/application"
	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/catalog/domain"
	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/catalog/infrastructure/libraryfs"
	catalogsqlite "github.com/CarlosSV923/VideoExplorer/apps/api/internal/catalog/infrastructure/sqlite"
	ingestiondomain "github.com/CarlosSV923/VideoExplorer/apps/api/internal/ingestion/domain"
)

// importTMDB answers the lookups of the import from fixed tables. Configured says whether TMDB is set up.
type importTMDB struct {
	configured bool
	byID       map[string]application.TMDBTitle
	byIMDb     map[string][]application.TMDBTitle
	search     map[string][]application.TMDBTitle
}

func (f *importTMDB) Configured() bool { return f.configured }

func (f *importTMDB) SearchTitles(_ context.Context, kind domain.TitleKind, query string, _ int) ([]application.TMDBTitle, error) {
	return f.search[string(kind)+"|"+query], nil
}

func (f *importTMDB) Title(_ context.Context, kind domain.TitleKind, id int64) (application.TMDBTitle, error) {
	title, ok := f.byID[string(kind)+"|"+fmt.Sprint(id)]
	if !ok {
		return application.TMDBTitle{}, application.ErrTMDBNotFound
	}
	return title, nil
}

func TestScanLinksPreviouslyFreeTitleFromFileTMDBTagWithoutRenaming(t *testing.T) {
	t.Parallel()
	tmdb := &importTMDB{
		configured: true,
		byID: map[string]application.TMDBTitle{
			"movie|8077": {ID: 8077, Title: "Alien Resurrection", Year: 1997},
		},
	}
	fx := newImportFixture(t, tmdb)
	folder := "Movies/Alien Resurrection (1997)"
	rel := folder + "/Alien Resurrection (1997) [tmdbid-8077].mp4"
	fx.put(t, rel, []byte("video"), fx.settled())
	title, err := domain.NewLibraryTitle(domain.KindMovie, "Alien Resurrection", "Alien Resurrection", 1997, 0, false, folder, fx.now)
	if err != nil {
		t.Fatal(err)
	}
	if err := commitLibrary(t.Context(), fx.conn, func(rec application.Records) error {
		id, err := rec.InsertLibraryTitle(t.Context(), title)
		if err != nil {
			return err
		}
		return rec.InsertLibraryFiles(t.Context(), id, []domain.LibraryFile{{Kind: domain.FileVideo, Rel: rel, Size: 5}})
	}); err != nil {
		t.Fatal(err)
	}

	fx.scan(t)
	titles, err := fx.library.Titles(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(titles) != 1 || titles[0].TMDBID != 8077 || titles[0].Title != "Alien Resurrection" || titles[0].Folder != folder {
		t.Fatalf("linked title = %+v", titles)
	}
	if !fx.exists(rel) {
		t.Fatalf("scan renamed or moved the imported file %q", rel)
	}
}

func (f *importTMDB) FindIMDb(_ context.Context, kind domain.TitleKind, imdbID string) ([]application.TMDBTitle, error) {
	return f.byIMDb[string(kind)+"|"+imdbID], nil
}

func (f *importTMDB) Chapters(context.Context, int64, int, int, int) (application.TMDBChapters, error) {
	return application.TMDBChapters{}, application.ErrTMDBNotFound
}

// headerProbe tells the archives by their first bytes, with the ingestion's signatures.
type headerProbe struct {
	files *libraryfs.FS
}

func (p headerProbe) IsArchive(path string) (bool, error) {
	h, err := p.files.ReadHeader(path, 8)
	if err != nil {
		return false, err
	}
	_, ok := ingestiondomain.DetectArchive(h)
	return ok, nil
}

// readNames reads the names with the ingestion's analysis, as the composition root does.
func readNames(rel string, subtitle bool) domain.FileReading {
	kind := ingestiondomain.KindVideo
	if subtitle {
		kind = ingestiondomain.KindSubtitle
	}
	a := ingestiondomain.AnalyzeFile(rel, kind)
	return domain.FileReading{
		Season: a.Season, Chapter: a.Chapter, Last: a.LastChapter, Language: a.Language, Forced: a.Forced,
		SDH: a.SDH, TMDBID: a.TMDBID, IMDbID: a.IMDbID,
	}
}

// importFixture is a library on a temporary disk, with the real registry, journal, section and scan.
type importFixture struct {
	conn    *sql.DB
	root    string
	files   *libraryfs.FS
	library *catalogsqlite.LibraryRepository
	section *catalogsqlite.UnassignedRepository
	tmdb    *importTMDB
	imp     *application.LibraryImport
	now     time.Time
}

func newImportFixture(t *testing.T, tmdb *importTMDB) *importFixture {
	t.Helper()
	conn := openDB(t)
	root := t.TempDir()
	for _, dir := range []string{"Movies", "TV Shows", domain.UnassignedDir, ".videoexplorer"} {
		if err := os.MkdirAll(filepath.Join(root, dir), 0o750); err != nil {
			t.Fatal(err)
		}
	}
	now := time.Date(2026, time.October, 14, 12, 0, 0, 0, time.UTC)
	clock := func() time.Time { return now }
	files := libraryfs.New(root)
	engine := application.NewEngine(files, catalogsqlite.NewJournal(conn), clock, nil)
	library := catalogsqlite.NewLibraryRepository(conn)
	unassignedRepo := catalogsqlite.NewUnassignedRepository(conn)
	imp, err := application.NewLibraryImport(application.ImportDeps{
		Files:    files,
		Engine:   engine,
		Library:  library,
		Seen:     library,
		Aside:    application.NewUnassigned(unassignedRepo, files, engine, clock),
		Archives: headerProbe{files: files},
		TMDB:     tmdb,
		Names:    readNames,
		Roots:    application.Roots{Movies: "Movies", Series: "TV Shows"},
		Media:    domain.MediaExts{Video: []string{".mkv", ".mp4", ".avi"}, Subtitle: []string{".srt"}},
		Now:      clock,
	})
	if err != nil {
		t.Fatalf("NewLibraryImport: %v", err)
	}
	return &importFixture{conn: conn, root: root, files: files, library: library, section: unassignedRepo, tmdb: tmdb, imp: imp, now: now}
}

// put writes a file of the library with the given modification time (old files are settled, RF-27).
func (fx *importFixture) put(t *testing.T, rel string, content []byte, mtime time.Time) {
	t.Helper()
	p := filepath.Join(fx.root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, content, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(p, mtime, mtime); err != nil {
		t.Fatal(err)
	}
}

// exists reports whether a path of the library is on the disk.
func (fx *importFixture) exists(rel string) bool {
	_, err := os.Stat(filepath.Join(fx.root, filepath.FromSlash(rel)))
	return err == nil
}

// registry returns the titles of the registry by folder, with their files' rels.
func (fx *importFixture) registry(t *testing.T) map[string][]string {
	t.Helper()
	titles, err := fx.library.Titles(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	out := map[string][]string{}
	for _, title := range titles {
		rows, err := fx.library.Files(t.Context(), title.ID)
		if err != nil {
			t.Fatal(err)
		}
		var rels []string
		for _, f := range rows {
			rels = append(rels, f.Rel)
		}
		slices.Sort(rels)
		out[title.Folder] = rels
	}
	return out
}

// scan runs the library scan once and fails the test when it does not finish.
func (fx *importFixture) scan(t *testing.T) {
	t.Helper()
	if err := fx.imp.Scan(t.Context()); err != nil {
		t.Fatalf("Scan: %v", err)
	}
}

func TestImportIgnoresTitlesWhoseOnlyRegisteredFilesAreInTrash(t *testing.T) {
	t.Parallel()
	fx := newImportFixture(t, &importTMDB{})
	old := fx.now.Add(-time.Hour)
	rel := "Movies/Only Trash (2020)/Only Trash (2020).mkv"
	fx.put(t, rel, []byte("video bytes"), old)
	fx.scan(t)
	titles, err := fx.library.Titles(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	var title domain.LibraryTitle
	for _, candidate := range titles {
		if candidate.Folder == "Movies/Only Trash (2020)" {
			title = candidate
		}
	}
	if title.ID == 0 {
		t.Fatalf("import did not register test title: %+v", titles)
	}
	rows, err := fx.library.Files(t.Context(), title.ID)
	if err != nil || len(rows) != 1 {
		t.Fatalf("registered files: %+v, %v", rows, err)
	}
	trashDir := "only-trash"
	if err := os.MkdirAll(fx.files.TrashPath(trashDir), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(filepath.Join(fx.root, filepath.FromSlash(rel)), fx.files.TrashPath(trashDir, filepath.Base(rel))); err != nil {
		t.Fatal(err)
	}
	entry, err := domain.NewTrashEntryOf(domain.TrashOriginLibrary, domain.TrashReasonRemoved, trashDir, domain.Provenance{RelativePath: rel}, []domain.TrashFile{{Name: filepath.Base(rel), Size: 12}}, fx.now)
	if err != nil {
		t.Fatal(err)
	}
	var trashID int64
	if err := commitLibrary(t.Context(), fx.conn, func(rec application.Records) error {
		var err error
		trashID, err = rec.InsertTrash(t.Context(), entry)
		if err != nil {
			return err
		}
		return rec.TrashLibraryFiles(t.Context(), []int64{rows[0].ID}, trashID)
	}); err != nil {
		t.Fatal(err)
	}
	fx.scan(t)
	again, err := fx.library.Titles(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(again) != 1 || again[0].ID != title.ID {
		t.Fatalf("trash-only title was reimported or lost: %+v", again)
	}
	rows, err = fx.library.Files(t.Context(), title.ID)
	if err != nil || len(rows) != 1 || rows[0].TrashEntryID != trashID {
		t.Fatalf("trash-only file metadata after scan = %+v, %v", rows, err)
	}
}

// settled is a time long before the fixture's clock, so that the files are not being copied.
func (fx *importFixture) settled() time.Time { return fx.now.Add(-2 * time.Hour) }

// TestTheImportRegistersWhatJellyfinRecognizesInPlace imports a library shaped as the user's Movies and TV
// Shows folders: the movies by their folder and their IMDb tag, the chapters of a series with a scene name,
// and a series folder that holds only archives, which go to No asignados (RF-29, RF-27a).
func TestTheImportRegistersWhatJellyfinRecognizesInPlace(t *testing.T) {
	t.Parallel()
	tmdb := &importTMDB{
		configured: true,
		byIMDb: map[string][]application.TMDBTitle{
			"movie|tt18412256": {{ID: 1000001, Title: "Alien: Romulus", Year: 2024}},
		},
		search: map[string][]application.TMDBTitle{
			"movie|Godzilla": {{ID: 1000002, Title: "Godzilla", Year: 2014}},
			"series|Monstruo - La historia de Lizzie Borden": {{ID: 2000001, Title: "Monstruo: La historia de Lizzie Borden", Year: 2026}},
		},
	}
	fx := newImportFixture(t, tmdb)
	alien := "Alien Romulus (2024) [imdbid-tt18412256]"
	fx.put(t, "Movies/"+alien+"/"+alien+".mkv", []byte("alien"), fx.settled())
	fx.put(t, "Movies/Godzilla (2014)/Godzilla (2014).mp4", []byte("godzilla"), fx.settled())
	show := "Monstruo - La historia de Lizzie Borden (2026)"
	for n := 1; n <= 8; n++ {
		fx.put(t, fmt.Sprintf("TV Shows/%s/Season 01/Monstruo.La.historia.de.Lizzie.Borden.S01E%02d.2026.1080P-Dual-Lat.mkv", show, n), []byte("chapter"), fx.settled())
	}
	dahmer := "Monstruo - La historia de Jeffrey Dahmer (2022)"
	fx.put(t, "TV Shows/"+dahmer+"/a1b2c3.rar", []byte("Rar!\x1a\x07\x01\x00rest"), fx.settled())
	fx.put(t, "TV Shows/"+dahmer+"/d4e5f6.rar", []byte("Rar!\x1a\x07\x00rest"), fx.settled())

	fx.scan(t)

	reg := fx.registry(t)
	if got := reg["Movies/"+alien]; !slices.Equal(got, []string{"Movies/" + alien + "/" + alien + ".mkv"}) {
		t.Errorf("Alien registry = %v", got)
	}
	if got := reg["Movies/Godzilla (2014)"]; !slices.Equal(got, []string{"Movies/Godzilla (2014)/Godzilla (2014).mp4"}) {
		t.Errorf("Godzilla registry = %v", got)
	}
	if got := reg["TV Shows/"+show]; len(got) != 8 {
		t.Errorf("Lizzie Borden registry has %d files, want 8 chapters: %v", len(got), got)
	}
	if _, ok := reg["TV Shows/"+dahmer]; ok {
		t.Errorf("the archives-only folder must not be a title: %v", reg)
	}
	for n := 1; n <= 8; n++ {
		rel := fmt.Sprintf("TV Shows/%s/Season 01/Monstruo.La.historia.de.Lizzie.Borden.S01E%02d.2026.1080P-Dual-Lat.mkv", show, n)
		if !fx.exists(rel) {
			t.Errorf("the chapter %s was moved or renamed", rel)
		}
	}
	if !fx.exists("Movies/" + alien + "/" + alien + ".mkv") {
		t.Error("the movie was renamed or moved")
	}
	if fx.exists("TV Shows/" + dahmer) {
		t.Error("the folder emptied by the moves should be gone")
	}
	entries, err := fx.section.List(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	var found []string
	for _, e := range entries {
		if e.Name == dahmer && e.Origin == domain.OriginSamba {
			for _, f := range e.Files {
				found = append(found, f.Rel)
			}
		}
	}
	slices.Sort(found)
	if !slices.Equal(found, []string{"a1b2c3.rar", "d4e5f6.rar"}) {
		t.Errorf("No asignados holds %v for the archives, want both archives with origin samba", found)
	}
	if !fx.exists(domain.UnassignedDir + "/" + dahmer + "/a1b2c3.rar") {
		t.Error("the archive was not moved into No asignados")
	}
}

// TestASecondScanChangesNothing runs the scan again over the result of the first (RF-29: idempotence).
func TestASecondScanChangesNothing(t *testing.T) {
	t.Parallel()
	fx := newImportFixture(t, &importTMDB{configured: false})
	fx.put(t, "Movies/Godzilla (2014)/Godzilla (2014).mp4", []byte("godzilla"), fx.settled())
	fx.put(t, "Movies/Foo (2010)/Foo (2010).mkv", []byte("foo"), fx.settled())
	fx.put(t, "Movies/Foo (2010)/Foo (2010).es.srt", []byte("sub"), fx.settled())
	fx.put(t, "Movies/Foo (2010)/junk.rar", []byte("Rar!\x1a\x07\x00"), fx.settled())
	fx.scan(t)
	before := fx.registry(t)
	fx.scan(t)
	if after := fx.registry(t); !equalRegistry(before, after) {
		t.Errorf("the second scan changed the registry:\nbefore %v\nafter  %v", before, after)
	}
	if fx.exists("Movies/Foo (2010)/junk.rar") {
		t.Error("the archive should have gone to No asignados on the first scan")
	}
}

// equalRegistry compares two readings of the registry.
func equalRegistry(a, b map[string][]string) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if !slices.Equal(v, b[k]) {
			return false
		}
	}
	return true
}

// TestFreeTitlesWithoutTMDB registers the titles with their folder's name and year when TMDB is not set up
// (RF-29, spec §6): «Sin vincular», with no TMDB id and no tag.
func TestFreeTitlesWithoutTMDB(t *testing.T) {
	t.Parallel()
	fx := newImportFixture(t, &importTMDB{configured: false})
	fx.put(t, "Movies/Godzilla (2014) [tmdbid-1234]/Godzilla (2014).mp4", []byte("godzilla"), fx.settled())
	fx.scan(t)
	titles, err := fx.library.Titles(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(titles) != 1 {
		t.Fatalf("titles = %+v, want one", titles)
	}
	got := titles[0]
	if got.TMDBID != 0 || got.IDTag || got.Title != "Godzilla" || got.Year != 2014 || got.Folder != "Movies/Godzilla (2014) [tmdbid-1234]" {
		t.Errorf("title = %+v, want a free title with the folder's name and year", got)
	}
	if !fx.exists("Movies/Godzilla (2014) [tmdbid-1234]/Godzilla (2014).mp4") {
		t.Error("the file was moved")
	}
}

// TestRemovedFilesLeaveTheRegistry: what Samba deletes leaves the registry, and its title with it when nothing
// is left (RF-29).
func TestRemovedFilesLeaveTheRegistry(t *testing.T) {
	t.Parallel()
	fx := newImportFixture(t, &importTMDB{configured: false})
	fx.put(t, "Movies/Foo (2010)/Foo (2010).mkv", []byte("foo"), fx.settled())
	fx.put(t, "Movies/Foo (2010)/Foo (2010) - 1080p.mkv", []byte("foo hd"), fx.settled())
	fx.put(t, "Movies/Bar (2011)/Bar (2011).mkv", []byte("bar"), fx.settled())
	fx.scan(t)

	if err := os.Remove(filepath.Join(fx.root, "Movies/Foo (2010)/Foo (2010) - 1080p.mkv")); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(filepath.Join(fx.root, "Movies/Bar (2011)")); err != nil {
		t.Fatal(err)
	}
	fx.scan(t)

	reg := fx.registry(t)
	if got := reg["Movies/Foo (2010)"]; !slices.Equal(got, []string{"Movies/Foo (2010)/Foo (2010).mkv"}) {
		t.Errorf("Foo registry = %v, want only the file still on disk", got)
	}
	if _, ok := reg["Movies/Bar (2011)"]; ok {
		t.Errorf("Bar has no file left and must be gone: %v", reg)
	}
}

// TestAFileStillBeingCopiedWaitsForTheNextScan: a file modified less than a minute ago is not registered; the
// next scan takes it once it has settled (RF-28, RF-29).
func TestAFileStillBeingCopiedWaitsForTheNextScan(t *testing.T) {
	t.Parallel()
	fx := newImportFixture(t, &importTMDB{configured: false})
	fx.put(t, "Movies/Foo (2010)/Foo (2010).mkv", []byte("foo"), fx.now.Add(-10*time.Second))
	fx.scan(t)
	if len(fx.registry(t)) != 0 {
		t.Fatalf("a copy in progress was registered: %v", fx.registry(t))
	}
	if !fx.exists("Movies/Foo (2010)/Foo (2010).mkv") {
		t.Fatal("a copy in progress was moved")
	}

	// The copy ends with a new modification time: that change is read by the next scan, which still waits
	// (RF-28, «cambió desde la lectura anterior»), and the scan after it registers the file.
	fx.put(t, "Movies/Foo (2010)/Foo (2010).mkv", []byte("foo"), fx.settled())
	fx.scan(t)
	if len(fx.registry(t)) != 0 {
		t.Fatalf("a file that changed since the last scan was registered: %v", fx.registry(t))
	}
	fx.scan(t)
	if got := fx.registry(t)["Movies/Foo (2010)"]; !slices.Equal(got, []string{"Movies/Foo (2010)/Foo (2010).mkv"}) {
		t.Errorf("registry after the copy settled = %v", got)
	}
}

// TestAnEpisodeWithNoChapterAndAnArchiveInASeries: a chapter the name does not number is no chapter, and an
// archive in a series goes to No asignados while its chapters stay (RF-29, RF-27a).
func TestAnEpisodeWithNoChapterAndAnArchiveInASeries(t *testing.T) {
	t.Parallel()
	fx := newImportFixture(t, &importTMDB{configured: false})
	fx.put(t, "TV Shows/Show (2010)/Season 01/Show.S01E01.mkv", []byte("e1"), fx.settled())
	fx.put(t, "TV Shows/Show (2010)/Season 01/Pilot.mkv", []byte("pilot"), fx.settled())
	fx.put(t, "TV Shows/Show (2010)/Season 02/pack.rar", []byte("Rar!\x1a\x07\x00"), fx.settled())
	fx.scan(t)

	if got := fx.registry(t)["TV Shows/Show (2010)"]; !slices.Equal(got, []string{"TV Shows/Show (2010)/Season 01/Show.S01E01.mkv"}) {
		t.Errorf("registry = %v", got)
	}
	if !fx.exists(domain.UnassignedDir + "/Show (2010)/Season 01/Pilot.mkv") {
		t.Error("the chapter with no number should be in No asignados, keeping its folder")
	}
	if !fx.exists(domain.UnassignedDir + "/Show (2010)/Season 02/pack.rar") {
		t.Error("the archive should be in No asignados, keeping its folder")
	}
}

// TestSubtitlesFollowTheirVideo registers a subtitle with its video by name and leaves one with no video in
// No asignados (RF-23, RF-29).
func TestSubtitlesFollowTheirVideo(t *testing.T) {
	t.Parallel()
	fx := newImportFixture(t, &importTMDB{configured: false})
	fx.put(t, "Movies/Foo (2010)/Foo (2010).mkv", []byte("foo"), fx.settled())
	fx.put(t, "Movies/Foo (2010)/Foo (2010).es.srt", []byte("sub"), fx.settled())
	fx.put(t, "Movies/Foo (2010)/Orphan.es.srt", []byte("orphan"), fx.settled())
	fx.scan(t)

	want := []string{"Movies/Foo (2010)/Foo (2010).es.srt", "Movies/Foo (2010)/Foo (2010).mkv"}
	if got := fx.registry(t)["Movies/Foo (2010)"]; !slices.Equal(got, want) {
		t.Errorf("registry = %v, want %v", got, want)
	}
	if !fx.exists(domain.UnassignedDir + "/Foo (2010)/Orphan.es.srt") {
		t.Error("the subtitle with no video should be in No asignados")
	}
}

// TestAnUnknownTagLeavesTheTitleFree: a folder whose id tag TMDB does not know is «Sin vincular», even when a
// title with its name and year exists; it is not searched by name (RF-29).
func TestAnUnknownTagLeavesTheTitleFree(t *testing.T) {
	t.Parallel()
	tmdb := &importTMDB{
		configured: true,
		search:     map[string][]application.TMDBTitle{"movie|Foo": {{ID: 77, Title: "Foo", Year: 2010}}},
	}
	fx := newImportFixture(t, tmdb)
	fx.put(t, "Movies/Foo (2010) [tmdbid-999]/Foo (2010).mkv", []byte("foo"), fx.settled())
	fx.scan(t)

	titles, err := fx.library.Titles(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(titles) != 1 || titles[0].TMDBID != 0 || titles[0].IDTag || titles[0].Title != "Foo" {
		t.Errorf("titles = %+v, want one free title «Sin vincular»", titles)
	}
	if !fx.exists("Movies/Foo (2010) [tmdbid-999]/Foo (2010).mkv") {
		t.Error("the file was moved")
	}
}

// TestASubtitleWithNoLanguageIsRegisteredAsLatinSpanish: a subtitle whose name gives no language is registered
// with its video as es-419, and it keeps its name on the disk (RF-29, RF-23).
func TestASubtitleWithNoLanguageIsRegisteredAsLatinSpanish(t *testing.T) {
	t.Parallel()
	fx := newImportFixture(t, &importTMDB{configured: false})
	fx.put(t, "Movies/Foo (2010)/Foo (2010).mkv", []byte("foo"), fx.settled())
	fx.put(t, "Movies/Foo (2010)/Foo (2010).srt", []byte("sub"), fx.settled())
	fx.scan(t)

	rows, err := fx.library.Files(t.Context(), mustTitleID(t, fx, "Movies/Foo (2010)"))
	if err != nil {
		t.Fatal(err)
	}
	var sub *domain.LibraryFile
	for i := range rows {
		if rows[i].Kind == domain.FileSubtitle {
			sub = &rows[i]
		}
	}
	if sub == nil || sub.Language != "es-419" || sub.Rel != "Movies/Foo (2010)/Foo (2010).srt" {
		t.Fatalf("subtitle row = %+v, want es-419 under its own name", sub)
	}
	if !fx.exists("Movies/Foo (2010)/Foo (2010).srt") {
		t.Error("the subtitle was renamed or moved")
	}
}

// mustTitleID returns the id of the title in a folder of the registry.
func mustTitleID(t *testing.T, fx *importFixture, folder string) int64 {
	t.Helper()
	titles, err := fx.library.Titles(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	for _, title := range titles {
		if title.Folder == folder {
			return title.ID
		}
	}
	t.Fatalf("no title in %s", folder)
	return 0
}

// TestVersionsOfAChapterAreKeptOnlyWhenAllCarryALabel: two files of one chapter that both carry a version label
// are registered as its versions; a chapter whose files do not all carry one goes to No asignados (RF-29, §5).
func TestVersionsOfAChapterAreKeptOnlyWhenAllCarryALabel(t *testing.T) {
	t.Parallel()
	fx := newImportFixture(t, &importTMDB{configured: false})
	show := "TV Shows/Show (2010)/Season 01/"
	fx.put(t, show+"Show (2010) - S01E01 - Pilot - 1080p.mkv", []byte("hd"), fx.settled())
	fx.put(t, show+"Show (2010) - S01E01 - Pilot - 720p.mkv", []byte("sd"), fx.settled())
	fx.put(t, show+"Show (2010) - S01E02.mkv", []byte("plain"), fx.settled())
	fx.put(t, show+"Show (2010) - S01E02 - Pilot - 720p.mkv", []byte("labelled"), fx.settled())
	fx.scan(t)

	want := []string{
		"TV Shows/Show (2010)/Season 01/Show (2010) - S01E01 - Pilot - 1080p.mkv",
		"TV Shows/Show (2010)/Season 01/Show (2010) - S01E01 - Pilot - 720p.mkv",
	}
	if got := fx.registry(t)["TV Shows/Show (2010)"]; !slices.Equal(got, want) {
		t.Errorf("registry = %v, want the two versions of chapter 1", got)
	}
	for _, rel := range []string{"Show (2010) - S01E02.mkv", "Show (2010) - S01E02 - Pilot - 720p.mkv"} {
		if !fx.exists(domain.UnassignedDir + "/Show (2010)/Season 01/" + rel) {
			t.Errorf("%s should be in No asignados, keeping its folder", rel)
		}
	}
}
