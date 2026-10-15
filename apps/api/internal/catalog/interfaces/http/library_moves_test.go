package http

import (
	"testing"

	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/catalog/application"
	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/platform/httpapi"
)

func TestTheScopeOfAMoveOrATrashNeedsItsNumberOrItsId(t *testing.T) {
	t.Parallel()
	season, file := 1, int64(7)
	tests := []struct {
		name string
		body *httpapi.LibraryScope
		want application.Scope
		ok   bool
	}{
		{"nothing sent", nil, application.Scope{}, false},
		{"the whole title", &httpapi.LibraryScope{Kind: httpapi.LibraryScopeKind("title")}, application.Scope{Kind: application.ScopeTitle}, true},
		{"a season with its number", &httpapi.LibraryScope{Kind: httpapi.LibraryScopeKind("season"), Season: &season}, application.Scope{Kind: application.ScopeSeason, Season: 1}, true},
		{"a season without its number", &httpapi.LibraryScope{Kind: httpapi.LibraryScopeKind("season")}, application.Scope{}, false},
		{"a file with its id", &httpapi.LibraryScope{Kind: httpapi.LibraryScopeKind("file"), FileId: &file}, application.Scope{Kind: application.ScopeFile, FileID: 7}, true},
		{"a file without its id", &httpapi.LibraryScope{Kind: httpapi.LibraryScopeKind("file")}, application.Scope{}, false},
		{"an unknown kind", &httpapi.LibraryScope{Kind: httpapi.LibraryScopeKind("library")}, application.Scope{}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, ok := scopeOf(tt.body)
			if ok != tt.ok || got != tt.want {
				t.Errorf("scopeOf = %+v, %v; want %+v, %v", got, ok, tt.want, tt.ok)
			}
		})
	}
}
