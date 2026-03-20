//go:build embed

package static

import (
	"embed"
	"io/fs"
	"net/http"
	"strings"
)

//go:embed all:files
var embeddedFiles embed.FS

// Handler returns an http.Handler that serves embedded static files with SPA
// fallback: any path not matching a real file is served as index.html.
func Handler() http.Handler {
	fsys, err := fs.Sub(embeddedFiles, "files")
	if err != nil {
		panic("embedded static files not available: " + err.Error())
	}
	fileServer := http.FileServer(http.FS(fsys))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/")
		if path == "" {
			path = "index.html"
		}
		if _, err := fs.Stat(fsys, path); err != nil {
			// SPA fallback — serve index.html for client-side routing
			r2 := r.Clone(r.Context())
			r2.URL.Path = "/"
			fileServer.ServeHTTP(w, r2)
			return
		}
		fileServer.ServeHTTP(w, r)
	})
}
