package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"go-learn/internal/store"
)

func TestListFiles(t *testing.T) {
	s, err := store.NewInMemoryStore(store.NewParams{MaxFiles: 10, MaxTotalBytes: 1024 * 1024})
	if err != nil {
		t.Fatal(err)
	}

	now := time.Unix(10, 0).UTC()
	meta, err := s.Add(store.AddParams{
		Name:     "a.txt",
		Bytes:    []byte("hello"),
		Encoding: "UTF-8",
		IsText:   true,
		Now:      now,
	})
	if err != nil {
		t.Fatalf("add: %v", err)
	}

	h := NewRouter(RouterDeps{
		ExternalOrigin: "http://127.0.0.1:8080",
		Store:          s,
		UploadSem:      NewSemaphore(1),
		MaxFileBytes:   1024 * 1024,
	})
	req := httptest.NewRequest(http.MethodGet, "/api/files", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}

	var items []fileListItem
	if err := json.Unmarshal(rr.Body.Bytes(), &items); err != nil {
		t.Fatalf("unmarshal: %v body=%s", err, rr.Body.String())
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(items))
	}
	if items[0].ID != meta.ID || items[0].Name != "a.txt" || items[0].SizeBytes != 5 || items[0].Encoding != "UTF-8" || !items[0].IsText {
		t.Fatalf("unexpected item: %#v", items[0])
	}
}

