package domain

import (
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"
)

// FallbackName is the name of an unassigned entry whose name sanitizes to nothing
// (phase 4 decision, not in spec §5): the entry is still named, not left unnamed.
const FallbackName = "Sin nombre"

// dashes replaces the characters that cannot be in a name on every system (spec §5, steps 3 and 4).
var dashes = strings.NewReplacer("/", "-", `\`, "-", `"`, "'")

// SanitizeName applies the steps 1 to 6 of spec §5 to one path component: a title, a
// tag or the name of an entry. The result may be empty when nothing is left of the
// name. The steps run in order:
//
//  1. Unicode NFC: accents, ñ and symbols such as ™ are kept.
//  2. ": " becomes " - " and ":" becomes "-".
//  3. "/" and "\" become "-".
//  4. '"' becomes "'".
//  5. "?", "*", "<", ">", "|" and the control characters are removed.
//  6. Each block of white space becomes one space; the leading spaces and dots, and the
//     trailing spaces and dots, of the whole component are removed, not those of each title
//     in it: a name never starts with a dot (RF-28 hides dot names).
func SanitizeName(s string) string {
	s = norm.NFC.String(s)
	s = strings.ReplaceAll(s, ": ", " - ")
	s = strings.ReplaceAll(s, ":", "-")
	s = dashes.Replace(s)
	s = strings.Map(func(r rune) rune {
		if strings.ContainsRune("?*<>|", r) || unicode.IsControl(r) {
			return -1
		}
		return r
	}, s)
	s = collapseSpaces(s)
	return strings.TrimRight(strings.TrimLeft(s, " ."), " .")
}

// EntryName returns the name of an unassigned entry from the name it was given:
// sanitized, or FallbackName when nothing is left of it.
func EntryName(raw string) string {
	if name := SanitizeName(raw); name != "" {
		return name
	}
	return FallbackName
}

// EntryNameFor returns the name an unassigned entry is given from the name it came with
// (spec RF-08): a folder takes it as EntryName does; a loose file keeps its extension and
// sanitizes the rest of its name.
func EntryNameFor(raw string, loose bool) string {
	if !loose {
		return EntryName(raw)
	}
	stem, ext := splitName(raw, true)
	return EntryName(stem) + ext
}

// collapseSpaces replaces every block of white space (the Unicode White_Space
// property, the same set as the TypeScript implementation) with one space.
func collapseSpaces(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	inSpace := false
	for _, r := range s {
		if unicode.Is(unicode.White_Space, r) {
			if !inSpace {
				b.WriteRune(' ')
			}
			inSpace = true
			continue
		}
		inSpace = false
		b.WriteRune(r)
	}
	return b.String()
}
