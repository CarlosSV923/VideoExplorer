package sqlite_test

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/catalog/application"
	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/catalog/domain"
	catalogsqlite "github.com/CarlosSV923/VideoExplorer/apps/api/internal/catalog/infrastructure/sqlite"
)

var libraryNow = time.Date(2026, time.October, 13, 10, 0, 0, 0, time.UTC)

// commitLibrary runs fn in the commit of a journal change, as the engine does, and returns its error.
func commitLibrary(ctx context.Context, conn *sql.DB, fn func(application.Records) error) error {
	return catalogsqlite.NewJournal(conn).Commit(ctx, "", fn)
}

func mustTitle(t *testing.T, kind domain.TitleKind, title, folder string, tmdbID int64, idTag bool) domain.LibraryTitle {
	t.Helper()
	got, err := domain.NewLibraryTitle(kind, title, title, 2008, tmdbID, idTag, folder, libraryNow)
	if err != nil {
		t.Fatalf("NewLibraryTitle: %v", err)
	}
	return got
}

func mustVideo(t *testing.T, kind domain.TitleKind, s domain.VideoSpec) domain.LibraryFile {
	t.Helper()
	got, err := domain.NewVideo(kind, s, libraryNow)
	if err != nil {
		t.Fatalf("NewVideo: %v", err)
	}
	return got
}

func mustSubtitle(t *testing.T, s domain.SubtitleSpec) domain.LibraryFile {
	t.Helper()
	got, err := domain.NewSubtitle(s, libraryNow)
	if err != nil {
		t.Fatalf("NewSubtitle: %v", err)
	}
	return got
}

// recordTitle stores a title with its files in one commit and returns the title's id.
func recordTitle(t *testing.T, conn *sql.DB, title domain.LibraryTitle, files ...domain.LibraryFile) int64 {
	t.Helper()
	var id int64
	err := commitLibrary(t.Context(), conn, func(r application.Records) error {
		var err error
		if id, err = r.InsertLibraryTitle(t.Context(), title); err != nil {
			return err
		}
		return r.InsertLibraryFiles(t.Context(), id, files)
	})
	if err != nil {
		t.Fatalf("record title: %v", err)
	}
	return id
}

func TestLibraryTitlesAndFilesSurviveTheDatabase(t *testing.T) {
	t.Parallel()

	conn := openDB(t)
	repo := catalogsqlite.NewLibraryRepository(conn)

	title := mustTitle(t, domain.KindMovie, "Iron Man", "Movies/Iron Man (2008) [tmdbid-1726]", 1726, true)
	video := mustVideo(t, domain.KindMovie, domain.VideoSpec{
		Rel: "Movies/Iron Man (2008) [tmdbid-1726]/Iron Man (2008) [tmdbid-1726].mkv", Size: 100, PartialHash: "ph1:aa",
	})
	subtitle := mustSubtitle(t, domain.SubtitleSpec{
		Rel:      "Movies/Iron Man (2008) [tmdbid-1726]/Iron Man (2008) [tmdbid-1726].es-MX.srt",
		VideoRel: video.Rel, Language: "es-MX", Size: 5,
	})
	id := recordTitle(t, conn, title, video, subtitle)

	got, found, err := repo.TitleByFolder(t.Context(), "movies/iron man (2008) [TMDBID-1726]")
	if err != nil || !found {
		t.Fatalf("TitleByFolder = %v, %v; want the title, in any letter case", found, err)
	}
	if got.ID != id || got.Kind != domain.KindMovie || got.Title != "Iron Man" || got.TMDBID != 1726 ||
		!got.IDTag || got.Year != 2008 || !got.CreatedAt.Equal(libraryNow) {
		t.Errorf("title = %+v", got)
	}

	files, err := repo.Files(t.Context(), id)
	if err != nil {
		t.Fatalf("Files: %v", err)
	}
	if len(files) != 2 {
		t.Fatalf("files = %+v; want the video and its subtitle", files)
	}
	v, s := files[0], files[1]
	if v.Kind != domain.FileVideo || v.Size != 100 || v.PartialHash != "ph1:aa" || v.Place != (domain.Place{}) || !v.Live() {
		t.Errorf("video = %+v", v)
	}
	if s.Kind != domain.FileSubtitle || s.Language != "es-419" || s.VideoRel != video.Rel || s.Place != (domain.Place{}) {
		t.Errorf("subtitle = %+v; want es-419 with its video's path", s)
	}
}

