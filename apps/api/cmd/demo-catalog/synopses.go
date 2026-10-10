package main

import (
	"bytes"
	"encoding/json"
	"os"
)

// synopsesFile is the project's own synopses, versioned next to the generator
// (spec RF-71): one short text in Spanish and English per movie and series,
// keyed by TMDB id. The generator never copies TMDB overviews into it.
type synopsesFile struct {
	Movies map[int64]localized `json:"movies"`
	Series map[int64]localized `json:"series"`
}

// loadSynopses reads the synopses file. Unknown fields are an error, so a typo
// such as "EN" or "sinopsis" cannot pass unnoticed.
func loadSynopses(path string) (synopsesFile, error) {
	raw, err := os.ReadFile(path) //nolint:gosec // developer-supplied path of a committed file
	if err != nil {
		return synopsesFile{}, err
	}
	return parseSynopses(raw)
}

func parseSynopses(raw []byte) (synopsesFile, error) {
	var s synopsesFile
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&s); err != nil {
		return synopsesFile{}, err
	}
	return s, nil
}
