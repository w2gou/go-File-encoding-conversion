package httpapi

import (
	"fmt"
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
		r.Post("/bridge/upload", createBridgeUploadHandler(d))
		r.Post("/bridge/download", createBridgeDownloadHandler(d))
		r.Post("/bridge/{bridgeToken}/upload", bridgeUploadHandler(d))
		r.Post("/bridge/{bridgeToken}/download-token", bridgeDownloadTokenHandler(d))
	})

	r.Get("/dl/{token}", downloadByTokenHandler(d))
	r.Get("/qrcode/{bridgeToken}.png", qrcodeHandler(d))

	r.Get("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprintf(w, `<!doctype html>
<html lang="zh-CN">
<head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>go-File-encoding-conversion</title></head>
<body>
  <h1>go-File-encoding-conversion</h1>
  <p>服务已启动（第五步骨架）。</p>
  <ul>
    <li>手机上传页：<code>/m/upload/{bridgeToken}</code></li>
    <li>手机下载页：<code>/m/download/{bridgeToken}</code></li>
  </ul>
  <p>external_origin: <code>%s</code></p>
</body>
</html>`, d.ExternalOrigin)
	})

	r.Get("/m/upload/{bridgeToken}", mobileUploadPageHandler(d))
	r.Get("/m/download/{bridgeToken}", mobileDownloadPageHandler(d))

	r.NotFound(func(w http.ResponseWriter, r *http.Request) {
		Error(w, http.StatusNotFound, "NOT_FOUND", "not found", "")
	})

	return r
}
