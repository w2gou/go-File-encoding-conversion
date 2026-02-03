package httpapi

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"go-learn/internal/store"
)

func TestDeleteFileNotFound(t *testing.T) {
	s, err := store.NewInMemoryStore(store.NewParams{MaxFiles: 10, MaxTotalBytes: 1024 * 1024})
	if err != nil {
		t.Fatal(err)
	}

	h := NewRouter(RouterDeps{
		ExternalOrigin: "http://127.0.0.1:8080",
		Store:          s,
		UploadSem:      NewSemaphore(1),
		MaxFileBytes:   1024,
	})
	req := httptest.NewRequest(http.MethodDelete, "/api/files/nope", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d body=%s", rr.Code, rr.Body.String())
	}
}

func TestDeleteFileSuccess(t *testing.T) {
	s, err := store.NewInMemoryStore(store.NewParams{MaxFiles: 10, MaxTotalBytes: 1024 * 1024})
	if err != nil {
		t.Fatal(err)
	}

	meta, err := s.Add(store.AddParams{Name: "a.txt", Bytes: []byte("hello")})
	if err != nil {
		t.Fatal(err)
	}

	h := NewRouter(RouterDeps{
		ExternalOrigin: "http://127.0.0.1:8080",
		Store:          s,
		UploadSem:      NewSemaphore(1),
		MaxFileBytes:   1024,
	})
	req := httptest.NewRequest(http.MethodDelete, "/api/files/"+meta.ID, nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d body=%s", rr.Code, rr.Body.String())
	}
	if _, err := s.GetMeta(meta.ID); err != store.ErrNotFound {
		t.Fatalf("expected deleted, got %v", err)
	}
}

