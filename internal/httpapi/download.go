package httpapi

import (
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"go-learn/internal/store"
	"go-learn/internal/tokens"
)

const tokenKindDownload = "download"

type downloadTokenResponse struct {
	Token string `json:"token"`
	URL   string `json:"url"`
}

func createDownloadTokenHandler(d RouterDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if d.Store == nil {
			Error(w, http.StatusInternalServerError, "INTERNAL", "store not initialized", "")
			return
		}
		if d.Tokens == nil {
			Error(w, http.StatusInternalServerError, "INTERNAL", "token store not initialized", "")
			return
		}
		if d.DownloadTTL <= 0 {
			Error(w, http.StatusInternalServerError, "INTERNAL", "download ttl not initialized", "")
			return
		}

		id := chi.URLParam(r, "id")
		if id == "" {
			Error(w, http.StatusBadRequest, "BAD_REQUEST", "缺少文件 id", "")
			return
		}

		if _, err := d.Store.GetMeta(id); err != nil {
			if errors.Is(err, store.ErrNotFound) {
				Error(w, http.StatusNotFound, "NOT_FOUND", "not found", "")
				return
			}
			Error(w, http.StatusInternalServerError, "INTERNAL", "读取文件信息失败", err.Error())
			return
		}

		it, err := d.Tokens.Create(tokenKindDownload, id, d.DownloadTTL)
		if err != nil {
			Error(w, http.StatusInternalServerError, "INTERNAL", "创建下载链接失败", err.Error())
			return
		}

		JSON(w, http.StatusOK, downloadTokenResponse{
			Token: it.Token,
			URL:   "/dl/" + it.Token,
		})
	}
}

func downloadByTokenHandler(d RouterDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if d.Store == nil {
			Error(w, http.StatusInternalServerError, "INTERNAL", "store not initialized", "")
			return
		}
		if d.Tokens == nil {
			Error(w, http.StatusInternalServerError, "INTERNAL", "token store not initialized", "")
			return
		}

		token := chi.URLParam(r, "token")
		if token == "" {
			Error(w, http.StatusBadRequest, "BAD_REQUEST", "缺少 token", "")
			return
		}

		it, err := d.Tokens.Consume(token, tokenKindDownload)
		if err != nil {
			// token 过期/不存在/重复使用 -> 统一返回 410（一次性/时效性语义更明确）
			if errors.Is(err, tokens.ErrNotFound) || errors.Is(err, tokens.ErrKindMismatch) {
				Error(w, http.StatusGone, "TOKEN_INVALID", "下载链接已失效", "")
				return
			}
			if errors.Is(err, tokens.ErrInvalidInput) {
				Error(w, http.StatusBadRequest, "BAD_REQUEST", "token 不合法", err.Error())
				return
			}
			Error(w, http.StatusInternalServerError, "INTERNAL", "校验下载链接失败", err.Error())
			return
		}

		meta, reader, err := d.Store.Open(it.FileID)
		if err != nil {
			if errors.Is(err, store.ErrNotFound) {
				Error(w, http.StatusNotFound, "NOT_FOUND", "not found", "")
				return
			}
			Error(w, http.StatusInternalServerError, "INTERNAL", "读取文件失败", err.Error())
			return
		}

		modTime := meta.CreatedAt
		if modTime.IsZero() {
			modTime = time.Now()
		}

		w.Header().Set("Content-Type", "application/octet-stream")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Content-Disposition", contentDispositionAttachment(meta.Name))

		http.ServeContent(w, r, meta.Name, modTime, reader)
	}
}

func contentDispositionAttachment(filename string) string {
	orig := normalizeFilename(filename)
	if orig == "" {
		orig = "download"
	}
	fallback := asciiFallback(orig)
	if fallback == "" {
		fallback = "download"
	}

	// RFC 5987: filename*=UTF-8''<pct-encoded>
	utf8Name := url.PathEscape(orig)

	// filename must be quoted-string for safety.
	fallback = strings.ReplaceAll(fallback, `\`, `_`)
	fallback = strings.ReplaceAll(fallback, `"`, `_`)

	return `attachment; filename="` + fallback + `"; filename*=UTF-8''` + utf8Name
}

func normalizeFilename(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	// Prevent header injection & odd client behaviors.
	s = strings.Map(func(r rune) rune {
		switch r {
		case 0, '\r', '\n':
			return '_'
		case '/', '\\':
			return '_'
		default:
			return r
		}
	}, s)
	return s
}

func asciiFallback(s string) string {
	if s == "" {
		return ""
	}
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c >= 'a' && c <= 'z':
			b.WriteByte(c)
		case c >= 'A' && c <= 'Z':
			b.WriteByte(c)
		case c >= '0' && c <= '9':
			b.WriteByte(c)
		case c == '.' || c == '_' || c == '-' || c == ' ':
			b.WriteByte(c)
		default:
			b.WriteByte('_')
		}
	}
	return strings.TrimSpace(b.String())
}
