package domain_test

import (
	"testing"
	"time"

	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/catalog/domain"
)

func TestAssignLock(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 10, 13, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		name     string
		lock     domain.AssignLock
		token    string
		wantHeld bool
		wantBy   bool
	}{
		{name: "free entry", lock: domain.AssignLock{}, token: "a", wantHeld: false, wantBy: false},
		{name: "held by the tab", lock: domain.AssignLock{Token: "a", Until: now.Add(time.Minute)}, token: "a", wantHeld: true, wantBy: true},
		{name: "held by another tab", lock: domain.AssignLock{Token: "b", Until: now.Add(time.Minute)}, token: "a", wantHeld: true, wantBy: false},
		{name: "expired at the instant", lock: domain.AssignLock{Token: "a", Until: now}, token: "a", wantHeld: false, wantBy: false},
		{name: "expired long ago", lock: domain.AssignLock{Token: "a", Until: now.Add(-time.Hour)}, token: "a", wantHeld: false, wantBy: false},
		{name: "no token given", lock: domain.AssignLock{Token: "a", Until: now.Add(time.Minute)}, token: "", wantHeld: true, wantBy: false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := tc.lock.Held(now); got != tc.wantHeld {
				t.Errorf("Held = %v, want %v", got, tc.wantHeld)
			}
			if got := tc.lock.HeldBy(tc.token, now); got != tc.wantBy {
				t.Errorf("HeldBy = %v, want %v", got, tc.wantBy)
			}
		})
	}
}

func intp(n int) *int { return &n }

func TestPlaceOf(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		kind    domain.TitleKind
		part    int
		season  *int
		chapter *int
		last    *int
		want    domain.Place
		issue   domain.Issue
	}{
		{name: "whole movie", kind: domain.KindMovie, want: domain.Place{}},
		{name: "movie part", kind: domain.KindMovie, part: 2, want: domain.Place{Part: 2}},
		{name: "movie part too big", kind: domain.KindMovie, part: 1000, issue: domain.Issue{Field: "part", Code: "invalid"}},
		{name: "movie with a season", kind: domain.KindMovie, season: intp(1), issue: domain.Issue{Field: "place", Code: "kind_mismatch"}},
		{
			name: "chapter", kind: domain.KindSeries, season: intp(1), chapter: intp(5),
			want: domain.Place{Episode: true, Season: 1, First: 5, Last: 5},
		},
		{
			name: "chapters of one file", kind: domain.KindSeries, season: intp(1), chapter: intp(1), last: intp(2),
			want: domain.Place{Episode: true, Season: 1, First: 1, Last: 2},
		},
		{
			name: "specials", kind: domain.KindSeries, season: intp(0), chapter: intp(3),
			want: domain.Place{Episode: true, Season: 0, First: 3, Last: 3},
		},
		{name: "series without season", kind: domain.KindSeries, chapter: intp(1), issue: domain.Issue{Field: "season", Code: "required"}},
		{name: "series without chapter", kind: domain.KindSeries, season: intp(1), issue: domain.Issue{Field: "chapter", Code: "required"}},
		{name: "chapter zero", kind: domain.KindSeries, season: intp(1), chapter: intp(0), issue: domain.Issue{Field: "chapter", Code: "invalid"}},
		{
			name: "last before first", kind: domain.KindSeries, season: intp(1), chapter: intp(4), last: intp(3),
			issue: domain.Issue{Field: "lastChapter", Code: "invalid"},
		},
		{
			name: "series with a part", kind: domain.KindSeries, part: 1, season: intp(1), chapter: intp(1),
			issue: domain.Issue{Field: "place", Code: "kind_mismatch"},
		},
		{name: "unknown kind", kind: "", issue: domain.Issue{Field: "title", Code: "invalid"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, issue := domain.PlaceOf(tc.kind, tc.part, tc.season, tc.chapter, tc.last)
			if issue != tc.issue {
				t.Fatalf("issue = %+v, want %+v", issue, tc.issue)
			}
			if issue == (domain.Issue{}) && got != tc.want {
				t.Errorf("place = %+v, want %+v", got, tc.want)
			}
		})
	}
}

