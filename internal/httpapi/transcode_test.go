package httpapi

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"go-learn/internal/store"
	"go-learn/internal/text"
)

func TestTranscodeFileNotFound(t *testing.T) {
	s, err := store.NewInMemoryStore(store.NewParams{MaxFiles: 10, MaxTotalBytes: 1024 * 1024})
	if err != nil {
		t.Fatal(err)
	}

	body, _ := json.Marshal(transcodeFileRequest{
		SourceEncoding: text.SourceEncodingAuto,
		TargetEncoding: text.EncodingUTF8,
	})
	req := httptest.NewRequest(http.MethodPost, "/api/files/nope/transcode", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	rr := httptest.NewRecorder()
	NewRouter(RouterDeps{
		ExternalOrigin: "http://127.0.0.1:8080",
		Store:          s,
		UploadSem:      NewSemaphore(1),
		TranscodeSem:   NewSemaphore(1),
		MaxFileBytes:   1024,
	}).ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d body=%s", rr.Code, rr.Body.String())
	}
}

func TestTranscodeNotTextRejected(t *testing.T) {
	s, err := store.NewInMemoryStore(store.NewParams{MaxFiles: 10, MaxTotalBytes: 1024 * 1024})
	if err != nil {
		t.Fatal(err)
	}
	meta, err := s.Add(store.AddParams{
		Name:     "a.bin",
		Bytes:    []byte{0x00, 0x01, 0x02},
		Encoding: text.EncodingUnknown,
		IsText:   false,
	})
	if err != nil {
		t.Fatal(err)
	}

	body, _ := json.Marshal(transcodeFileRequest{
		SourceEncoding: text.SourceEncodingAuto,
		TargetEncoding: text.EncodingUTF8,
	})
	req := httptest.NewRequest(http.MethodPost, "/api/files/"+meta.ID+"/transcode", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	rr := httptest.NewRecorder()
	NewRouter(RouterDeps{
		ExternalOrigin: "http://127.0.0.1:8080",
		Store:          s,
		UploadSem:      NewSemaphore(1),
		TranscodeSem:   NewSemaphore(1),
		MaxFileBytes:   1024,
	}).ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d body=%s", rr.Code, rr.Body.String())
	}
}

