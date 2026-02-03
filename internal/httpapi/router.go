package httpapi

import (
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"
	"go-learn/internal/store"
)

type RouterDeps struct {
	ExternalOrigin string
	Store          *store.InMemoryStore
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
		r.Delete("/files/{id}", deleteFileHandler(d))
	})

	r.Get("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprintf(w, `<!doctype html>
<html lang="zh-CN">
<head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>go-File-encoding-conversion</title></head>
<body>
  <h1>go-File-encoding-conversion</h1>
  <p>服务已启动（第五步骨架）。</p>
  <ul>
    <li>手机上传页（占位）：<code>/m/upload/{token}</code></li>
    <li>手机下载页（占位）：<code>/m/download/{token}</code></li>
  </ul>
  <p>external_origin: <code>%s</code></p>
</body>
</html>`, d.ExternalOrigin)
	})

	r.Get("/m/upload/{token}", func(w http.ResponseWriter, r *http.Request) {
		token := chi.URLParam(r, "token")
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprintf(w, `<!doctype html>
<html lang="zh-CN">
<head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>手机上传</title></head>
<body>
  <h1>手机上传（占位页）</h1>
  <p>token: <code>%s</code></p>
  <p>后续步骤会在此页接入上传表单并消费一次性 bridge token。</p>
</body>
</html>`, token)
	})

	r.Get("/m/download/{token}", func(w http.ResponseWriter, r *http.Request) {
		token := chi.URLParam(r, "token")
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprintf(w, `<!doctype html>
<html lang="zh-CN">
<head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>手机下载</title></head>
<body>
  <h1>手机下载（占位页）</h1>
  <p>token: <code>%s</code></p>
  <p>后续步骤会在此页展示文件信息并触发一次性下载链接。</p>
</body>
</html>`, token)
	})

	r.NotFound(func(w http.ResponseWriter, r *http.Request) {
		Error(w, http.StatusNotFound, "NOT_FOUND", "not found", "")
	})

	return r
}
