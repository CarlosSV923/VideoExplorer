package http

import (
	"testing"

	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/catalog/application"
	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/catalog/domain"
	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/platform/httpapi"
)

func TestTheBodyOfTheIDTagActionBecomesItsRequest(t *testing.T) {
	t.Parallel()

	replace := httpapi.DuplicateDecision("replace")
	version := "2160p"
	videos := []httpapi.IDTagVideo{
		{FileId: 4, Duplicate: &replace},
		{FileId: 9, Version: &version},
	}
	labels := []httpapi.AssignLabel{{Rel: "Movies/a.mkv", Version: "1080p"}}
	tests := []struct {
		name string
		body *httpapi.IDTagRequest
		want application.IDTagRequest
		ok   bool
	}{
		{"nothing sent", nil, application.IDTagRequest{}, false},
		{
			name: "the tag only",
			body: &httpapi.IDTagRequest{Revision: "r", IdTag: true},
			want: application.IDTagRequest{Revision: "r", IDTag: true},
			ok:   true,
		},
		{
			name: "the answers of the form",
			body: &httpapi.IDTagRequest{Revision: "r", IdTag: false, Videos: &videos, Labels: &labels},
			want: application.IDTagRequest{
				Revision: "r",
				Videos: []application.IDTagVideo{
					{FileID: 4, Duplicate: domain.DecisionReplace},
					{FileID: 9, Version: "2160p"},
				},
				Labels: []application.LabelChoice{{Rel: "Movies/a.mkv", Version: "1080p"}},
			},
			ok: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, ok := idTagRequestOf(tt.body)
			if ok != tt.ok {
				t.Fatalf("ok = %t, want %t", ok, tt.ok)
			}
			if got.Revision != tt.want.Revision || got.IDTag != tt.want.IDTag || len(got.Videos) != len(tt.want.Videos) || len(got.Labels) != len(tt.want.Labels) {
				t.Fatalf("request = %+v, want %+v", got, tt.want)
			}
			for i := range got.Videos {
				if got.Videos[i] != tt.want.Videos[i] {
					t.Errorf("video %d = %+v, want %+v", i, got.Videos[i], tt.want.Videos[i])
				}
			}
			for i := range got.Labels {
				if got.Labels[i] != tt.want.Labels[i] {
					t.Errorf("label %d = %+v, want %+v", i, got.Labels[i], tt.want.Labels[i])
				}
			}
		})
	}
}
