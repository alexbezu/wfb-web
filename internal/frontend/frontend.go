package frontend

import (
	"bytes"
	"embed"
	"io/fs"
	"mime"
	"net/http"
	"path"
	"strings"
)

//go:embed dist
var dist embed.FS

func RegisterRoutes(mux *http.ServeMux) {
	assets, err := fs.Sub(dist, "dist")
	if err != nil {
		panic(err)
	}
	handler := spaHandler{assets: assets}
	mux.Handle("GET /app/", http.StripPrefix("/app/", handler))
	mux.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/app/", http.StatusTemporaryRedirect)
	})
}

type spaHandler struct {
	assets fs.FS
}

func (h spaHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimPrefix(path.Clean("/"+r.URL.Path), "/")
	if name == "" {
		name = "index.html"
	}
	if _, err := fs.Stat(h.assets, name); err != nil {
		if path.Ext(name) != "" {
			http.NotFound(w, r)
			return
		}
		name = "index.html"
	}
	h.serveFile(w, r, name)
}

func (h spaHandler) serveFile(w http.ResponseWriter, r *http.Request, name string) {
	info, err := fs.Stat(h.assets, name)
	if err != nil || info.IsDir() {
		http.NotFound(w, r)
		return
	}
	data, err := fs.ReadFile(h.assets, name)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if contentType := mime.TypeByExtension(path.Ext(name)); contentType != "" {
		w.Header().Set("Content-Type", contentType)
	}
	http.ServeContent(w, r, name, info.ModTime(), bytes.NewReader(data))
}
