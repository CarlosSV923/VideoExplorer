package sqlite_test

import (
	"testing"

	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/catalog/application"
	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/catalog/domain"
)

// unassignedEntry reads the registry's entry of No asignados with the name, its origin and its relation.
func (e *editEnv) unassignedEntry(name string) (origin, relation string, loose bool, found bool) {
	e.t.Helper()
	err := e.conn.QueryRowContext(e.ctx,
		`SELECT origin, relation, loose FROM unassigned_entries WHERE name = ?`, name).Scan(&origin, &relation, &loose)
	if err != nil {
		return "", "", false, false
	}
	return origin, relation, loose, true
}

// trashedTo returns the origin and reason of the trash entries, one per video (RF-30).
func (e *editEnv) trashedEntries() (count int, origin, reason string) {
	e.t.Helper()
	rows, err := e.conn.QueryContext(e.ctx, `SELECT origin, reason FROM trash_entries ORDER BY id`)
	if err != nil {
		e.t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		if err := rows.Scan(&origin, &reason); err != nil {
			e.t.Fatal(err)
		}
		count++
	}
	return count, origin, reason
}

// inLibrary reports whether a title is in the library's listing (RF-30: a title in the trash is not).
func (e *editEnv) inLibrary(titleID int64) bool {
	e.t.Helper()
	titles, err := application.NewLibraryViews(e.repo, editTMDB{}, nil).Titles(e.ctx)
	if err != nil {
		e.t.Fatalf("Titles: %v", err)
	}
	for _, t := range titles {
		if t.ID == titleID {
			return true
		}
	}
	return false
}

// TestMovingAWholeTitleToUnassignedKeepsItsFolderAndFreesTheRegistry moves a title with its subtitle to No
// asignados: the folder's name is kept, the entry is samba with no relation, and the registry forgets the title
// (RF-44, RF-29).
func TestMovingAWholeTitleToUnassignedKeepsItsFolderAndFreesTheRegistry(t *testing.T) {
	t.Parallel()
	e := newEditEnv(t, nil)
	video := e.write(ironFolder+"/Iron Man (2008).mkv", "iron")
	sub := e.write(ironFolder+"/Iron Man (2008).es.srt", "sub")
	titleID := e.seedMovie(ironFolder, 0, false,
		e.video(domain.KindMovie, video, domain.Place{}, ""),
		mustSubtitle(t, domain.SubtitleSpec{Rel: sub, VideoRel: video, Language: "es", Size: 3}))

	if err := e.edits.MoveToUnassigned(e.ctx, titleID, application.Scope{Kind: application.ScopeTitle}); err != nil {
		t.Fatalf("MoveToUnassigned: %v", err)
	}
	if !e.exists("_unassigned/Iron Man (2008)/Iron Man (2008).mkv") || !e.exists("_unassigned/Iron Man (2008)/Iron Man (2008).es.srt") {
		t.Error("the video and its subtitle are in the entry")
	}
	if e.exists(video) || e.exists(ironFolder) {
		t.Error("the library folder is gone")
	}
	origin, relation, loose, ok := e.unassignedEntry("Iron Man (2008)")
	if !ok || origin != domain.OriginSamba || relation != domain.RelationNone || loose {
		t.Errorf("the entry = %s %s loose=%v found=%v", origin, relation, loose, ok)
	}
	if e.inLibrary(titleID) {
		t.Error("the title is no longer in the library")
	}
	if _, found := e.titleOf(ironFolder); found {
		t.Error("the title is gone from the registry, with no file left")
	}
	e.assertNothingJournaled()
}

