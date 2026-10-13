package http

import (
	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/catalog/application"
	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/catalog/domain"
	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/platform/httpapi"
)

// requestOf turns the body of a choice request into the application's request. It answers false
// when the body is missing or an action is not one of the two the contract names.
func requestOf(body *httpapi.AssignRequest) (application.Request, bool) {
	if body == nil {
		return application.Request{}, false
	}
	var out application.Request
	for _, v := range body.Videos {
		c := application.VideoChoice{
			File:         v.File,
			Part:         intOf(v.Part),
			Season:       v.Season,
			Chapter:      v.Chapter,
			LastChapter:  v.LastChapter,
			ChapterTitle: stringOf(v.ChapterTitle),
			Version:      stringOf(v.Version),
		}
		switch v.Action {
		case httpapi.AssignVideoAction("keep"):
			c.Keep = true
		case httpapi.AssignVideoAction("place"):
			if v.Title != nil {
				c.Title = titleOf(*v.Title)
			}
		default:
			return application.Request{}, false
		}
		if v.Duplicate != nil {
			c.Duplicate = domain.Decision(*v.Duplicate)
		}
		out.Videos = append(out.Videos, c)
	}
	for _, s := range body.Subtitles {
		out.Subtitles = append(out.Subtitles, application.SubtitleChoice{
			File:     s.File,
			Video:    stringOf(s.Video),
			Language: s.Language,
			Forced:   boolOf(s.Forced),
			SDH:      boolOf(s.Sdh),
		})
	}
	return out, true
}

// titleOf turns the title of a choice into the application's title choice.
func titleOf(t httpapi.TitleChoice) application.TitleChoice {
	c := application.TitleChoice{Source: string(t.Source), Title: stringOf(t.Title), Year: intOf(t.Year)}
	if t.LibraryTitleId != nil {
		c.LibraryID = *t.LibraryTitleId
	}
	if t.TmdbId != nil {
		c.TMDBID = *t.TmdbId
	}
	if t.Kind != nil {
		c.Kind = domain.TitleKind(*t.Kind)
	}
	return c
}

// suggestionsToAPI turns the suggestions of an entry into the contract.
func suggestionsToAPI(s application.Suggestions) httpapi.AssignSuggestions {
	out := httpapi.AssignSuggestions{
		EntryId:        s.EntryID,
		Name:           s.Name,
		Relation:       httpapi.BatchRelation(s.Relation),
		TmdbConfigured: s.TMDBConfigured,
		Videos:         []httpapi.VideoSuggestion{},
		Subtitles:      []httpapi.SubtitleSuggestion{},
	}
	if s.Title != nil {
		h := hitToAPI(*s.Title)
		out.Title = &h
	}
	for _, v := range s.Videos {
		item := httpapi.VideoSuggestion{File: v.File, Reading: readingToAPI(v.Reading)}
		if v.Match != nil {
			h := hitToAPI(*v.Match)
			item.Match = &h
		}
		out.Videos = append(out.Videos, item)
	}
	for _, sub := range s.Subtitles {
		item := httpapi.SubtitleSuggestion{File: sub.File, Language: sub.Language, Forced: sub.Forced, Sdh: sub.SDH}
		if sub.Video != "" {
			video := sub.Video
			item.Video = &video
		}
		out.Subtitles = append(out.Subtitles, item)
	}
	return out
}

// readingToAPI turns what a name says into the contract. Absent fields stay absent.
func readingToAPI(r application.NameReading) httpapi.NameReading {
	kind := "unknown"
	switch r.Kind {
	case domain.KindMovie:
		kind = "movie"
	case domain.KindSeries:
		kind = "series"
	}
	out := httpapi.NameReading{Type: httpapi.NameReadingType(kind), Season: r.Season, Chapter: r.Chapter, LastChapter: r.Last}
	if r.Title != "" {
		out.Title = &r.Title
	}
	if r.Year != 0 {
		out.Year = &r.Year
	}
	if r.Part != 0 {
		part := r.Part
		out.Part = &part
	}
	if r.Language != "" {
		out.Language = &r.Language
	}
	if r.Forced {
		out.Forced = &r.Forced
	}
	if r.SDH {
		out.Sdh = &r.SDH
	}
	return out
}

