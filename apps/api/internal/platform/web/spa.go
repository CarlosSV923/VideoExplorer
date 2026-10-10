// Package web serves the React SPA embedded in the binary.
//
// `task build` copies apps/web/dist into ./dist before compiling. Without a
// build (plain `go run`), only dist/.gitkeep is embedded and a short page
// points to the Vite dev server instead.
package web

import (
	"embed"
	"io/fs"
	"net/http"
	"path"
	"strings"
)

//go:embed all:dist
var embedded embed.FS

// Handler serves the embedded SPA.
func Handler() http.Handler {
	dist, err := fs.Sub(embedded, "dist")
	if err != nil {
		panic(err) // the embed directive guarantees the directory exists
	}
	return NewHandler(dist)
}

const notBuiltPage = `<!doctype html><html lang="en"><meta charset="utf-8"><title>VideoExplorer</title>
<body style="font-family:system-ui;background:#141414;color:#fff;padding:40px">
<h1>VideoExplorer API</h1>
<p>The web interface is not included in this binary. For development use <code>task dev:web</code>
(http://localhost:5173); for a complete binary use <code>task build</code>.</p></body></html>`

// NewHandler serves files from fsys with SPA fallback: unknown paths get
// index.html so client-side routes work on reload. Paths under /api/ must
// never reach this handler; the router answers them before.
func NewHandler(fsys fs.FS) http.Handler {
	_, err := fs.Stat(fsys, "index.html")
	built := err == nil

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.Header().Set("Allow", "GET, HEAD")
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if !built {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write([]byte(notBuiltPage))
			return
		}

		name := strings.TrimPrefix(path.Clean("/"+r.URL.Path), "/")
		if name != "" && name != "index.html" && fs.ValidPath(name) {
			if info, err := fs.Stat(fsys, name); err == nil && !info.IsDir() {
				if strings.HasPrefix(name, "assets/") {
					// Vite fingerprints these file names, so they never change.
					w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
				}
				// name is cleaned, rooted and fs.ValidPath-checked; fsys is the embedded SPA.
				http.ServeFileFS(w, r, fsys, name) //nolint:gosec // see comment above
				return
			}
		}
		w.Header().Set("Cache-Control", "no-cache")
		serveIndex(w, r, fsys)
	})
}

func serveIndex(w http.ResponseWriter, r *http.Request, fsys fs.FS) {
	b, err := fs.ReadFile(fsys, "index.html")
	if err != nil {
		http.Error(w, "index.html missing", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if r.Method == http.MethodHead {
		return
	}
	_, _ = w.Write(b)
}
