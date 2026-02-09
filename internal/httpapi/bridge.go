package httpapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"io"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/skip2/go-qrcode"
	"go-learn/internal/store"
	"go-learn/internal/tokens"
)

const (
	tokenKindBridgeUpload   = "bridge-upload"
	tokenKindBridgeDownload = "bridge-download"
)

type bridgeCreateResponse struct {
	BridgeToken string `json:"bridgeToken"`
	PageURL     string `json:"pageUrl"`
	QRURL       string `json:"qrUrl"`
}

type createBridgeDownloadRequest struct {
	FileID string `json:"fileId"`
}

func createBridgeUploadHandler(d RouterDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if d.Tokens == nil {
			Error(w, http.StatusInternalServerError, "INTERNAL", "token store not initialized", "")
			return
		}
		if d.BridgeTTL <= 0 {
			Error(w, http.StatusInternalServerError, "INTERNAL", "bridge ttl not initialized", "")
			return
		}

		it, err := d.Tokens.Create(tokenKindBridgeUpload, "", d.BridgeTTL)
		if err != nil {
			Error(w, http.StatusInternalServerError, "INTERNAL", "创建桥接链接失败", err.Error())
			return
		}

		pageURL := "/m/upload/" + it.Token
		JSON(w, http.StatusOK, bridgeCreateResponse{
			BridgeToken: it.Token,
			PageURL:     pageURL,
			QRURL:       "/qrcode/" + it.Token + ".png",
		})
	}
}

func createBridgeDownloadHandler(d RouterDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if d.Store == nil {
			Error(w, http.StatusInternalServerError, "INTERNAL", "store not initialized", "")
			return
		}
		if d.Tokens == nil {
			Error(w, http.StatusInternalServerError, "INTERNAL", "token store not initialized", "")
			return
		}
		if d.BridgeTTL <= 0 {
			Error(w, http.StatusInternalServerError, "INTERNAL", "bridge ttl not initialized", "")
			return
		}

		var req createBridgeDownloadRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			if errors.Is(err, io.EOF) {
				Error(w, http.StatusBadRequest, "BAD_REQUEST", "缺少请求体", "")
				return
			}
			Error(w, http.StatusBadRequest, "BAD_REQUEST", "请求体不是合法 JSON", err.Error())
			return
		}
		if req.FileID == "" {
			Error(w, http.StatusBadRequest, "BAD_REQUEST", "缺少 fileId", "")
			return
		}

		if _, err := d.Store.GetMeta(req.FileID); err != nil {
			if errors.Is(err, store.ErrNotFound) {
				Error(w, http.StatusNotFound, "NOT_FOUND", "not found", "")
				return
			}
			Error(w, http.StatusInternalServerError, "INTERNAL", "读取文件信息失败", err.Error())
			return
		}

		it, err := d.Tokens.Create(tokenKindBridgeDownload, req.FileID, d.BridgeTTL)
		if err != nil {
			Error(w, http.StatusInternalServerError, "INTERNAL", "创建桥接链接失败", err.Error())
			return
		}

		pageURL := "/m/download/" + it.Token
		JSON(w, http.StatusOK, bridgeCreateResponse{
			BridgeToken: it.Token,
			PageURL:     pageURL,
			QRURL:       "/qrcode/" + it.Token + ".png",
		})
	}
}

func bridgeUploadHandler(d RouterDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if d.Tokens == nil {
			Error(w, http.StatusInternalServerError, "INTERNAL", "token store not initialized", "")
			return
		}

		bridgeToken := chi.URLParam(r, "bridgeToken")
		if bridgeToken == "" {
			Error(w, http.StatusBadRequest, "BAD_REQUEST", "缺少 bridgeToken", "")
			return
		}

		if _, err := d.Tokens.Consume(bridgeToken, tokenKindBridgeUpload); err != nil {
			if errors.Is(err, tokens.ErrNotFound) || errors.Is(err, tokens.ErrKindMismatch) {
				Error(w, http.StatusGone, "TOKEN_INVALID", "二维码已失效", "")
				return
			}
			Error(w, http.StatusInternalServerError, "INTERNAL", "校验二维码失败", err.Error())
			return
		}

		meta, ok := saveUploadedFile(w, r, d)
		if !ok {
			return
		}
		JSON(w, http.StatusCreated, metaToFileListItem(meta))
	}
}

