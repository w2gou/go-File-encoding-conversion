package httpapi

import (
	"embed"
	"io/fs"
	"net/http"
)

//go:embed web/*
var embeddedWeb embed.FS

var embeddedWebRoot fs.FS = func() fs.FS {
	sub, err := fs.Sub(embeddedWeb, "web")
	if err != nil {
		panic(err)
	}
	return sub
}()

func serveWebHTML(w http.ResponseWriter, r *http.Request, name string) {
	b, err := fs.ReadFile(embeddedWebRoot, name)
	if err != nil {
		Error(w, http.StatusNotFound, "NOT_FOUND", "not found", "")
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(b)
}

func webAssetsHandler() http.Handler {
	assets, err := fs.Sub(embeddedWebRoot, "assets")
	if err != nil {
		panic(err)
	}
	return http.StripPrefix("/assets/", http.FileServer(http.FS(assets)))
}