func TestTranscodeInvalidTargetRejected(t *testing.T) {
	s, err := store.NewInMemoryStore(store.NewParams{MaxFiles: 10, MaxTotalBytes: 1024 * 1024})
	if err != nil {
		t.Fatal(err)
	}
	meta, err := s.Add(store.AddParams{
		Name:     "a.txt",
		Bytes:    []byte("hello"),
		Encoding: text.EncodingUTF8,
		IsText:   true,
	})
	if err != nil {
		t.Fatal(err)
	}

	body, _ := json.Marshal(transcodeFileRequest{
		SourceEncoding: text.SourceEncodingAuto,
		TargetEncoding: "UTF-16",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/files/"+meta.ID+"/transcode", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	rr := httptest.NewRecorder()
	NewRouter(RouterDeps{
		ExternalOrigin: "http://127.0.0.1:8080",
		Store:          s,
		UploadSem:      NewSemaphore(1),
		TranscodeSem:   NewSemaphore(1),
		MaxFileBytes:   1024,
	}).ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d body=%s", rr.Code, rr.Body.String())
	}
}

func TestTranscodeBusyRejected(t *testing.T) {
	s, err := store.NewInMemoryStore(store.NewParams{MaxFiles: 10, MaxTotalBytes: 1024 * 1024})
	if err != nil {
		t.Fatal(err)
	}
	meta, err := s.Add(store.AddParams{
		Name:     "a.txt",
		Bytes:    []byte("hello"),
		Encoding: text.EncodingUTF8,
		IsText:   true,
	})
	if err != nil {
		t.Fatal(err)
	}

	sem := NewSemaphore(1)
	if !sem.TryAcquire() {
		t.Fatal("failed to acquire semaphore in test setup")
	}
	defer sem.Release()

	body, _ := json.Marshal(transcodeFileRequest{
		SourceEncoding: text.SourceEncodingAuto,
		TargetEncoding: text.EncodingUTF8,
	})
	req := httptest.NewRequest(http.MethodPost, "/api/files/"+meta.ID+"/transcode", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	rr := httptest.NewRecorder()
	NewRouter(RouterDeps{
		ExternalOrigin: "http://127.0.0.1:8080",
		Store:          s,
		UploadSem:      NewSemaphore(1),
		TranscodeSem:   sem,
		MaxFileBytes:   1024,
	}).ServeHTTP(rr, req)

	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d body=%s", rr.Code, rr.Body.String())
	}
	if rr.Header().Get("Retry-After") != "1" {
		t.Fatalf("expected Retry-After=1, got %q", rr.Header().Get("Retry-After"))
	}
}

func TestTranscodeFailureKeepsOriginalBytes(t *testing.T) {
	s, err := store.NewInMemoryStore(store.NewParams{MaxFiles: 10, MaxTotalBytes: 1024 * 1024})
	if err != nil {
		t.Fatal(err)
	}

	origin := []byte("hello🙂")
	meta, err := s.Add(store.AddParams{
		Name:     "a.txt",
		Bytes:    origin,
		Encoding: text.EncodingUTF8,
		IsText:   true,
	})
	if err != nil {
		t.Fatal(err)
	}

	body, _ := json.Marshal(transcodeFileRequest{
		SourceEncoding: text.EncodingUTF8,
		TargetEncoding: text.EncodingGBK,
	})
	req := httptest.NewRequest(http.MethodPost, "/api/files/"+meta.ID+"/transcode", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	rr := httptest.NewRecorder()
	NewRouter(RouterDeps{
		ExternalOrigin: "http://127.0.0.1:8080",
		Store:          s,
		UploadSem:      NewSemaphore(1),
		TranscodeSem:   NewSemaphore(1),
		MaxFileBytes:   1024,
	}).ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d body=%s", rr.Code, rr.Body.String())
	}

	got, err := s.Get(meta.ID)
	if err != nil {
		t.Fatalf("get after failed transcode: %v", err)
	}
	if !bytes.Equal(got.Bytes, origin) {
		t.Fatalf("expected original bytes kept, got=%q", string(got.Bytes))
	}
	if got.Meta.Encoding != text.EncodingUTF8 {
		t.Fatalf("expected original encoding kept, got=%s", got.Meta.Encoding)
	}
}

func TestTranscodeSuccessUpdatesBytesAndEncoding(t *testing.T) {
	s, err := store.NewInMemoryStore(store.NewParams{MaxFiles: 10, MaxTotalBytes: 1024 * 1024})
	if err != nil {
		t.Fatal(err)
	}

	origin := []byte("中文测试")
	meta, err := s.Add(store.AddParams{
		Name:     "a.txt",
		Bytes:    origin,
		Encoding: text.EncodingUTF8,
		IsText:   true,
	})
	if err != nil {
		t.Fatal(err)
	}

	body, _ := json.Marshal(transcodeFileRequest{
		SourceEncoding: text.EncodingUTF8,
		TargetEncoding: text.EncodingGBK,
	})
	req := httptest.NewRequest(http.MethodPost, "/api/files/"+meta.ID+"/transcode", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	rr := httptest.NewRecorder()
	NewRouter(RouterDeps{
		ExternalOrigin: "http://127.0.0.1:8080",
		Store:          s,
		UploadSem:      NewSemaphore(1),
		TranscodeSem:   NewSemaphore(1),
		MaxFileBytes:   1024 * 1024,
	}).ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rr.Code, rr.Body.String())
	}

	var out fileListItem
	if err := json.Unmarshal(rr.Body.Bytes(), &out); err != nil {
		t.Fatalf("unmarshal: %v body=%s", err, rr.Body.String())
	}
	if out.Encoding != text.EncodingGBK {
		t.Fatalf("expected response encoding=%s, got=%s", text.EncodingGBK, out.Encoding)
	}

	got, err := s.Get(meta.ID)
	if err != nil {
		t.Fatalf("get after transcode: %v", err)
	}
	if got.Meta.Encoding != text.EncodingGBK {
		t.Fatalf("expected store encoding=%s, got=%s", text.EncodingGBK, got.Meta.Encoding)
	}
	if bytes.Equal(got.Bytes, origin) {
		t.Fatalf("expected bytes updated after transcode")
	}
}

