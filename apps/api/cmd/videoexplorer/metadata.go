package main

import (
	"database/sql"
	"fmt"
	"log/slog"
	"path/filepath"
	"time"

	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/metadata/application"
	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/metadata/infrastructure/imagecache"
	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/metadata/infrastructure/sqlite"
	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/metadata/infrastructure/tmdb"
	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/platform/config"
)

// metadataWiring is the metadata context as the composition root builds it.
type metadataWiring struct {
	Service *application.Service
	Images  application.ImageStore
}

// newMetadata wires TMDB (spec §6). The client exists only when a credential
// is configured: without one, the service answers ErrNotConfigured and images
// are served only from the cache. With one, the client is wrapped by the
// response cache, and the images are downloaded from the TMDB CDN into
// DATA_PATH/cache/images.
func newMetadata(cfg config.Config, conn *sql.DB, log *slog.Logger) (metadataWiring, error) {
	var provider application.Provider
	var source application.ImageSource
	if cfg.TMDBAPIKey != "" || cfg.TMDBReadToken != "" {
		client, err := tmdb.New(tmdb.Config{APIKey: cfg.TMDBAPIKey, ReadToken: cfg.TMDBReadToken})
		if err != nil {
			return metadataWiring{}, fmt.Errorf("TMDB: %w", err)
		}
		provider = application.NewCachedProvider(client, sqlite.NewResponseStore(conn), time.Now, log)
		source = tmdb.NewImageSource(nil, "")
	}
	return metadataWiring{
		Service: application.NewService(provider, cfg.TMDBLanguage),
		Images:  imagecache.New(filepath.Join(cfg.DataPath, "cache", "images"), source),
	}, nil
}