func bridgeDownloadTokenHandler(d RouterDeps) http.HandlerFunc {
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

		bridgeToken := chi.URLParam(r, "bridgeToken")
		if bridgeToken == "" {
			Error(w, http.StatusBadRequest, "BAD_REQUEST", "缺少 bridgeToken", "")
			return
		}

		it, err := d.Tokens.Consume(bridgeToken, tokenKindBridgeDownload)
		if err != nil {
			if errors.Is(err, tokens.ErrNotFound) || errors.Is(err, tokens.ErrKindMismatch) {
				Error(w, http.StatusGone, "TOKEN_INVALID", "二维码已失效", "")
				return
			}
			Error(w, http.StatusInternalServerError, "INTERNAL", "校验二维码失败", err.Error())
			return
		}

		if _, err := d.Store.GetMeta(it.FileID); err != nil {
			if errors.Is(err, store.ErrNotFound) {
				Error(w, http.StatusNotFound, "NOT_FOUND", "not found", "")
				return
			}
			Error(w, http.StatusInternalServerError, "INTERNAL", "读取文件信息失败", err.Error())
			return
		}

		downloadItem, err := d.Tokens.Create(tokenKindDownload, it.FileID, d.DownloadTTL)
		if err != nil {
			Error(w, http.StatusInternalServerError, "INTERNAL", "创建下载链接失败", err.Error())
			return
		}

		JSON(w, http.StatusOK, downloadTokenResponse{
			Token: downloadItem.Token,
			URL:   "/dl/" + downloadItem.Token,
		})
	}
}

func mobileUploadPageHandler(d RouterDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		bridgeToken := chi.URLParam(r, "bridgeToken")
		if bridgeToken == "" {
			Error(w, http.StatusBadRequest, "BAD_REQUEST", "缺少 bridgeToken", "")
			return
		}
		if err := ensureBridgeToken(d, bridgeToken, tokenKindBridgeUpload); err != nil {
			writeBridgePageError(w, err)
			return
		}

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		escaped := template.HTMLEscapeString(bridgeToken)
		_, _ = fmt.Fprintf(w, `<!doctype html>
<html lang="zh-CN">
<head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>手机上传</title></head>
<body>
  <h1>手机上传</h1>
  <form id="f" action="/api/bridge/%s/upload" method="post" enctype="multipart/form-data">
    <input type="file" name="file" required />
    <button type="submit">上传</button>
  </form>
  <p id="msg"></p>
  <script>
    const f = document.getElementById('f');
    const msg = document.getElementById('msg');
    f.addEventListener('submit', async (e) => {
      e.preventDefault();
      msg.textContent = '上传中...';
      const res = await fetch(f.action, { method: 'POST', body: new FormData(f) });
      const text = await res.text();
      if (res.ok) {
        msg.textContent = '上传成功';
      } else {
        msg.textContent = '上传失败: ' + text;
      }
    });
  </script>
</body>
</html>`, escaped)
	}
}

