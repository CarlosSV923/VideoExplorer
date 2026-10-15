package domain

import (
	"path"
	"strings"
)

// The layout of a title under edit (spec RF-44, RF-22). Unlike an assignment, a title already has the
// files it keeps: several whole movies are versions of each other (RF-24), so only the places clash here:
// a part and a whole movie of one title, two parts with the same number, and chapters that overlap.
// Pure: no I/O.

// PlacementMoves returns the moves that place a video and its subtitles at their final paths (spec §5),
// with version as its <versión>: the video first, then its subtitles. The moves go from in.Source and
// in.Subtitles[i].Source to their names; an edit that leaves a file where it is gets a move with From == To.
func PlacementMoves(in Incoming, version string) ([]Move, error) {
	return placeIncoming(in, version)
}

// CheckEditLayout returns, for each item that clashes with another item of its title, the code of the
// clash. Movies: a part and a whole movie cannot share a title (RF-22); two parts with the same number
// clash (RF-22). Series: chapters that overlap in one season clash (CheckLayout). The first item keeps its
// place; a later one is the clash. Items are compared in the order given.
func CheckEditLayout(items []LayoutItem) map[string]string {
	out := map[string]string{}
	movies := map[string][]LayoutItem{}
	var series []LayoutItem
	for _, it := range items {
		switch it.Kind {
		case KindMovie:
			key := strings.ToLower(it.Folder)
			movies[key] = append(movies[key], it)
		case KindSeries:
			series = append(series, it)
		}
	}
	for _, group := range movies {
		hasWhole, hasPart := false, false
		for _, it := range group {
			if it.Place.Part == 0 {
				hasWhole = true
			} else {
				hasPart = true
			}
		}
		parts := map[int]bool{}
		for _, it := range group {
			switch {
			case hasWhole && hasPart:
				out[it.File] = CodePartsAndWhole
			case it.Place.Part > 0 && parts[it.Place.Part]:
				out[it.File] = CodeDuplicateTarget
			case it.Place.Part > 0:
				parts[it.Place.Part] = true
			}
		}
	}
	for file, code := range CheckLayout(series) {
		out[file] = code
	}
	return out
}

// TitleEntryName returns the name that a title's folder takes in No asignados when the whole title is
// moved there: the folder's name as it is (RF-44, as the import of RF-29 keeps it).
func TitleEntryName(folder string) string {
	return path.Base(folder)
}

// SeasonEntryName returns the name of the entry that a season takes in No asignados (RF-44):
// "<Título> (<Año>) Season NN", with the disc title and year of the title and no tag of ID, so that it is
// not taken for the title itself.
func SeasonEntryName(t LibraryTitle, season int) (string, error) {
	head, year, _, err := titleParts(Title{Name: t.DiscTitle, Year: t.Year})
	if err != nil {
		return "", err
	}
	return fitHead(head, year+" "+seasonFolder(season), ""), nil
}

// SubtitledEntryName returns the name of the folder that a video with subtitles takes in No asignados
// (RF-44): the video's name without its extension.
func SubtitledEntryName(videoRel string) string {
	return stemOf(path.Base(videoRel))
}
