package httpapi

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"go-learn/internal/store"
	"go-learn/internal/tokens"
)

type RouterDeps struct {
	ExternalOrigin string
	Store          *store.InMemoryStore
	Tokens         *tokens.Store
	DownloadTTL    time.Duration
	BridgeTTL      time.Duration
	UploadSem      *Semaphore
	TranscodeSem   *Semaphore
	MaxFileBytes   int64
	MaxRequestBytes int64
}

func NewRouter(d RouterDeps) http.Handler {
	r := chi.NewRouter()
	r.Use(Recover)
	r.Use(RequestLog)

	r.Route("/api", func(r chi.Router) {
		r.Get("/files", listFilesHandler(d))
		r.Post("/files", uploadFileHandler(d))
		r.Patch("/files/{id}", renameFileHandler(d))
		r.Delete("/files/{id}", deleteFileHandler(d))
		r.Post("/files/{id}/download-token", createDownloadTokenHandler(d))
		r.Post("/files/{id}/transcode", transcodeFileHandler(d))
		r.Post("/bridge/upload", createBridgeUploadHandler(d))
		r.Post("/bridge/download", createBridgeDownloadHandler(d))
		r.Post("/bridge/{bridgeToken}/upload", bridgeUploadHandler(d))
		r.Get("/bridge/{bridgeToken}/download-info", bridgeDownloadInfoHandler(d))
		r.Post("/bridge/{bridgeToken}/download-token", bridgeDownloadTokenHandler(d))
	})

	r.Handle("/assets/*", webAssetsHandler())
	r.Get("/dl/{token}", downloadByTokenHandler(d))
	r.Get("/qrcode/{bridgeToken}.png", qrcodeHandler(d))
	r.Get("/", func(w http.ResponseWriter, r *http.Request) { serveWebHTML(w, r, "index.html") })

	r.Get("/m/upload/{bridgeToken}", mobileUploadPageHandler(d))
	r.Get("/m/download/{bridgeToken}", mobileDownloadPageHandler(d))

	r.NotFound(func(w http.ResponseWriter, r *http.Request) {
		Error(w, http.StatusNotFound, "NOT_FOUND", "not found", "")
	})

	return r
}