// planToAPI turns a plan into the contract.
func planToAPI(p application.Plan) httpapi.AssignPlan {
	out := httpapi.AssignPlan{
		EntryId:   p.EntryID,
		Valid:     p.Valid,
		Videos:    []httpapi.PlanVideo{},
		Subtitles: []httpapi.PlanSubtitle{},
		Errors:    issuesToAPI(p.Errors),
		Warnings:  issuesToAPI(p.Warnings),
	}
	for _, v := range p.Videos {
		item := httpapi.PlanVideo{
			File:       v.File,
			Keep:       v.Keep,
			Duplicates: []httpapi.PlanDuplicate{},
			ExactCopy:  v.ExactCopy,
			Allowed:    []httpapi.DuplicateDecision{},
		}
		if v.FinalPath != "" {
			item.FinalPath = &v.FinalPath
		}
		for _, d := range v.Duplicates {
			dup := httpapi.PlanDuplicate{Rel: d.Rel, Size: d.Size}
			if d.Version != "" {
				dup.Version = &d.Version
			}
			item.Duplicates = append(item.Duplicates, dup)
		}
		for _, a := range v.Allowed {
			item.Allowed = append(item.Allowed, httpapi.DuplicateDecision(a))
		}
		if v.Proposal != "" {
			p := httpapi.DuplicateDecision(v.Proposal)
			item.Proposal = &p
		}
		if v.Decision != "" {
			d := httpapi.DuplicateDecision(v.Decision)
			item.Decision = &d
		}
		if v.Version != "" {
			item.Version = &v.Version
		}
		if v.VersionProposal != "" {
			item.VersionProposal = &v.VersionProposal
		}
		if v.VersionIncomplete {
			incomplete := true
			item.VersionIncomplete = &incomplete
		}
		out.Videos = append(out.Videos, item)
	}
	for _, s := range p.Subtitles {
		item := httpapi.PlanSubtitle{File: s.File, Video: s.Video}
		if s.FinalPath != "" {
			item.FinalPath = &s.FinalPath
		}
		out.Subtitles = append(out.Subtitles, item)
	}
	return out
}

// issuesToAPI turns the issues of a plan into the contract.
func issuesToAPI(issues []application.FieldIssue) []httpapi.PlanIssue {
	out := make([]httpapi.PlanIssue, 0, len(issues))
	for _, i := range issues {
		item := httpapi.PlanIssue{Field: i.Field, Code: i.Code}
		if i.File != "" {
			file := i.File
			item.File = &file
		}
		out = append(out, item)
	}
	return out
}

// resultToAPI turns the result of a commit into the contract.
func resultToAPI(r application.Result) httpapi.AssignResult {
	out := httpapi.AssignResult{EntryId: r.EntryID, EntryRemoved: r.EntryRemoved, Placed: []httpapi.PlacedFile{}}
	for _, p := range r.Placed {
		out.Placed = append(out.Placed, httpapi.PlacedFile{File: p.File, Path: p.Path})
	}
	return out
}

// hitsToAPI turns titles that an assignment can take into the contract.
func hitsToAPI(hits []application.TitleHit) []httpapi.TitleHit {
	out := make([]httpapi.TitleHit, 0, len(hits))
	for _, h := range hits {
		out = append(out, hitToAPI(h))
	}
	return out
}

// hitToAPI turns one title that an assignment can take into the contract.
func hitToAPI(h application.TitleHit) httpapi.TitleHit {
	out := httpapi.TitleHit{Source: httpapi.TitleSource(h.Source), Kind: httpapi.TitleKind(h.Kind), Title: h.Title}
	if h.LibraryID != 0 {
		id := h.LibraryID
		out.LibraryTitleId = &id
	}
	if h.TMDBID != 0 {
		id := h.TMDBID
		out.TmdbId = &id
	}
	if h.Year != 0 {
		year := h.Year
		out.Year = &year
	}
	if h.PosterFile != "" {
		poster := h.PosterFile
		out.PosterFile = &poster
	}
	return out
}

// stringOf returns the value of an optional string, or "" when it is absent.
func stringOf(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// intOf returns the value of an optional number, or 0 when it is absent.
func intOf(n *int) int {
	if n == nil {
		return 0
	}
	return *n
}

// boolOf returns the value of an optional flag, or false when it is absent.
func boolOf(b *bool) bool {
	return b != nil && *b
}
