package http

import (
	"reflect"
	"testing"

	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/catalog/application"
)

func TestHTTPResponseMappersDoNotReturnNullArrays(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		value any
	}{
		{"library title summary", summaryToAPI(application.TitleSummary{})},
		{"library title detail", detailToAPI(application.TitleDetail{})},
		{"library collection", collectionToAPI(application.CollectionView{})},
		{"library technical details", technicalToAPI(application.TechnicalFile{})},
		{"unassigned section", sectionToAPI(nil)},
		{"scan statuses", scanStatusesToAPI(nil)},
		{"assignment suggestions", suggestionsToAPI(application.Suggestions{})},
		{"assignment plan", planToAPI(application.Plan{})},
		{"assignment result", resultToAPI(application.Result{})},
		{"assignment title hits", hitsToAPI(nil)},
		{"title edit plan", editPlanToAPI(application.EditPlan{})},
		{"title edit issues", editIssuesToAPI(nil)},
		{"trash restore plan", trashRestorePlanToAPI(application.TrashRestorePlan{})},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assertResponseSlicesNotNil(t, reflect.ValueOf(tt.value), tt.name)
		})
	}
}

func assertResponseSlicesNotNil(t *testing.T, value reflect.Value, path string) {
	t.Helper()
	switch value.Kind() {
	case reflect.Interface, reflect.Pointer:
		if !value.IsNil() {
			assertResponseSlicesNotNil(t, value.Elem(), path)
		}
	case reflect.Struct:
		for i := 0; i < value.NumField(); i++ {
			assertResponseSlicesNotNil(t, value.Field(i), path+"."+value.Type().Field(i).Name)
		}
	case reflect.Slice:
		if value.IsNil() {
			t.Errorf("%s is nil; JSON would contain null instead of an array", path)
			return
		}
		for i := 0; i < value.Len(); i++ {
			assertResponseSlicesNotNil(t, value.Index(i), path)
		}
	}
}
