package domain

import (
	"regexp"
	"strings"
)

// languageTag matches an ISO 639-1 code with an optional region: "es", "es-MX" or "es-419".
var languageTag = regexp.MustCompile(`^([A-Za-z]{2})(?:-([A-Za-z]{2}|[0-9]{3}))?$`)

// jellyfinRegions are the regions Jellyfin 12.2 recognizes in a subtitle's language (spike 12.2).
var jellyfinRegions = map[string]bool{
	"pt-BR":  true,
	"pt-PT":  true,
	"fr-CA":  true,
	"zh-TW":  true,
	"es-419": true,
}

// latinAmericanSpanish are the Spanish regions that Jellyfin does not know and that become es-419
// (spec §5, decision 2026-10-10).
var latinAmericanSpanish = map[string]bool{
	"es-AR": true, "es-BO": true, "es-CL": true, "es-CO": true, "es-CR": true,
	"es-CU": true, "es-DO": true, "es-EC": true, "es-SV": true, "es-GT": true,
	"es-HN": true, "es-MX": true, "es-NI": true, "es-PA": true, "es-PY": true,
	"es-PE": true, "es-PR": true, "es-UY": true, "es-VE": true,
}

// NormalizeLanguage returns the language <idioma> of a subtitle as its file name writes it
// (spec §5): ISO 639-1 with a region only when Jellyfin recognizes it. Latin American Spanish
// regions become es-419; any other region is dropped (es-ES becomes es, en-US becomes en).
// The language is written in lower case and the region in upper case. A tag that is not
// ISO 639-1 with an optional region is rejected with a NameError for "language".
func NormalizeLanguage(tag string) (string, error) {
	m := languageTag.FindStringSubmatch(tag)
	if m == nil {
		return "", &NameError{Field: "language"}
	}
	lang := strings.ToLower(m[1])
	if m[2] == "" {
		return lang, nil
	}
	full := lang + "-" + strings.ToUpper(m[2])
	switch {
	case jellyfinRegions[full]:
		return full, nil
	case latinAmericanSpanish[full]:
		return "es-419", nil
	default:
		return lang, nil
	}
}
