package httpapi

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"go-learn/internal/store"
)

func TestRenameFileNotFound(t *testing.T) {
	s, err := store.NewInMemoryStore(store.NewParams{MaxFiles: 10, MaxTotalBytes: 1024 * 1024})
	if err != nil {
		t.Fatal(err)
	}

	body, _ := json.Marshal(renameFileRequest{Name: "b.txt"})
	req := httptest.NewRequest(http.MethodPatch, "/api/files/nope", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	rr := httptest.NewRecorder()
	NewRouter(RouterDeps{
		ExternalOrigin: "http://127.0.0.1:8080",
		Store:          s,
		UploadSem:      NewSemaphore(1),
		MaxFileBytes:   1024,
	}).ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d body=%s", rr.Code, rr.Body.String())
	}
}

func TestRenameFileNameConflictKeepsOriginal(t *testing.T) {
	s, err := store.NewInMemoryStore(store.NewParams{MaxFiles: 10, MaxTotalBytes: 1024 * 1024})
	if err != nil {
		t.Fatal(err)
	}
	a, err := s.Add(store.AddParams{Name: "a.txt", Bytes: []byte("a")})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Add(store.AddParams{Name: "b.txt", Bytes: []byte("b")}); err != nil {
		t.Fatal(err)
	}

	body, _ := json.Marshal(renameFileRequest{Name: "b.txt"})
	req := httptest.NewRequest(http.MethodPatch, "/api/files/"+a.ID, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	rr := httptest.NewRecorder()
	NewRouter(RouterDeps{
		ExternalOrigin: "http://127.0.0.1:8080",
		Store:          s,
		UploadSem:      NewSemaphore(1),
		MaxFileBytes:   1024,
	}).ServeHTTP(rr, req)

	if rr.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d body=%s", rr.Code, rr.Body.String())
	}

	meta, err := s.GetMeta(a.ID)
	if err != nil {
		t.Fatalf("get meta: %v", err)
	}
	if meta.Name != "a.txt" {
		t.Fatalf("expected original name kept, got %q", meta.Name)
	}
}

func TestRenameFileSuccess(t *testing.T) {
	s, err := store.NewInMemoryStore(store.NewParams{MaxFiles: 10, MaxTotalBytes: 1024 * 1024})
	if err != nil {
		t.Fatal(err)
	}
	a, err := s.Add(store.AddParams{Name: "a.txt", Bytes: []byte("hello"), Encoding: "UTF-8", IsText: true})
	if err != nil {
		t.Fatal(err)
	}

	body, _ := json.Marshal(renameFileRequest{Name: "c.txt"})
	req := httptest.NewRequest(http.MethodPatch, "/api/files/"+a.ID, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	rr := httptest.NewRecorder()
	NewRouter(RouterDeps{
		ExternalOrigin: "http://127.0.0.1:8080",
		Store:          s,
		UploadSem:      NewSemaphore(1),
		MaxFileBytes:   1024,
	}).ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rr.Code, rr.Body.String())
	}

	var out fileListItem
	if err := json.Unmarshal(rr.Body.Bytes(), &out); err != nil {
		t.Fatalf("unmarshal: %v body=%s", err, rr.Body.String())
	}
	if out.ID != a.ID || out.Name != "c.txt" || out.SizeBytes != 5 || out.Encoding != "UTF-8" || !out.IsText {
		t.Fatalf("unexpected response: %#v", out)
	}

	meta, err := s.GetMeta(a.ID)
	if err != nil {
		t.Fatalf("get meta: %v", err)
	}
	if meta.Name != "c.txt" {
		t.Fatalf("expected renamed, got %q", meta.Name)
	}
}