func TestLibraryKeepsTheChaptersOfASeriesExactly(t *testing.T) {
	t.Parallel()

	conn := openDB(t)
	repo := catalogsqlite.NewLibraryRepository(conn)

	title := mustTitle(t, domain.KindSeries, "Breaking Bad", "TV Shows/Breaking Bad (2008) [tmdbid-1396]", 1396, true)
	specials := mustVideo(t, domain.KindSeries, domain.VideoSpec{
		Rel:   "TV Shows/Breaking Bad (2008) [tmdbid-1396]/Season 00/Breaking Bad (2008) - S00E01.mkv",
		Place: domain.Place{Episode: true, Season: 0, First: 1, Last: 1}, Size: 1,
	})
	double := mustVideo(t, domain.KindSeries, domain.VideoSpec{
		Rel:   "TV Shows/Breaking Bad (2008) [tmdbid-1396]/Season 01/Breaking Bad (2008) - S01E01-E02.mkv",
		Place: domain.Place{Episode: true, Season: 1, First: 1, Last: 2}, Size: 2,
	})
	recordTitle(t, conn, title, specials, double)

	got, found, err := repo.TitleByFolder(t.Context(), title.Folder)
	if err != nil || !found {
		t.Fatalf("TitleByFolder = %v, %v", found, err)
	}
	files, err := repo.Files(t.Context(), got.ID)
	if err != nil || len(files) != 2 {
		t.Fatalf("Files = %+v, %v", files, err)
	}
	if want := (domain.Place{Episode: true, Season: 0, First: 1, Last: 1}); files[0].Place != want {
		t.Errorf("specials place = %+v; want %+v (season 0 is a season, not a movie)", files[0].Place, want)
	}
	if want := (domain.Place{Episode: true, Season: 1, First: 1, Last: 2}); files[1].Place != want {
		t.Errorf("double place = %+v; want %+v", files[1].Place, want)
	}
}

func TestLibraryRefusesTheRecordsThatCollide(t *testing.T) {
	t.Parallel()

	conn := openDB(t)
	title := mustTitle(t, domain.KindMovie, "Iron Man", "Movies/Iron Man (2008)", 0, false)
	video := mustVideo(t, domain.KindMovie, domain.VideoSpec{Rel: "Movies/Iron Man (2008)/Iron Man (2008).mkv", Size: 1})
	recordTitle(t, conn, title, video)

	otherCase := mustTitle(t, domain.KindMovie, "Other", "movies/IRON MAN (2008)", 0, false)
	sameFile := mustVideo(t, domain.KindMovie, domain.VideoSpec{Rel: "movies/iron man (2008)/IRON MAN (2008).MKV", Size: 1})
	orphanSub := mustSubtitle(t, domain.SubtitleSpec{Rel: "Movies/Other/a.srt", VideoRel: "Movies/Other/a.mkv", Language: "en"})
	freeWithTag := domain.LibraryTitle{Kind: domain.KindMovie, Title: "Free", DiscTitle: "Free", Folder: "Movies/Free", IDTag: true}

	tests := []struct {
		name   string
		commit func(application.Records) error
	}{
		{name: "a folder taken in another letter case", commit: func(r application.Records) error {
			_, err := r.InsertLibraryTitle(t.Context(), otherCase)
			return err
		}},
		{name: "a file path taken in another letter case", commit: func(r application.Records) error {
			return r.InsertLibraryFiles(t.Context(), 1, []domain.LibraryFile{sameFile})
		}},
		{name: "a subtitle whose video is not in the registry", commit: func(r application.Records) error {
			return r.InsertLibraryFiles(t.Context(), 1, []domain.LibraryFile{orphanSub})
		}},
		{name: "a free title with the tag of ID, which the schema refuses", commit: func(r application.Records) error {
			_, err := r.InsertLibraryTitle(t.Context(), freeWithTag)
			return err
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if err := commitLibrary(t.Context(), conn, tt.commit); err == nil {
				t.Error("the commit was accepted")
			}
		})
	}
	if _, found, _ := catalogsqlite.NewLibraryRepository(conn).TitleByFolder(t.Context(), "Movies/Other"); found {
		t.Error("a refused commit left a title behind")
	}
}

