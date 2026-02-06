package httpapi

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/go-chi/chi/v5"
	"go-learn/internal/store"
)

type renameFileRequest struct {
	Name string `json:"name"`
}

func renameFileHandler(d RouterDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if d.Store == nil {
			Error(w, http.StatusInternalServerError, "INTERNAL", "store not initialized", "")
			return
		}

		id := chi.URLParam(r, "id")
		if id == "" {
			Error(w, http.StatusBadRequest, "BAD_REQUEST", "缺少文件 id", "")
			return
		}

		var req renameFileRequest
		dec := json.NewDecoder(r.Body)
		if err := dec.Decode(&req); err != nil {
			if errors.Is(err, io.EOF) {
				Error(w, http.StatusBadRequest, "BAD_REQUEST", "缺少请求体", "")
				return
			}
			Error(w, http.StatusBadRequest, "BAD_REQUEST", "请求体不是合法 JSON", err.Error())
			return
		}
		// Avoid silently ignoring trailing garbage.
		if err := dec.Decode(&struct{}{}); err != io.EOF {
			if err == nil {
				err = errors.New("unexpected trailing tokens")
			}
			Error(w, http.StatusBadRequest, "BAD_REQUEST", "请求体不是合法 JSON", err.Error())
			return
		}
		if req.Name == "" {
			Error(w, http.StatusBadRequest, "BAD_REQUEST", "缺少 name", "")
			return
		}

		meta, err := d.Store.Rename(id, req.Name)
		if err != nil {
			switch {
			case errors.Is(err, store.ErrNotFound):
				Error(w, http.StatusNotFound, "NOT_FOUND", "not found", "")
			case errors.Is(err, store.ErrNameConflict):
				// 需求口径：冲突直接拒绝，并保持原名不变（store 层已保证不修改）。
				Error(w, http.StatusConflict, "NAME_CONFLICT", "重名", "")
			case errors.Is(err, store.ErrInvalidInput):
				Error(w, http.StatusBadRequest, "BAD_REQUEST", "请求不合法", err.Error())
			default:
				Error(w, http.StatusInternalServerError, "INTERNAL", "重命名失败", err.Error())
			}
			return
		}

		JSON(w, http.StatusOK, fileListItem{
			ID:        meta.ID,
			Name:      meta.Name,
			CreatedAt: meta.CreatedAt,
			SizeBytes: meta.SizeBytes,
			Encoding:  normalizeEncoding(meta.Encoding),
			IsText:    meta.IsText,
		})
	}
}
