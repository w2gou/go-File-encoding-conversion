package httpapi

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"go-learn/internal/store"
	"go-learn/internal/text"
)

type transcodeFileRequest struct {
	SourceEncoding string `json:"sourceEncoding"`
	TargetEncoding string `json:"targetEncoding"`
}

func transcodeFileHandler(d RouterDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if d.Store == nil {
			Error(w, http.StatusInternalServerError, "INTERNAL", "store not initialized", "")
			return
		}
		if d.TranscodeSem == nil {
			Error(w, http.StatusInternalServerError, "INTERNAL", "transcode limiter not initialized", "")
			return
		}

		id := chi.URLParam(r, "id")
		if id == "" {
			Error(w, http.StatusBadRequest, "BAD_REQUEST", "缺少文件 id", "")
			return
		}

		req, ok := decodeTranscodeRequest(w, r)
		if !ok {
			return
		}

		sourceEncoding := strings.TrimSpace(req.SourceEncoding)
		if sourceEncoding == "" {
			sourceEncoding = text.SourceEncodingAuto
		}
		targetEncoding := strings.TrimSpace(req.TargetEncoding)
		if targetEncoding == "" {
			Error(w, http.StatusBadRequest, "BAD_REQUEST", "缺少 targetEncoding", "")
			return
		}
		if !isAllowedTargetEncoding(targetEncoding) {
			Error(w, http.StatusBadRequest, "BAD_REQUEST", "targetEncoding 不在允许列表", "")
			return
		}
		if !isAllowedSourceEncoding(sourceEncoding) {
			Error(w, http.StatusBadRequest, "BAD_REQUEST", "sourceEncoding 不在允许列表", "")
			return
		}

		if !d.TranscodeSem.TryAcquire() {
			w.Header().Set("Retry-After", "1")
			Error(w, http.StatusServiceUnavailable, "BUSY", "转码并发已满，请稍后重试", "")
			return
		}
		defer d.TranscodeSem.Release()

		file, err := d.Store.Get(id)
		if err != nil {
			if errors.Is(err, store.ErrNotFound) {
				Error(w, http.StatusNotFound, "NOT_FOUND", "not found", "")
				return
			}
			Error(w, http.StatusInternalServerError, "INTERNAL", "读取文件失败", err.Error())
			return
		}
		if !file.Meta.IsText {
			Error(w, http.StatusBadRequest, "BAD_REQUEST", "不支持转码（非可识别文本）", "")
			return
		}

		out, resolvedTarget, err := text.StrictTranscode(file.Bytes, text.TranscodeParams{
			SourceEncoding: sourceEncoding,
			TargetEncoding: targetEncoding,
		})
		if err != nil {
			writeTranscodeError(w, err)
			return
		}

		updated, err := d.Store.ReplaceBytes(store.ReplaceParams{
			ID:       id,
			Bytes:    out,
			Encoding: resolvedTarget,
			IsText:   true,
		})
		if err != nil {
			switch {
			case errors.Is(err, store.ErrNotFound):
				// 并发删除场景：转码流程中目标文件已不存在。
				Error(w, http.StatusNotFound, "NOT_FOUND", "not found", "")
			case errors.Is(err, store.ErrReplaceWouldExceed), errors.Is(err, store.ErrTooLarge):
				Error(w, http.StatusInsufficientStorage, "INSUFFICIENT_STORAGE", "空间不足，无法保存转码结果", "")
			case errors.Is(err, store.ErrInvalidInput):
				Error(w, http.StatusBadRequest, "BAD_REQUEST", "请求不合法", err.Error())
			default:
				Error(w, http.StatusInternalServerError, "INTERNAL", "写入转码结果失败", err.Error())
			}
			return
		}

		JSON(w, http.StatusOK, metaToFileListItem(updated))
	}
}

func decodeTranscodeRequest(w http.ResponseWriter, r *http.Request) (transcodeFileRequest, bool) {
	var req transcodeFileRequest
	dec := json.NewDecoder(r.Body)
	if err := dec.Decode(&req); err != nil {
		if errors.Is(err, io.EOF) {
			Error(w, http.StatusBadRequest, "BAD_REQUEST", "缺少请求体", "")
			return transcodeFileRequest{}, false
		}
		Error(w, http.StatusBadRequest, "BAD_REQUEST", "请求体不是合法 JSON", err.Error())
		return transcodeFileRequest{}, false
	}
	if err := dec.Decode(&struct{}{}); err != io.EOF {
		if err == nil {
			err = errors.New("unexpected trailing tokens")
		}
		Error(w, http.StatusBadRequest, "BAD_REQUEST", "请求体不是合法 JSON", err.Error())
		return transcodeFileRequest{}, false
	}
	return req, true
}

func writeTranscodeError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, text.ErrNotText):
		Error(w, http.StatusBadRequest, "BAD_REQUEST", "不支持转码（非可识别文本）", "")
	case errors.Is(err, text.ErrUnsupportedEncoding), errors.Is(err, text.ErrUnknownSource), errors.Is(err, text.ErrInvalidInput):
		Error(w, http.StatusBadRequest, "BAD_REQUEST", "编码参数不合法", err.Error())
	case errors.Is(err, text.ErrDecodeFailed):
		Error(w, http.StatusBadRequest, "TRANSCODE_FAILED", "源编码解码失败", "")
	case errors.Is(err, text.ErrEncodeFailed), errors.Is(err, text.ErrUnrepresentable):
		Error(w, http.StatusBadRequest, "TRANSCODE_FAILED", "目标编码无法表示该内容", "")
	default:
		Error(w, http.StatusInternalServerError, "INTERNAL", "转码失败", err.Error())
	}
}

func isAllowedTargetEncoding(v string) bool {
	for _, enc := range text.TargetEncodings() {
		if v == enc {
			return true
		}
	}
	return false
}

func isAllowedSourceEncoding(v string) bool {
	if v == text.SourceEncodingAuto {
		return true
	}
	return isAllowedTargetEncoding(v)
}

