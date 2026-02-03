package httpapi

import (
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"go-learn/internal/store"
)

func deleteFileHandler(d RouterDeps) http.HandlerFunc {
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

		_, err := d.Store.Delete(id)
		if err != nil {
			if errors.Is(err, store.ErrNotFound) {
				Error(w, http.StatusNotFound, "NOT_FOUND", "not found", "")
				return
			}
			Error(w, http.StatusInternalServerError, "INTERNAL", "删除失败", err.Error())
			return
		}

		w.WriteHeader(http.StatusNoContent)
	}
}

