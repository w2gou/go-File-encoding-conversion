package httpapi

import (
	"encoding/json"
	"errors"
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

func bridgeDownloadInfoHandler(d RouterDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if d.Store == nil {
			Error(w, http.StatusInternalServerError, "INTERNAL", "store not initialized", "")
			return
		}
		if d.Tokens == nil {
			Error(w, http.StatusInternalServerError, "INTERNAL", "token store not initialized", "")
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
		if it.Kind != tokenKindBridgeDownload {
			Error(w, http.StatusGone, "TOKEN_INVALID", "二维码已失效", "")
			return
		}

		meta, err := d.Store.GetMeta(it.FileID)
		if err != nil {
			if errors.Is(err, store.ErrNotFound) {
				Error(w, http.StatusNotFound, "NOT_FOUND", "not found", "")
				return
			}
			Error(w, http.StatusInternalServerError, "INTERNAL", "读取文件信息失败", err.Error())
			return
		}

		JSON(w, http.StatusOK, metaToFileListItem(meta))
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
		serveWebHTML(w, r, "mobile-upload.html")
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
		_, err = d.Store.GetMeta(it.FileID)
		if err != nil {
			if errors.Is(err, store.ErrNotFound) {
				writeBridgePageError(w, errBridgeTokenInvalid)
				return
			}
			Error(w, http.StatusInternalServerError, "INTERNAL", "读取文件信息失败", err.Error())
			return
		}
		serveWebHTML(w, r, "mobile-download.html")
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
