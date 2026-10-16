package sqlite_test

import (
	"testing"
	"time"

	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/catalog/application"
	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/catalog/domain"
	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/catalog/infrastructure/sqlite/sqlcgen"
)

func TestTrashedSubtitleRowSurvivesDeletingItsVideo(t *testing.T) {
	t.Parallel()
	conn := openDB(t)
	ctx := t.Context()
	now := time.Date(2026, 10, 15, 12, 0, 0, 0, time.UTC)
	title, err := domain.NewLibraryTitle(domain.KindMovie, "Dune", "Dune", 2021, 0, false, "Movies/Dune (2021)", now)
	if err != nil {
		t.Fatal(err)
	}
	video, err := domain.NewVideo(domain.KindMovie, domain.VideoSpec{Rel: "Movies/Dune (2021)/Dune (2021).mkv", Size: 12}, now)
	if err != nil {
		t.Fatal(err)
	}
	sub, err := domain.NewSubtitle(domain.SubtitleSpec{Rel: "Movies/Dune (2021)/Dune (2021).en.srt", VideoRel: video.Rel, Language: "en", Size: 3}, now)
	if err != nil {
		t.Fatal(err)
	}
	var titleID, videoID, subID, trashID int64
	if err := commitLibrary(ctx, conn, func(rec application.Records) error {
		var err error
		titleID, err = rec.InsertLibraryTitle(ctx, title)
		if err != nil {
			return err
		}
		return rec.InsertLibraryFiles(ctx, titleID, []domain.LibraryFile{video, sub})
	}); err != nil {
		t.Fatal(err)
	}
	q := sqlcgen.New(conn)
	videoID, err = q.LibraryFileIDByRel(ctx, video.Rel)
	if err != nil {
		t.Fatal(err)
	}
	subID, err = q.LibraryFileIDByRel(ctx, sub.Rel)
	if err != nil {
		t.Fatal(err)
	}
	if err := commitLibrary(ctx, conn, func(rec application.Records) error {
		entry := domain.TrashEntry{Origin: domain.TrashOriginLibrary, Reason: domain.TrashReasonRemoved, Dir: "restore-check", TrashedAt: now, RelativePath: video.Rel, Files: []domain.TrashFile{{Name: "Dune (2021).mkv", Size: 12}, {Name: "Dune (2021).en.srt", Size: 3}}}
		var err error
		trashID, err = rec.InsertTrash(ctx, entry)
		if err != nil {
			return err
		}
		return rec.TrashLibraryFiles(ctx, []int64{videoID, subID}, trashID)
	}); err != nil {
		t.Fatal(err)
	}
	if err := commitLibrary(ctx, conn, func(rec application.Records) error { return rec.DeleteLibraryFile(ctx, videoID) }); err != nil {
		t.Fatalf("delete video: %v", err)
	}
	var retained, videoLogical, entryLink int64
	var videoRel string
	if err := conn.QueryRowContext(ctx, `SELECT id, video_id, video_rel, trash_entry_id FROM library_files WHERE id = ?`, subID).Scan(&retained, &videoLogical, &videoRel, &entryLink); err != nil {
		t.Fatalf("read subtitle after video deletion: %v", err)
	}
	if retained != subID || videoLogical != videoID || videoRel != video.Rel || entryLink != trashID {
		t.Fatalf("subtitle reference after video deletion: id=%d video=%d rel=%q trash=%d", retained, videoLogical, videoRel, entryLink)
	}
}
