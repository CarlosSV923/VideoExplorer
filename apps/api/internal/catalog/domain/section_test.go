package domain_test

import (
	"reflect"
	"testing"
	"time"

	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/catalog/domain"
)

var sectionNow = time.Date(2026, time.October, 13, 9, 0, 0, 0, time.UTC)

func TestIgnoredMarksTheJunkOfTheSection(t *testing.T) {
	t.Parallel()
	tests := []struct {
		rel  string
		want bool
	}{
		{rel: "Movie.mkv", want: false},
		{rel: "Season 01/Pilot.mkv", want: false},
		{rel: "Resampled.mkv", want: false},
		{rel: "My.Movie.DS_Store", want: false},
		{rel: "", want: false},
		{rel: ".DS_Store", want: true},
		{rel: "Thumbs.db", want: true},
		{rel: "thumbs.DB", want: true},
		{rel: "desktop.ini", want: true},
		{rel: "Folder/Thumbs.db", want: true},
		{rel: "@eaDir/x.jpg", want: true},
		{rel: "Show/#recycle/old.mkv", want: true},
		{rel: "$RECYCLE.BIN/x", want: true},
		{rel: ".hidden/movie.mkv", want: true},
	}
	for _, tt := range tests {
		t.Run(tt.rel, func(t *testing.T) {
			t.Parallel()
			if got := domain.Ignored(tt.rel); got != tt.want {
				t.Errorf("Ignored(%q) = %v, want %v", tt.rel, got, tt.want)
			}
		})
	}
}

