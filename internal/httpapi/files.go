package httpapi

import (
	"net/http"
	"time"
)

type fileListItem struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
	SizeBytes int64     `json:"size_bytes"`
	Encoding  string    `json:"encoding"`
	IsText    bool      `json:"is_text"`
}

func listFilesHandler(d RouterDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if d.Store == nil {
			Error(w, http.StatusInternalServerError, "INTERNAL", "store not initialized", "")
			return
		}

		metas := d.Store.List()
		out := make([]fileListItem, 0, len(metas))
		for _, m := range metas {
			out = append(out, fileListItem{
				ID:        m.ID,
				Name:      m.Name,
				CreatedAt: m.CreatedAt,
				SizeBytes: m.SizeBytes,
				Encoding:  normalizeEncoding(m.Encoding),
				IsText:    m.IsText,
			})
		}
		JSON(w, http.StatusOK, out)
	}
}

func normalizeEncoding(enc string) string {
	if enc == "" {
		return "Unknown"
	}
	return enc
}
