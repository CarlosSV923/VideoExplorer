package domain

import (
	"path"
	"strings"
)

// ChapterTitleFromName reads the <título cap.> of a series file from its name (spec §5, RF-44): the
// registry does not keep it, so the disk does. The name is the one spec §5 writes,
// "<T> (<A>) - S<SS>E<EE>[-E<FF>] - <título cap.>[ - <versión>].<ext>". It returns "" when the name has
// no title after the chapter code (the title is then absent) or when the name does not have the form.
func ChapterTitleFromName(rel string, season, first, last int, version string) string {
	code, err := episodeCode(season, first, last)
	if err != nil {
		return ""
	}
	stem := stemOf(path.Base(rel))
	marker := " - " + code
	i := strings.Index(stem, marker)
	if i < 0 {
		return ""
	}
	rest := stem[i+len(marker):]
	if version != "" && strings.HasSuffix(rest, " - "+version) {
		rest = rest[:len(rest)-len(" - "+version)]
	}
	if rest == "" {
		return ""
	}
	if !strings.HasPrefix(rest, " - ") {
		return ""
	}
	return strings.TrimPrefix(rest, " - ")
}