func mobileDownloadPageHandler(d RouterDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		bridgeToken := chi.URLParam(r, "bridgeToken")
		if bridgeToken == "" {
			Error(w, http.StatusBadRequest, "BAD_REQUEST", "缺少 bridgeToken", "")
			return
		}
		if d.Tokens == nil {
			Error(w, http.StatusInternalServerError, "INTERNAL", "token store not initialized", "")
			return
		}
		if d.Store == nil {
			Error(w, http.StatusInternalServerError, "INTERNAL", "store not initialized", "")
			return
		}

		it, err := d.Tokens.Peek(bridgeToken)
		if err != nil || it.Kind != tokenKindBridgeDownload {
			writeBridgePageError(w, errBridgeTokenInvalid)
			return
		}
		meta, err := d.Store.GetMeta(it.FileID)
		if err != nil {
			if errors.Is(err, store.ErrNotFound) {
				writeBridgePageError(w, errBridgeTokenInvalid)
				return
			}
			Error(w, http.StatusInternalServerError, "INTERNAL", "读取文件信息失败", err.Error())
			return
		}

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		escapedToken := template.HTMLEscapeString(bridgeToken)
		escapedName := template.HTMLEscapeString(meta.Name)
		escapedEnc := template.HTMLEscapeString(normalizeEncoding(meta.Encoding))
		_, _ = fmt.Fprintf(w, `<!doctype html>
<html lang="zh-CN">
<head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>手机下载</title></head>
<body>
  <h1>手机下载</h1>
  <p>文件名: %s</p>
  <p>大小: %d 字节</p>
  <p>编码: %s</p>
  <button id="dl">下载</button>
  <p id="msg"></p>
  <script>
    const btn = document.getElementById('dl');
    const msg = document.getElementById('msg');
    btn.addEventListener('click', async () => {
      btn.disabled = true;
      msg.textContent = '准备下载链接...';
      const res = await fetch('/api/bridge/%s/download-token', { method: 'POST' });
      const text = await res.text();
      if (!res.ok) {
        msg.textContent = '下载失败: ' + text;
        btn.disabled = false;
        return;
      }
      const data = JSON.parse(text);
      if (!data || !data.url) {
        msg.textContent = '下载失败: 响应不合法';
        btn.disabled = false;
        return;
      }
      window.location.href = data.url;
      msg.textContent = '已开始下载';
    });
  </script>
</body>
</html>`, escapedName, meta.SizeBytes, escapedEnc, escapedToken)
	}
}

func qrcodeHandler(d RouterDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if d.Tokens == nil {
			Error(w, http.StatusInternalServerError, "INTERNAL", "token store not initialized", "")
			return
		}
		if d.ExternalOrigin == "" {
			Error(w, http.StatusInternalServerError, "INTERNAL", "external origin not initialized", "")
			return
		}

		bridgeToken := chi.URLParam(r, "bridgeToken")
		if bridgeToken == "" {
			Error(w, http.StatusBadRequest, "BAD_REQUEST", "缺少 bridgeToken", "")
			return
		}

		it, err := d.Tokens.Peek(bridgeToken)
		if err != nil {
			if errors.Is(err, tokens.ErrNotFound) {
				Error(w, http.StatusGone, "TOKEN_INVALID", "二维码已失效", "")
				return
			}
			Error(w, http.StatusInternalServerError, "INTERNAL", "校验二维码失败", err.Error())
			return
		}

		pageURL := ""
		switch it.Kind {
		case tokenKindBridgeUpload:
			pageURL = "/m/upload/" + bridgeToken
		case tokenKindBridgeDownload:
			pageURL = "/m/download/" + bridgeToken
		default:
			Error(w, http.StatusGone, "TOKEN_INVALID", "二维码已失效", "")
			return
		}

		absoluteURL := strings.TrimRight(d.ExternalOrigin, "/") + pageURL
		png, err := qrcode.Encode(absoluteURL, qrcode.Medium, 256)
		if err != nil {
			Error(w, http.StatusInternalServerError, "INTERNAL", "生成二维码失败", err.Error())
			return
		}

		w.Header().Set("Content-Type", "image/png")
		w.Header().Set("Cache-Control", "no-store")
		_, _ = w.Write(png)
	}
}

var errBridgeTokenInvalid = errors.New("bridge token invalid")

func ensureBridgeToken(d RouterDeps, bridgeToken, kind string) error {
	if d.Tokens == nil {
		return errors.New("token store not initialized")
	}
	it, err := d.Tokens.Peek(bridgeToken)
	if err != nil {
		if errors.Is(err, tokens.ErrNotFound) {
			return errBridgeTokenInvalid
		}
		return err
	}
	if it.Kind != kind {
		return errBridgeTokenInvalid
	}
	return nil
}

func writeBridgePageError(w http.ResponseWriter, err error) {
	if errors.Is(err, errBridgeTokenInvalid) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusGone)
		_, _ = io.WriteString(w, "<!doctype html><html lang=\"zh-CN\"><body><h1>二维码已失效</h1></body></html>")
		return
	}
	Error(w, http.StatusInternalServerError, "INTERNAL", "页面加载失败", err.Error())
}

