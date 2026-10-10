package main

import (
	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/platform/config"
	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/platform/httpapi"
	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/platform/system"
)

// newHealth wires the startup diagnostics of RF-63: the library and data
// folders of the configuration, and the external programs 7zz and ffprobe
// from the PATH (RNF-03a). TMDB is not diagnosed.
func newHealth(cfg config.Config) *system.Handler {
	runner := system.ExecRunner{}
	return system.NewHandler(
		system.WritableDir("library", httpapi.LibraryNotWritable, cfg.LibraryPath),
		system.WritableDir("data", httpapi.DataNotWritable, cfg.DataPath),
		system.Tool("7zz", httpapi.SevenzipMissing, runner, "7zz", "i"),
		system.Tool("ffprobe", httpapi.FfprobeMissing, runner, "ffprobe", "-version"),
	)
}
