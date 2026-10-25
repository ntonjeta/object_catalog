package httpapi

import (
	"bytes"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"biblio-oggetto/web"
)

func (s *Server) registerStatic(mux *http.ServeMux) {
	index, err := fs.ReadFile(web.FS, "index.html")
	if err != nil {
		panic("read embedded index.html: " + err.Error())
	}
	assets, err := fs.Sub(web.FS, "static")
	if err != nil {
		panic("read embedded static assets: " + err.Error())
	}

	mux.HandleFunc("GET /static/", func(w http.ResponseWriter, r *http.Request) {
		name := strings.TrimPrefix(r.URL.Path, "/static/")
		if strings.HasPrefix(name, "uploads/") {
			s.serveUpload(w, r, name)
			return
		}
		if !fs.ValidPath(name) {
			writeError(w, errNotFoundRoute)
			return
		}
		data, err := fs.ReadFile(assets, name)
		if err != nil {
			writeError(w, errNotFoundRoute)
			return
		}
		w.Header().Set("Cache-Control", "no-cache")
		http.ServeContent(w, r, name, time.Time{}, bytes.NewReader(data))
	})

	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-cache")
		w.Write(index)
	})
}

func (s *Server) serveUpload(w http.ResponseWriter, r *http.Request, name string) {
	if s.uploadDir == "" {
		writeError(w, errNotFoundRoute)
		return
	}
	fileName := strings.TrimPrefix(name, "uploads/")
	if fileName == "" || strings.Contains(fileName, "/") || !fs.ValidPath(fileName) {
		writeError(w, errNotFoundRoute)
		return
	}
	file, err := os.Open(filepath.Join(s.uploadDir, fileName))
	if err != nil {
		writeError(w, errNotFoundRoute)
		return
	}
	defer file.Close()

	info, err := file.Stat()
	if err != nil || info.IsDir() {
		writeError(w, errNotFoundRoute)
		return
	}
	w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	http.ServeContent(w, r, fileName, info.ModTime(), file)
}