func TestCopyingTellsAFileStillBeingCopied(t *testing.T) {
	t.Parallel()
	old := sectionNow.Add(-61 * time.Second)
	tests := []struct {
		name string
		prev *domain.Stamp
		cur  domain.Stamp
		want bool
	}{
		{name: "modified 30 s ago, never read", cur: domain.Stamp{Size: 10, ModTime: sectionNow.Add(-30 * time.Second)}, want: true},
		{name: "modified 61 s ago, never read", cur: domain.Stamp{Size: 10, ModTime: old}, want: false},
		{name: "exactly one minute old", cur: domain.Stamp{Size: 10, ModTime: sectionNow.Add(-time.Minute)}, want: false},
		{name: "modified in the future (clock skew) does not hold it", cur: domain.Stamp{Size: 10, ModTime: sectionNow.Add(5 * time.Minute)}, want: false},
		{name: "size changed since the last reading", prev: &domain.Stamp{Size: 10, ModTime: old}, cur: domain.Stamp{Size: 20, ModTime: old}, want: true},
		{name: "mtime changed since the last reading", prev: &domain.Stamp{Size: 10, ModTime: old}, cur: domain.Stamp{Size: 10, ModTime: old.Add(time.Second)}, want: true},
		{name: "unchanged and old", prev: &domain.Stamp{Size: 10, ModTime: old}, cur: domain.Stamp{Size: 10, ModTime: old}, want: false},
		{name: "unchanged to the millisecond", prev: &domain.Stamp{Size: 10, ModTime: old}, cur: domain.Stamp{Size: 10, ModTime: old.Add(500 * time.Microsecond)}, want: false},
		{name: "unchanged but modified just now", prev: &domain.Stamp{Size: 10, ModTime: sectionNow}, cur: domain.Stamp{Size: 10, ModTime: sectionNow}, want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := domain.Copying(tt.prev, tt.cur, sectionNow); got != tt.want {
				t.Errorf("Copying = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestEntryStatusPutsCopyingFirst(t *testing.T) {
	t.Parallel()
	tests := []struct {
		copying, assigning bool
		want               string
	}{
		{want: domain.StatusReady},
		{assigning: true, want: domain.StatusAssigning},
		{copying: true, want: domain.StatusCopying},
		{copying: true, assigning: true, want: domain.StatusCopying},
	}
	for _, tt := range tests {
		if got := domain.EntryStatus(tt.copying, tt.assigning); got != tt.want {
			t.Errorf("EntryStatus(%v, %v) = %q, want %q", tt.copying, tt.assigning, got, tt.want)
		}
	}
}

// item is a disk item of the test tree: a folder when dir is set.
func item(rel string, dir bool, size int64) domain.Item {
	return domain.Item{Rel: rel, IsDir: dir, Size: size, ModTime: sectionNow}
}

func TestEntriesGroupsTheFirstLevelOfTheSection(t *testing.T) {
	t.Parallel()
	tree := []domain.Item{
		item("Movie.mkv", false, 5),
		item("Show", true, 0),
		item("Show/S01", true, 0),
		item("Show/S01/e1.mkv", false, 7),
		item("Show/Thumbs.db", false, 1),
		item("Show/@eaDir", true, 0),
		item("Show/@eaDir/x.jpg", false, 2),
		item("Other", true, 0),
		item("Thumbs.db", false, 3),
		item(".DS_Store", false, 3),
		item("#recycle", true, 0),
		item("#recycle/old.mkv", false, 9),
	}
	got := domain.Entries(tree)
	type view struct {
		name  string
		loose bool
		items []string
	}
	var views []view
	for _, f := range got {
		v := view{name: f.Name, loose: f.Loose}
		for _, it := range f.Items {
			v.items = append(v.items, it.Rel)
		}
		views = append(views, v)
	}
	want := []view{
		{name: "Movie.mkv", loose: true, items: []string{""}},
		{name: "Other"},
		{name: "Show", items: []string{"S01", "S01/e1.mkv", "Thumbs.db", "@eaDir", "@eaDir/x.jpg"}},
	}
	if !reflect.DeepEqual(views, want) {
		t.Errorf("Entries = %+v, want %+v", views, want)
	}
}

func TestFilesListsOnlyWhatIsNotJunk(t *testing.T) {
	t.Parallel()
	items := []domain.Item{
		item("S01", true, 0),
		item("S01/e1.mkv", false, 7),
		item("S01/Thumbs.db", false, 1),
		item("@eaDir", true, 0),
		item("@eaDir/x.jpg", false, 2),
	}
	var rels []string
	for _, it := range domain.Files(items) {
		rels = append(rels, it.Rel)
	}
	if want := []string{"S01/e1.mkv"}; !reflect.DeepEqual(rels, want) {
		t.Errorf("Files = %v, want %v", rels, want)
	}
}

func TestVacateWorksOutWhatLeavesAFolder(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name        string
		items       []domain.Item
		gone        []string
		empty       bool
		wantJunk    []string
		wantFolders []string
	}{
		{
			name:  "a folder that holds junk stays",
			items: []domain.Item{item("A", true, 0), item("A/x.mkv", false, 1), item("A/Thumbs.db", false, 1)},
			gone:  []string{"A/x.mkv"},
		},
		{
			name:        "a folder emptied by the move goes",
			items:       []domain.Item{item("A", true, 0), item("A/x.mkv", false, 1), item("B", true, 0), item("B/y.mkv", false, 1)},
			gone:        []string{"A/x.mkv"},
			wantFolders: []string{"A"},
		},
		{
			name:        "a parent goes when its last child folder empties",
			items:       []domain.Item{item("A", true, 0), item("A/S", true, 0), item("A/S/x.mkv", false, 1)},
			gone:        []string{"A/S/x.mkv"},
			wantFolders: []string{"A/S", "A"},
		},
		{
			name: "a parent with another child stays",
			items: []domain.Item{
				item("A", true, 0), item("A/S", true, 0), item("A/S/x.mkv", false, 1), item("A/y.mkv", false, 1),
			},
			gone:        []string{"A/S/x.mkv"},
			wantFolders: []string{"A/S"},
		},
		{
			name: "an empty entry sends its outermost junk and its folders out",
			items: []domain.Item{
				item("A", true, 0), item("A/Thumbs.db", false, 1), item("A/@eaDir", true, 0), item("A/@eaDir/x.jpg", false, 2),
				item("B", true, 0), item("B/.DS_Store", false, 1), item("Thumbs.db", false, 1),
			},
			empty:       true,
			wantJunk:    []string{"A/@eaDir", "A/Thumbs.db", "B/.DS_Store", "Thumbs.db"},
			wantFolders: []string{"A", "B"},
		},
		{
			name:        "an empty entry removes nested folders deepest first",
			items:       []domain.Item{item("A", true, 0), item("A/S", true, 0), item("A/S/x.mkv", false, 1)},
			gone:        []string{"A/S/x.mkv"},
			empty:       true,
			wantFolders: []string{"A/S", "A"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			gone := map[string]bool{}
			for _, rel := range tt.gone {
				gone[rel] = true
			}
			junk, folders := domain.Vacate(tt.items, gone, tt.empty)
			if !reflect.DeepEqual(junk, tt.wantJunk) {
				t.Errorf("junk = %v, want %v", junk, tt.wantJunk)
			}
			if !reflect.DeepEqual(folders, tt.wantFolders) {
				t.Errorf("folders = %v, want %v", folders, tt.wantFolders)
			}
		})
	}
}

func TestNewSambaEntryHasNoBatchAndNoRelation(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		loose   bool
		wantErr bool
	}{
		{name: "Iron Man (2008)", loose: false},
		{name: "Movie.mkv", loose: true},
		{name: "../escape", wantErr: true},
		{name: "", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := domain.NewSambaEntry(tt.name, tt.loose, sectionNow)
			if (err != nil) != tt.wantErr {
				t.Fatalf("NewSambaEntry err = %v, want error %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			if got.Origin != domain.OriginSamba || got.Relation != domain.RelationNone || got.BatchID != "" || got.Loose != tt.loose || got.Source != "samba:"+tt.name {
				t.Errorf("entry = %+v", got)
			}
		})
	}
}

func TestCopyingKnownLetsAnUploadThatArrivedCompleteSettle(t *testing.T) {
	t.Parallel()
	old := sectionNow.Add(-2 * time.Minute)
	fresh := sectionNow.Add(-10 * time.Second)
	tests := []struct {
		name   string
		origin string
		prev   *domain.Stamp
		cur    domain.Stamp
		want   bool
	}{
		{name: "an upload read for the first time, modified just now", origin: domain.OriginUpload, cur: domain.Stamp{Size: 10, ModTime: fresh}, want: false},
		{name: "an upload unchanged since its reading, modified just now", origin: domain.OriginUpload, prev: &domain.Stamp{Size: 10, ModTime: fresh}, cur: domain.Stamp{Size: 10, ModTime: fresh}, want: false},
		{name: "an upload whose size changed", origin: domain.OriginUpload, prev: &domain.Stamp{Size: 10, ModTime: old}, cur: domain.Stamp{Size: 20, ModTime: old}, want: true},
		{name: "an upload whose time changed", origin: domain.OriginUpload, prev: &domain.Stamp{Size: 10, ModTime: old}, cur: domain.Stamp{Size: 10, ModTime: fresh}, want: true},
		{name: "a Samba file read for the first time, modified just now", origin: domain.OriginSamba, cur: domain.Stamp{Size: 10, ModTime: fresh}, want: true},
		{name: "a Samba file unchanged, modified just now", origin: domain.OriginSamba, prev: &domain.Stamp{Size: 10, ModTime: fresh}, cur: domain.Stamp{Size: 10, ModTime: fresh}, want: true},
		{name: "a Samba file unchanged and old", origin: domain.OriginSamba, prev: &domain.Stamp{Size: 10, ModTime: old}, cur: domain.Stamp{Size: 10, ModTime: old}, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := domain.CopyingKnown(tt.origin, tt.prev, tt.cur, sectionNow); got != tt.want {
				t.Errorf("CopyingKnown = %v, want %v", got, tt.want)
			}
		})
	}
}
