// Command demo-catalog builds the fixed catalog used by the backend-less demo
// (spec RF-70, RF-71). It reads TMDB only for the structure of each title and
// merges the synopses the project wrote (synopses.json). A developer runs it
// with a TMDB credential, and its output is committed: the demo never talks to
// TMDB and the file carries no TMDB texts or images.
//
//	TMDB_READ_TOKEN=… go run ./cmd/demo-catalog
//	task demo-catalog
//
// TMDB_READ_TOKEN is used when both credentials are set, because the client
// accepts exactly one of them.
package main

import (
	"context"
	"errors"
	"flag"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	env "github.com/caarlos0/env/v11"

	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/metadata/infrastructure/tmdb"
)

// buildTimeout bounds the whole run: about a hundred requests at the client's rate.
const buildTimeout = 10 * time.Minute

type credentials struct {
	ReadToken string `env:"TMDB_READ_TOKEN"`
	APIKey    string `env:"TMDB_API_KEY"`
}

func main() {
	out := flag.String("out", "../web/src/modules/metadata/infrastructure/demo/catalog.json", "output file")
	synopses := flag.String("synopses", "cmd/demo-catalog/synopses.json", "the project's synopses")
	flag.Parse()

	log := slog.New(slog.NewTextHandler(os.Stderr, nil))
	if err := run(*out, *synopses, log); err != nil {
		log.Error("demo catalog failed", "error", err)
		os.Exit(1)
	}
}

func run(out, synopsesPath string, log *slog.Logger) error {
	var creds credentials
	if err := env.Parse(&creds); err != nil {
		return err
	}
	cfg := tmdb.Config{ReadToken: creds.ReadToken}
	if cfg.ReadToken == "" {
		cfg.APIKey = creds.APIKey
	}
	client, err := tmdb.New(cfg)
	if err != nil {
		return errors.New("set TMDB_READ_TOKEN or TMDB_API_KEY to generate the demo catalog")
	}
	syn, err := loadSynopses(synopsesPath)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), buildTimeout)
	defer cancel()
	cat, err := build(ctx, client, curated, syn)
	if err != nil {
		return err
	}
	data, err := marshal(cat)
	if err != nil {
		return err
	}
	if err := checkNoTMDBTexts(data); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(out), 0o750); err != nil {
		return err
	}
	if err := os.WriteFile(out, data, 0o644); err != nil { //nolint:gosec // committed source file
		return err
	}
	movies, series := countTypes(cat)
	log.Info("demo catalog written", "file", out, "movies", movies, "series", series)
	return nil
}

func countTypes(c catalog) (movies, series int) {
	for _, t := range c.Titles {
		if t.Type == typeMovie {
			movies++
		} else {
			series++
		}
	}
	return movies, series
}
