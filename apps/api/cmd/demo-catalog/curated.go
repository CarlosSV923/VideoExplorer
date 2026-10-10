package main

import "github.com/CarlosSV923/VideoExplorer/apps/api/internal/metadata/domain"

// curatedTitle is one title picked by hand for the demo (spec RF-70, RF-71).
// original is the title TMDB must return as its original title: a wrong id
// fails the run instead of silently building another title.
type curatedTitle struct {
	id       int64
	kind     domain.MediaKind
	original string
}

// curated is the fixed selection of the demo. It covers several genres, the
// collections whose absent parts show dimmed (RF-40), a season 0 (Breaking
// Bad), titles whose original language is not Latin (Spirited Away, Parasite,
// Squid Game), a Latin title that is not English (Amélie, La casa de papel,
// Dark), and remakes with the same title (The Lion King, 1994 and 2019).
//
// Every collection of a movie in this list is added in full, so the demo can
// show all its films; those films get their own synopses (see synopses.json).
// Which titles the demo library holds is decided by the demo seed (phase 11),
// not here.
var curated = []curatedTitle{
	{id: 27205, kind: domain.KindMovie, original: "Inception"},
	{id: 155, kind: domain.KindMovie, original: "The Dark Knight"},
	{id: 1726, kind: domain.KindMovie, original: "Iron Man"},
	{id: 129, kind: domain.KindMovie, original: "千と千尋の神隠し"},
	{id: 496243, kind: domain.KindMovie, original: "기생충"},
	{id: 194, kind: domain.KindMovie, original: "Le Fabuleux Destin d'Amélie Poulain"},
	{id: 8587, kind: domain.KindMovie, original: "The Lion King"},
	{id: 420818, kind: domain.KindMovie, original: "The Lion King"},
	{id: 597, kind: domain.KindMovie, original: "Titanic"},
	{id: 419430, kind: domain.KindMovie, original: "Get Out"},
	{id: 545611, kind: domain.KindMovie, original: "Everything Everywhere All at Once"},
	{id: 438631, kind: domain.KindMovie, original: "Dune"},

	{id: 1396, kind: domain.KindSeries, original: "Breaking Bad"},
	{id: 93405, kind: domain.KindSeries, original: "오징어 게임"},
	{id: 71446, kind: domain.KindSeries, original: "La casa de papel"},
	{id: 70523, kind: domain.KindSeries, original: "Dark"},
	{id: 97546, kind: domain.KindSeries, original: "Ted Lasso"},
	{id: 87108, kind: domain.KindSeries, original: "Chernobyl"},
}
