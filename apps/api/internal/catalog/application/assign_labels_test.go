package application_test

import (
	"path/filepath"
	"testing"

	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/catalog/application"
	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/catalog/domain"
	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/catalog/infrastructure/libraryfs"
)

// oldIronPath is the Iron Man (2008) version that the registry holds before the assignment.
const oldIronPath = "Movies/Iron Man (2008) [tmdbid-1726]/Iron Man (2008) [tmdbid-1726].mkv"

// oldIronEnv registers Iron Man with its old version and adds a 4K copy as a loose entry. readable says
// whether the media of both versions can be read, which decides whether the old version needs a label.
func oldIronEnv(t *testing.T, readable bool) (*assignEnv, int64) {
	t.Helper()
	e := newAssignEnv(t, func(root string) application.Files { return libraryfs.New(root) })
	e.tmdbMovie(1726, "Iron Man", "Iron Man", 2008)
	e.registerTitle(domain.LibraryTitle{
		Kind: domain.KindMovie, Title: "Iron Man", DiscTitle: "Iron Man", Year: 2008, TMDBID: 1726, IDTag: true,
		Folder: "Movies/Iron Man (2008) [tmdbid-1726]",
	}, map[string]string{oldIronPath: "old"})
	if readable {
		e.media[filepath.Base(oldIronPath)] = domain.VersionMedia{Width: 1920, Height: 1080, Codec: "h264", Audio: []domain.AudioMedia{{Language: "eng"}}}
		e.media["Iron.Man.4K.mkv"] = domain.VersionMedia{Width: 3840, Height: 2160, Codec: "hevc", Audio: []domain.AudioMedia{{Language: "eng"}}}
	}
	id := e.entry("Iron.Man.4K.mkv", domain.RelationNone, true, map[string]string{"": "new"})
	e.lockAs(id)
	return e, id
}

// keepBothWith is the request that keeps both versions: the new one is labelled 2160, and the labels given.
func keepBothWith(labels ...application.LabelChoice) application.Request {
	return application.Request{
		Videos: []application.VideoChoice{{File: "", Title: registeredTitle, Version: "2160", Duplicate: domain.DecisionKeepBoth}},
		Labels: labels,
	}
}

// hasIssueAt reports whether the plan has an issue of this kind about this version of the library.
func hasIssueAt(issues []application.FieldIssue, code, target string) bool {
	for _, i := range issues {
		if i.File == "" && i.Field == "version" && i.Code == code && i.Target == target {
			return true
		}
	}
	return false
}

func TestAnOldVersionWithNoReadableLabelNeedsOne(t *testing.T) {
	t.Parallel()
	e, id := oldIronEnv(t, false)
	plan := planOf(t, e, id, keepBothWith())
	if plan.Valid || !hasIssueAt(plan.Errors, "incomplete", oldIronPath) {
		t.Fatalf("errors = %+v, want the old version to need a label (RF-24)", plan.Errors)
	}
	if len(plan.Videos[0].Duplicates) != 1 || !plan.Videos[0].Duplicates[0].VersionNeeded {
		t.Errorf("the duplicate says it needs a label: %+v", plan.Videos[0].Duplicates)
	}
}

func TestTheLabelTheUserWritesRenamesTheOldVersion(t *testing.T) {
	t.Parallel()
	e, id := oldIronEnv(t, false)
	req := keepBothWith(application.LabelChoice{Rel: oldIronPath, Version: "1080"})
	if plan := planOf(t, e, id, req); !plan.Valid {
		t.Fatalf("a label for the old version makes the plan valid: %+v", plan.Errors)
	}
	if _, err := e.svc.Commit(t.Context(), id, "tab-a", req); err != nil {
		t.Fatalf("Commit: %v", err)
	}
	oldLabelled := "Movies/Iron Man (2008) [tmdbid-1726]/Iron Man (2008) [tmdbid-1726] - 1080.mkv"
	newLabelled := "Movies/Iron Man (2008) [tmdbid-1726]/Iron Man (2008) [tmdbid-1726] - 2160.mkv"
	if e.read(oldLabelled) != "old" || e.read(newLabelled) != "new" || e.exists(oldIronPath) {
		t.Error("the old version is renamed with its label, in the same change (RF-24)")
	}
	versions := map[string]string{}
	for _, f := range e.store.files {
		versions[f.Rel] = f.Version
	}
	if versions[oldLabelled] != "1080" || versions[newLabelled] != "2160" {
		t.Errorf("the registry knows both labels: %+v", versions)
	}
}

func TestALabelThatCollidesWithTheNewVersionGetsASuffix(t *testing.T) {
	t.Parallel()
	e, id := oldIronEnv(t, false)
	req := keepBothWith(application.LabelChoice{Rel: oldIronPath, Version: "2160"})
	if _, err := e.svc.Commit(t.Context(), id, "tab-a", req); err != nil {
		t.Fatalf("Commit: %v", err)
	}
	if e.read("Movies/Iron Man (2008) [tmdbid-1726]/Iron Man (2008) [tmdbid-1726] - 2160.mkv") != "old" ||
		e.read("Movies/Iron Man (2008) [tmdbid-1726]/Iron Man (2008) [tmdbid-1726] - 2160 (2).mkv") != "new" {
		t.Error("the new version takes the suffix (2) when the labels collide (RF-24)")
	}
}

func TestALabelWithNothingUsableIsRefused(t *testing.T) {
	t.Parallel()
	e, id := oldIronEnv(t, false)
	plan := planOf(t, e, id, keepBothWith(application.LabelChoice{Rel: oldIronPath, Version: "  . "}))
	if plan.Valid || !hasIssueAt(plan.Errors, "invalid", oldIronPath) {
		t.Errorf("errors = %+v, want the label refused as invalid", plan.Errors)
	}
}

func TestALabelForAVersionThatNeedsNoneIsRefused(t *testing.T) {
	t.Parallel()
	e, id := oldIronEnv(t, true)
	plan := planOf(t, e, id, keepBothWith(application.LabelChoice{Rel: oldIronPath, Version: "1080"}))
	if plan.Valid || !hasIssueAt(plan.Errors, "not_needed", oldIronPath) {
		t.Errorf("errors = %+v, want a label for a readable version refused", plan.Errors)
	}
}

func TestALabelForAVersionNotInThePlaceIsRefused(t *testing.T) {
	t.Parallel()
	e, id := oldIronEnv(t, false)
	const nowhere = "Movies/Heat (1995) [tmdbid-949]/Heat (1995) [tmdbid-949].mkv"
	plan := planOf(t, e, id, keepBothWith(application.LabelChoice{Rel: nowhere, Version: "1080"}))
	if plan.Valid || !hasIssueAt(plan.Errors, "unknown", nowhere) {
		t.Errorf("errors = %+v, want a label for an unknown version refused", plan.Errors)
	}
}

func TestReplacingIgnoresTheLabelsOfTheOldVersions(t *testing.T) {
	t.Parallel()
	e, id := oldIronEnv(t, false)
	req := application.Request{
		Videos: []application.VideoChoice{{File: "", Title: registeredTitle, Duplicate: domain.DecisionReplace}},
		Labels: []application.LabelChoice{{Rel: oldIronPath, Version: "1080"}},
	}
	if plan := planOf(t, e, id, req); !plan.Valid {
		t.Errorf("a replacement takes no label of the old version: %+v", plan.Errors)
	}
	if _, err := e.svc.Commit(t.Context(), id, "tab-a", req); err != nil {
		t.Fatalf("Commit: %v", err)
	}
}
