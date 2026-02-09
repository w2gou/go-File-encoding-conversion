package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"go-learn/internal/store"
	"go-learn/internal/tokens"
)

func TestCreateDownloadTokenFileNotFound(t *testing.T) {
	s, err := store.NewInMemoryStore(store.NewParams{MaxFiles: 10, MaxTotalBytes: 1024 * 1024})
	if err != nil {
		t.Fatal(err)
	}

	h := NewRouter(RouterDeps{
		ExternalOrigin: "http://127.0.0.1:8080",
		Store:          s,
		Tokens: func() *tokens.Store {
			ts := tokens.NewStore(tokens.Options{})
			t.Cleanup(ts.Close)
			return ts
		}(),
		DownloadTTL:    60 * time.Second,
		UploadSem:      NewSemaphore(1),
		MaxFileBytes:   1024,
	})

	req := httptest.NewRequest(http.MethodPost, "/api/files/nope/download-token", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d body=%s", rr.Code, rr.Body.String())
	}
}

func TestDownloadTokenOneTime(t *testing.T) {
	s, err := store.NewInMemoryStore(store.NewParams{MaxFiles: 10, MaxTotalBytes: 1024 * 1024})
	if err != nil {
		t.Fatal(err)
	}
	meta, err := s.Add(store.AddParams{Name: "a.txt", Bytes: []byte("hello")})
	if err != nil {
		t.Fatal(err)
	}

	ts := tokens.NewStore(tokens.Options{})
	t.Cleanup(ts.Close)

	h := NewRouter(RouterDeps{
		ExternalOrigin: "http://127.0.0.1:8080",
		Store:          s,
		Tokens:         ts,
		DownloadTTL:    60 * time.Second,
		UploadSem:      NewSemaphore(1),
		MaxFileBytes:   1024,
	})

	// 1) Create token
	req := httptest.NewRequest(http.MethodPost, "/api/files/"+meta.ID+"/download-token", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rr.Code, rr.Body.String())
	}
	var tok downloadTokenResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &tok); err != nil {
		t.Fatalf("unmarshal: %v body=%s", err, rr.Body.String())
	}
	if tok.Token == "" || tok.URL == "" {
		t.Fatalf("unexpected token response: %#v", tok)
	}

	// 2) Download once
	req2 := httptest.NewRequest(http.MethodGet, tok.URL, nil)
	rr2 := httptest.NewRecorder()
	h.ServeHTTP(rr2, req2)
	if rr2.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rr2.Code, rr2.Body.String())
	}
	if got := rr2.Body.String(); got != "hello" {
		t.Fatalf("unexpected body: %q", got)
	}
	if cd := rr2.Header().Get("Content-Disposition"); cd == "" || !strings.HasPrefix(cd, "attachment") {
		t.Fatalf("unexpected content-disposition: %q", cd)
	}

	// 3) Download again must fail (one-time)
	req3 := httptest.NewRequest(http.MethodGet, tok.URL, nil)
	rr3 := httptest.NewRecorder()
	h.ServeHTTP(rr3, req3)
	if rr3.Code != http.StatusGone {
		t.Fatalf("expected 410, got %d body=%s", rr3.Code, rr3.Body.String())
	}
}
