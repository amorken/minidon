// Package static handles serving the compiled single-page application and other
// static assets from an embedded filesystem.
package static

import (
	"io/fs"
	"net/http"
	"path"
	"strings"
)

func NewHandler(fsys fs.FS) http.Handler {
	sub, err := fs.Sub(fsys, "web/dist")
	if err != nil {
		panic("static: cannot create sub FS: " + err.Error())
	}
	fileServer := http.FileServer(http.FS(sub))

	serveSPA := func(w http.ResponseWriter, r *http.Request) {
		r.URL.Path = "/"
		setCacheHeaders(w, "/index.html")
		fileServer.ServeHTTP(w, r)
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		cleanPath := path.Clean("/" + r.URL.Path)

		if strings.HasPrefix(cleanPath, "/api/") {
			http.NotFound(w, r)
			return
		}

		name := strings.TrimPrefix(cleanPath, "/")
		f, err := sub.Open(name)
		if err != nil {
			serveSPA(w, r)
			return
		}
		stat, statErr := f.Stat()
		f.Close()
		if statErr != nil || stat.IsDir() {
			serveSPA(w, r)
			return
		}

		setCacheHeaders(w, cleanPath)
		fileServer.ServeHTTP(w, r)
	})
}

func setCacheHeaders(w http.ResponseWriter, cleanPath string) {
	switch {
	case cleanPath == "/index.html":
		w.Header().Set("Cache-Control", "no-cache")
	case strings.HasPrefix(cleanPath, "/assets/"):
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	default:
		w.Header().Set("Cache-Control", "public, max-age=300")
	}
}