// TestMovingASeasonToUnassignedKeepsTheOtherSeasons names the entry "<Título> (<Año>) Season NN" and leaves the
// title with its other seasons (RF-44).
func TestMovingASeasonToUnassignedKeepsTheOtherSeasons(t *testing.T) {
	t.Parallel()
	e := newEditEnv(t, nil)
	s1 := e.write(lizzieFolder+"/Season 01/Lizzie (2008) - S01E01 - Pilot.mkv", "one")
	s2 := e.write(lizzieFolder+"/Season 02/Lizzie (2008) - S02E01 - Return.mkv", "two")
	titleID := recordTitle(t, e.conn, mustTitle(t, domain.KindSeries, "Lizzie", lizzieFolder, 0, false),
		e.video(domain.KindSeries, s1, domain.Place{Episode: true, Season: 1, First: 1, Last: 1}, ""),
		e.video(domain.KindSeries, s2, domain.Place{Episode: true, Season: 2, First: 1, Last: 1}, ""))

	if err := e.edits.MoveToUnassigned(e.ctx, titleID, application.Scope{Kind: application.ScopeSeason, Season: 1}); err != nil {
		t.Fatalf("MoveToUnassigned: %v", err)
	}
	if !e.exists("_unassigned/Lizzie (2008) Season 01/Lizzie (2008) - S01E01 - Pilot.mkv") {
		t.Error("the season's chapter is in the entry, named as its season")
	}
	if !e.exists(s2) {
		t.Error("the other season stays where it is")
	}
	rows := e.rows(titleID)
	if _, ok := rows[s1]; ok {
		t.Error("the season's chapter is forgotten by the registry")
	}
	if _, ok := rows[s2]; !ok {
		t.Error("the other season is still in the registry")
	}
	if _, _, _, ok := e.unassignedEntry("Lizzie (2008) Season 01"); !ok {
		t.Error("the entry is recorded in the registry")
	}
	if !e.inLibrary(titleID) {
		t.Error("the title stays in the library")
	}
}

// TestMovingAFileWithSubtitlesMakesAFolderAndAVideoWithoutMakesALooseFile checks the two names of a file (RF-08):
// a video with subtitles becomes a folder named after it, and a video without subtitles stays a loose file.
func TestMovingAFileWithSubtitlesMakesAFolderAndAVideoWithoutMakesALooseFile(t *testing.T) {
	t.Parallel()
	e := newEditEnv(t, nil)
	video := e.write(lizzieFolder+"/Season 01/Lizzie (2008) - S01E02 - Trial.mkv", "trial")
	sub := e.write(lizzieFolder+"/Season 01/Lizzie (2008) - S01E02 - Trial.es.srt", "sub")
	plain := e.write(ironFolder+"/Iron Man (2008).mkv", "iron")
	titleID := recordTitle(t, e.conn, mustTitle(t, domain.KindSeries, "Lizzie", lizzieFolder, 0, false),
		e.video(domain.KindSeries, video, domain.Place{Episode: true, Season: 1, First: 2, Last: 2}, ""),
		mustSubtitle(t, domain.SubtitleSpec{Rel: sub, VideoRel: video, Language: "es", Size: 3}))
	plainID := e.seedMovie(ironFolder, 0, false, e.video(domain.KindMovie, plain, domain.Place{}, ""))
	rows := e.rows(titleID)
	videoID := rows[video].ID

	if err := e.edits.MoveToUnassigned(e.ctx, titleID, application.Scope{Kind: application.ScopeFile, FileID: videoID}); err != nil {
		t.Fatalf("MoveToUnassigned with a subtitle: %v", err)
	}
	if !e.exists("_unassigned/Lizzie (2008) - S01E02 - Trial/Lizzie (2008) - S01E02 - Trial.mkv") ||
		!e.exists("_unassigned/Lizzie (2008) - S01E02 - Trial/Lizzie (2008) - S01E02 - Trial.es.srt") {
		t.Error("the video and its subtitle are in a folder named after the video")
	}
	if _, _, loose, ok := e.unassignedEntry("Lizzie (2008) - S01E02 - Trial"); !ok || loose {
		t.Errorf("a video with subtitles is a folder: loose=%v found=%v", loose, ok)
	}

	plainRows := e.rows(plainID)
	if err := e.edits.MoveToUnassigned(e.ctx, plainID, application.Scope{Kind: application.ScopeFile, FileID: plainRows[plain].ID}); err != nil {
		t.Fatalf("MoveToUnassigned without subtitles: %v", err)
	}
	if !e.exists("_unassigned/Iron Man (2008).mkv") {
		t.Error("a video without subtitles is a loose file, named with its extension")
	}
	if _, _, loose, ok := e.unassignedEntry("Iron Man (2008).mkv"); !ok || !loose {
		t.Errorf("the loose entry: loose=%v found=%v", loose, ok)
	}
}

