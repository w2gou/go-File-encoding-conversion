package httpapi

import (
	"bytes"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"go-learn/internal/store"
)

func TestUploadMissingContentLengthRejected(t *testing.T) {
	s, err := store.NewInMemoryStore(store.NewParams{MaxFiles: 10, MaxTotalBytes: 1024 * 1024})
	if err != nil {
		t.Fatal(err)
	}

	body, contentType := newMultipartBody(t, "a.txt", []byte("hello"))
	req := httptest.NewRequest(http.MethodPost, "/api/files", bytes.NewReader(body))
	req.Header.Set("Content-Type", contentType)
	req.ContentLength = -1

	rr := httptest.NewRecorder()
	NewRouter(RouterDeps{
		ExternalOrigin: "http://127.0.0.1:8080",
		Store:          s,
		UploadSem:      NewSemaphore(1),
		MaxFileBytes:   1024,
	}).ServeHTTP(rr, req)

	if rr.Code != http.StatusLengthRequired {
		t.Fatalf("expected 411, got %d body=%s", rr.Code, rr.Body.String())
	}
}

func TestUploadNameConflictEarly(t *testing.T) {
	s, err := store.NewInMemoryStore(store.NewParams{MaxFiles: 10, MaxTotalBytes: 1024 * 1024})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Add(store.AddParams{Name: "a.txt", Bytes: []byte("x")}); err != nil {
		t.Fatal(err)
	}

	body, contentType := newMultipartBody(t, "a.txt", []byte("hello"))
	req := httptest.NewRequest(http.MethodPost, "/api/files", bytes.NewReader(body))
	req.Header.Set("Content-Type", contentType)
	req.ContentLength = int64(len(body))

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
}

func TestUploadTooLargeFileRejected(t *testing.T) {
	s, err := store.NewInMemoryStore(store.NewParams{MaxFiles: 10, MaxTotalBytes: 1024 * 1024})
	if err != nil {
		t.Fatal(err)
	}

	body, contentType := newMultipartBody(t, "a.txt", []byte("012345"))
	req := httptest.NewRequest(http.MethodPost, "/api/files", bytes.NewReader(body))
	req.Header.Set("Content-Type", contentType)
	req.ContentLength = int64(len(body))

	rr := httptest.NewRecorder()
	NewRouter(RouterDeps{
		ExternalOrigin: "http://127.0.0.1:8080",
		Store:          s,
		UploadSem:      NewSemaphore(1),
		MaxFileBytes:   3,
	}).ServeHTTP(rr, req)

	if rr.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("expected 413, got %d body=%s", rr.Code, rr.Body.String())
	}
}

func TestUploadBusyRejected(t *testing.T) {
	s, err := store.NewInMemoryStore(store.NewParams{MaxFiles: 10, MaxTotalBytes: 1024 * 1024})
	if err != nil {
		t.Fatal(err)
	}

	sem := NewSemaphore(1)
	if !sem.TryAcquire() {
		t.Fatal("failed to acquire semaphore in test setup")
	}
	defer sem.Release()

	body, contentType := newMultipartBody(t, "a.txt", []byte("hello"))
	req := httptest.NewRequest(http.MethodPost, "/api/files", bytes.NewReader(body))
	req.Header.Set("Content-Type", contentType)
	req.ContentLength = int64(len(body))

	rr := httptest.NewRecorder()
	NewRouter(RouterDeps{
		ExternalOrigin: "http://127.0.0.1:8080",
		Store:          s,
		UploadSem:      sem,
		MaxFileBytes:   1024,
	}).ServeHTTP(rr, req)

	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d body=%s", rr.Code, rr.Body.String())
	}
}

func TestUploadSuccess(t *testing.T) {
	s, err := store.NewInMemoryStore(store.NewParams{MaxFiles: 10, MaxTotalBytes: 1024 * 1024})
	if err != nil {
		t.Fatal(err)
	}

	body, contentType := newMultipartBody(t, "a.txt", []byte("hello"))
	req := httptest.NewRequest(http.MethodPost, "/api/files", bytes.NewReader(body))
	req.Header.Set("Content-Type", contentType)
	req.ContentLength = int64(len(body))

	rr := httptest.NewRecorder()
	NewRouter(RouterDeps{
		ExternalOrigin: "http://127.0.0.1:8080",
		Store:          s,
		UploadSem:      NewSemaphore(1),
		MaxFileBytes:   1024,
	}).ServeHTTP(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d body=%s", rr.Code, rr.Body.String())
	}

	items := s.List()
	if len(items) != 1 || items[0].Name != "a.txt" || items[0].SizeBytes != 5 {
		t.Fatalf("unexpected store state: %#v", items)
	}
}

func newMultipartBody(t *testing.T, filename string, content []byte) ([]byte, string) {
	t.Helper()
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	fw, err := w.CreateFormFile("file", filename)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fw.Write(content); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes(), w.FormDataContentType()
}