func TestCheckLayout(t *testing.T) {
	t.Parallel()
	const movie = "Movies/Dune (2021) [tmdbid-1]"
	const other = "Movies/Heat (1995) [tmdbid-2]"
	const show = "TV Shows/Lost (2004) [tmdbid-3]"
	ep := func(season, first, last int) domain.Place {
		return domain.Place{Episode: true, Season: season, First: first, Last: last}
	}
	part := func(n int) domain.Place { return domain.Place{Part: n} }
	tests := []struct {
		name  string
		items []domain.LayoutItem
		want  map[string]string
	}{
		{name: "no items", want: map[string]string{}},
		{name: "distinct parts of one movie", items: []domain.LayoutItem{
			{File: "a", Folder: movie, Kind: domain.KindMovie, Place: part(1)},
			{File: "b", Folder: movie, Kind: domain.KindMovie, Place: part(2)},
		}, want: map[string]string{}},
		{name: "same part twice", items: []domain.LayoutItem{
			{File: "a", Folder: movie, Kind: domain.KindMovie, Place: part(1)},
			{File: "b", Folder: movie, Kind: domain.KindMovie, Place: part(1)},
		}, want: map[string]string{"b": domain.CodeDuplicateTarget}},
		{name: "two whole movies", items: []domain.LayoutItem{
			{File: "a", Folder: movie, Kind: domain.KindMovie},
			{File: "b", Folder: movie, Kind: domain.KindMovie},
		}, want: map[string]string{"b": domain.CodeDuplicateTarget}},
		{name: "whole after parts", items: []domain.LayoutItem{
			{File: "a", Folder: movie, Kind: domain.KindMovie, Place: part(1)},
			{File: "b", Folder: movie, Kind: domain.KindMovie},
		}, want: map[string]string{"b": domain.CodePartsAndWhole}},
		{name: "part after whole", items: []domain.LayoutItem{
			{File: "a", Folder: movie, Kind: domain.KindMovie},
			{File: "b", Folder: movie, Kind: domain.KindMovie, Place: part(1)},
		}, want: map[string]string{"b": domain.CodePartsAndWhole}},
		{name: "same whole movie in another title is not a conflict", items: []domain.LayoutItem{
			{File: "a", Folder: movie, Kind: domain.KindMovie},
			{File: "b", Folder: other, Kind: domain.KindMovie},
		}, want: map[string]string{}},
		{name: "consecutive chapters", items: []domain.LayoutItem{
			{File: "a", Folder: show, Kind: domain.KindSeries, Place: ep(1, 1, 1)},
			{File: "b", Folder: show, Kind: domain.KindSeries, Place: ep(1, 2, 2)},
		}, want: map[string]string{}},
		{name: "chapters that overlap in one file", items: []domain.LayoutItem{
			{File: "a", Folder: show, Kind: domain.KindSeries, Place: ep(1, 1, 1)},
			{File: "b", Folder: show, Kind: domain.KindSeries, Place: ep(1, 1, 2)},
		}, want: map[string]string{"b": domain.CodeChaptersOverlap}},
		{name: "same chapter in another season is not a conflict", items: []domain.LayoutItem{
			{File: "a", Folder: show, Kind: domain.KindSeries, Place: ep(1, 1, 1)},
			{File: "b", Folder: show, Kind: domain.KindSeries, Place: ep(2, 1, 1)},
		}, want: map[string]string{}},
		{name: "folders compare without case", items: []domain.LayoutItem{
			{File: "a", Folder: movie, Kind: domain.KindMovie, Place: part(1)},
			{File: "b", Folder: "MOVIES/DUNE (2021) [TMDBID-1]", Kind: domain.KindMovie, Place: part(1)},
		}, want: map[string]string{"b": domain.CodeDuplicateTarget}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := domain.CheckLayout(tc.items)
			if len(got) != len(tc.want) {
				t.Fatalf("CheckLayout = %v, want %v", got, tc.want)
			}
			for file, code := range tc.want {
				if got[file] != code {
					t.Errorf("file %s: code = %q, want %q", file, got[file], code)
				}
			}
		})
	}
}

func TestTitleFolder(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		root  string
		title domain.Title
		want  string
	}{
		{
			name: "linked movie with year and tag", root: "Movies", want: "Movies/Iron Man (2008) [tmdbid-1726]",
			title: domain.Title{Name: "Iron Man", Year: 2008, TMDBID: 1726, IDTag: true},
		},
		{
			name: "free series without year", root: "TV Shows", want: "TV Shows/Lost",
			title: domain.Title{Name: "Lost"},
		},
		{
			name: "tag off", root: "Movies", want: "Movies/Heat (1995)",
			title: domain.Title{Name: "Heat", Year: 1995, TMDBID: 949},
		},
		{
			name: "name with a colon", root: "Movies", want: "Movies/Mission - Impossible (1996) [tmdbid-954]",
			title: domain.Title{Name: "Mission: Impossible", Year: 1996, TMDBID: 954, IDTag: true},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := domain.TitleFolder(tc.root, tc.title)
			if err != nil {
				t.Fatalf("TitleFolder: %v", err)
			}
			if got != tc.want {
				t.Errorf("TitleFolder = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestPlanPlacementAndProposeVersion(t *testing.T) {
	t.Parallel()
	in := domain.Incoming{
		Source: "_unassigned/Dune/Dune.mkv", Kind: domain.KindMovie, Root: "Movies",
		Title: domain.Title{Name: "Dune", Year: 2021, TMDBID: 1, IDTag: true}, Extension: ".mkv",
	}
	plan, err := domain.PlanPlacement([]domain.Incoming{in})
	if err != nil {
		t.Fatalf("PlanPlacement: %v", err)
	}
	if len(plan.Place) != 1 || plan.Place[0].To != "Movies/Dune (2021) [tmdbid-1]/Dune (2021) [tmdbid-1].mkv" {
		t.Errorf("Place = %+v", plan.Place)
	}
	if plan.Decision != "" || len(plan.Trash) != 0 || len(plan.Renames) != 0 || plan.Version != "" {
		t.Errorf("a placement with no duplicate has no decision, trash or renames: %+v", plan)
	}

	withVersion := in
	withVersion.Version = "2160p"
	if _, err := domain.PlanPlacement([]domain.Incoming{withVersion}); err == nil {
		t.Error("PlanPlacement accepted a version for a video with no duplicate")
	}

	// An occupant with no label and a new video of another resolution: both get labels.
	occupant := domain.Occupant{
		Video: domain.Existing{Rel: "Movies/Dune (2021) [tmdbid-1]/Dune (2021) [tmdbid-1].mkv", VersionKnown: true},
		Media: domain.VersionMedia{Width: 1920, Height: 1080, Codec: "h264", Audio: []domain.AudioMedia{{Language: "eng"}}},
	}
	newer := in
	newer.Media = domain.VersionMedia{Width: 3840, Height: 2160, Codec: "hevc", Audio: []domain.AudioMedia{{Language: "eng"}}}
	label := domain.ProposeVersion([]domain.Occupant{occupant}, newer)
	if label.Text != "2160p" || label.Incomplete {
		t.Errorf("ProposeVersion = %+v, want 2160p complete", label)
	}
}