// TestTrashingAFileKeepsItsRowAndItsSubtitlesInTheTrash sends a chapter with its subtitle to the trash, with origin
// library and reason removed (RF-30, RF-44). The title stays in the library: it has other files.
func TestTrashingAFileKeepsItsRowAndItsSubtitlesInTheTrash(t *testing.T) {
	t.Parallel()
	e := newEditEnv(t, nil)
	video := e.write(lizzieFolder+"/Season 01/Lizzie (2008) - S01E02 - Trial.mkv", "trial")
	sub := e.write(lizzieFolder+"/Season 01/Lizzie (2008) - S01E02 - Trial.es.srt", "sub")
	other := e.write(lizzieFolder+"/Season 01/Lizzie (2008) - S01E01 - Pilot.mkv", "pilot")
	titleID := recordTitle(t, e.conn, mustTitle(t, domain.KindSeries, "Lizzie", lizzieFolder, 0, false),
		e.video(domain.KindSeries, video, domain.Place{Episode: true, Season: 1, First: 2, Last: 2}, ""),
		mustSubtitle(t, domain.SubtitleSpec{Rel: sub, VideoRel: video, Language: "es", Size: 3}),
		e.video(domain.KindSeries, other, domain.Place{Episode: true, Season: 1, First: 1, Last: 1}, ""))
	rows := e.rows(titleID)

	if err := e.edits.Trash(e.ctx, titleID, application.Scope{Kind: application.ScopeFile, FileID: rows[video].ID}); err != nil {
		t.Fatalf("Trash: %v", err)
	}
	if e.exists(video) || e.exists(sub) || !e.exists(other) {
		t.Error("the chapter and its subtitle left the library; the other chapter stays")
	}
	count, origin, reason := e.trashedEntries()
	if count != 1 || origin != string(domain.TrashOriginLibrary) || reason != string(domain.TrashReasonRemoved) {
		t.Errorf("trash entries = %d %s %s", count, origin, reason)
	}
	after := e.rows(titleID)
	if after[video].TrashEntryID == 0 || after[sub].TrashEntryID == 0 {
		t.Error("the chapter and its subtitle are marked as trashed in the registry")
	}
	if !e.inLibrary(titleID) {
		t.Error("the title stays in the library with its other chapter")
	}
	e.assertNothingJournaled()
}

// TestTrashingATitleTakesItOutOfTheLibraryAndLinkingItToTMDBSaysInTrash checks RF-30: a title whose files are all in
// the trash is not in the library, and a title of TMDB that the trash holds is answered as in_trash (RF-44).
func TestTrashingATitleTakesItOutOfTheLibraryAndLinkingItToTMDBSaysInTrash(t *testing.T) {
	t.Parallel()
	e := newEditEnv(t, nil)
	video := e.write(ironFolder+"/Iron Man (2008).mkv", "iron")
	titleID := e.seedMovie(ironFolder, 1726, true, e.video(domain.KindMovie, video, domain.Place{}, ""))

	if err := e.edits.Trash(e.ctx, titleID, application.Scope{Kind: application.ScopeTitle}); err != nil {
		t.Fatalf("Trash: %v", err)
	}
	if e.inLibrary(titleID) {
		t.Error("a title in the trash is not in the library")
	}
	if _, err := e.edits.Plan(e.ctx, titleID, application.EditRequest{}); err == nil {
		t.Error("the trashed title cannot be edited: it is not in the library")
	}

	// Another title of the library asks for the TMDB title the trash holds.
	other := e.write("Movies/Free Title (2010)/Free Title (2010).mkv", "free")
	freeID := recordTitle(t, e.conn, mustTitle(t, domain.KindMovie, "Free Title", "Movies/Free Title (2010)", 0, false),
		e.video(domain.KindMovie, other, domain.Place{}, ""))
	req := movieChoices(e.rows(freeID), application.TitleChoice{Source: "tmdb", Kind: domain.KindMovie, TMDBID: 1726})
	p := e.plan(freeID, req)
	if p.Valid || len(p.Errors) != 1 || p.Errors[0].Code != "in_trash" {
		t.Errorf("a TMDB title the trash holds is in_trash: valid=%v errors=%+v", p.Valid, p.Errors)
	}
}
