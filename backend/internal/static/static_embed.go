//go:build embed

package static

import (
	"embed"
	"io"
	"io/fs"
	"mime"
	"net/http"
	"path/filepath"
	"strings"
)

//go:embed all:files
var embeddedFiles embed.FS

// Handler returns an http.Handler that serves embedded static files with SPA
// fallback: any path not matching a real file is served as index.html.
//
// Next.js static export with trailingSlash:true generates pages as
// <route>/index.html. This handler resolves both /route and /route/ directly
// to <route>/index.html and serves it via http.ServeContent, bypassing
// http.FileServer's built-in redirect (any path ending in /index.html is
// redirected to its parent directory by http.FileServer, which would cause an
// infinite redirect loop).
func Handler() http.Handler {
	fsys, err := fs.Sub(embeddedFiles, "files")
	if err != nil {
		panic("embedded static files not available: " + err.Error())
	}
	// fileServer handles non-index assets (JS, CSS, images, fonts …).
	// Those paths never end in /index.html so the FileServer redirect rule
	// never fires for them.
	fileServer := http.FileServer(http.FS(fsys))

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resolved := resolveStaticPath(fsys, strings.TrimPrefix(r.URL.Path, "/"))

		if strings.HasSuffix(resolved, "index.html") {
			// Serve index files directly so http.FileServer's
			// "/index.html → /" redirect rule never triggers.
			serveFile(fsys, resolved, w, r)
			return
		}

		r2 := r.Clone(r.Context())
		r2.URL.Path = "/" + resolved
		fileServer.ServeHTTP(w, r2)
	})
}

// serveFile opens a file from fsys and writes it to w using http.ServeContent
// so that conditional GETs (If-Modified-Since, Range) work correctly.
func serveFile(fsys fs.FS, path string, w http.ResponseWriter, r *http.Request) {
	f, err := fsys.Open(path)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	// Set Content-Type explicitly; http.ServeContent infers from file name.
	ctype := mime.TypeByExtension(filepath.Ext(path))
	if ctype == "" {
		ctype = "text/html; charset=utf-8"
	}
	w.Header().Set("Content-Type", ctype)

	rs, ok := f.(io.ReadSeeker)
	if !ok {
		// Fallback: stream without range support (should not happen with embed.FS)
		w.WriteHeader(http.StatusOK)
		io.Copy(w, f) //nolint:errcheck
		return
	}
	http.ServeContent(w, r, info.Name(), info.ModTime(), rs)
}

// resolveStaticPath maps a URL path to a concrete file within fsys.
// Order of preference:
//  1. Exact file match (assets like /_next/static/…)
//  2. <path>/index.html  (Next.js trailingSlash pages)
//  3. index.html         (SPA fallback for unknown paths)
func resolveStaticPath(fsys fs.FS, path string) string {
	clean := strings.TrimSuffix(path, "/")
	if clean == "" {
		return "index.html"
	}
	// Exact file (not a directory)
	if info, err := fs.Stat(fsys, clean); err == nil && !info.IsDir() {
		return clean
	}
	// Directory index (trailingSlash pages)
	if idx := clean + "/index.html"; statOK(fsys, idx) {
		return idx
	}
	// SPA fallback
	return "index.html"
}

func statOK(fsys fs.FS, path string) bool {
	_, err := fs.Stat(fsys, path)
	return err == nil
}
