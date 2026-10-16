package main

import (
	"log/slog"

	mediaserversapplication "github.com/CarlosSV923/VideoExplorer/apps/api/internal/mediaservers/application"
	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/mediaservers/infrastructure/jellyfin"
	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/platform/config"
)

// newMediaServer wires the generic notifier port to the configured adapter.
// An empty Jellyfin configuration deliberately produces a no-op service.
func newMediaServer(cfg config.Config, log *slog.Logger) (*mediaserversapplication.Service, error) {
	adapter, err := jellyfin.New(jellyfin.Config{
		URL:               cfg.JellyfinURL,
		APIKey:            cfg.JellyfinAPIKey,
		LibraryPath:       cfg.LibraryPath,
		MoviesDir:         cfg.MoviesDir,
		SeriesDir:         cfg.SeriesDir,
		ForcedLibraryPath: cfg.JellyfinLibraryPath,
	})
	if err != nil {
		return nil, err
	}
	return mediaserversapplication.NewService(adapter, nil, log), nil
}