func TestLibraryCommitIsAllOrNothing(t *testing.T) {
	t.Parallel()

	conn := openDB(t)
	repo := catalogsqlite.NewLibraryRepository(conn)

	title := mustTitle(t, domain.KindMovie, "Heat", "Movies/Heat (1995)", 0, false)
	video := mustVideo(t, domain.KindMovie, domain.VideoSpec{Rel: "Movies/Heat (1995)/Heat (1995).mkv", Size: 1})
	badSub := mustSubtitle(t, domain.SubtitleSpec{Rel: "Movies/Heat (1995)/Heat.srt", VideoRel: "Movies/Heat (1995)/missing.mkv", Language: "en"})

	err := commitLibrary(t.Context(), conn, func(r application.Records) error {
		id, err := r.InsertLibraryTitle(t.Context(), title)
		if err != nil {
			return err
		}
		if err := r.InsertLibraryFiles(t.Context(), id, []domain.LibraryFile{video, badSub}); err != nil {
			return err
		}
		return nil
	})
	if err == nil {
		t.Fatal("the commit with a subtitle of no video was accepted")
	}
	if _, found, _ := repo.TitleByFolder(t.Context(), title.Folder); found {
		t.Error("the title of a failed commit was kept")
	}
}

func TestLibraryMovesAndTrashesFilesAndKeepsTheirSubtitles(t *testing.T) {
	t.Parallel()

	conn := openDB(t)
	repo := catalogsqlite.NewLibraryRepository(conn)

	title := mustTitle(t, domain.KindMovie, "Dune", "Movies/Dune (2021)", 0, false)
	video := mustVideo(t, domain.KindMovie, domain.VideoSpec{Rel: "Movies/Dune (2021)/Dune (2021).mkv", Size: 9})
	subtitle := mustSubtitle(t, domain.SubtitleSpec{Rel: "Movies/Dune (2021)/Dune (2021).en.srt", VideoRel: video.Rel, Language: "en", Size: 1})
	id := recordTitle(t, conn, title, video, subtitle)
	files, err := repo.Files(t.Context(), id)
	if err != nil {
		t.Fatal(err)
	}
	videoID, subID := files[0].ID, files[1].ID

	const newRel = "Movies/Dune (2021)/Dune (2021) - 2160p.mkv"
	trashed, err := domain.NewTrashEntry(domain.TrashOriginUpload, domain.TrashReasonFiltered, "entry-1",
		domain.Provenance{BatchID: "b1", RelativePath: "Dune.mkv"}, domain.TrashFile{Name: "x.mkv", Size: 1}, libraryNow)
	if err != nil {
		t.Fatal(err)
	}
	var entryID int64
	err = commitLibrary(t.Context(), conn, func(r application.Records) error {
		if err := r.MoveLibraryFile(t.Context(), videoID, newRel); err != nil {
			return err
		}
		var err error
		if entryID, err = r.InsertTrash(t.Context(), trashed); err != nil {
			return err
		}
		return r.TrashLibraryFiles(t.Context(), []int64{subID}, entryID)
	})
	if err != nil {
		t.Fatalf("commit: %v", err)
	}

	after, err := repo.Files(t.Context(), id)
	if err != nil {
		t.Fatal(err)
	}
	if after[0].Rel != newRel || !after[0].Live() {
		t.Errorf("moved video = %+v; want its new path, in the library", after[0])
	}
	if after[1].VideoRel != newRel {
		t.Errorf("subtitle's video = %q; want the video's new path %q", after[1].VideoRel, newRel)
	}
	if after[1].TrashEntryID != entryID || after[1].Live() {
		t.Errorf("trashed subtitle = %+v; want trash entry %d", after[1], entryID)
	}
}
