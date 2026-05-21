package api

import (
	"io/fs"
	"net/http"

	"github.com/amorken/minidon/internal/static"
)

func NewRouter(staticFS fs.FS) *http.ServeMux {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /api/timeline", func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "not implemented", http.StatusNotImplemented)
	})
	mux.HandleFunc("GET /api/search", func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "not implemented", http.StatusNotImplemented)
	})
	mux.HandleFunc("GET /api/stream", func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "not implemented", http.StatusNotImplemented)
	})
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc("GET /readyz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	spaHandler := static.NewHandler(staticFS)
	mux.Handle("GET /", spaHandler)

	return mux
}
