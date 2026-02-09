package httpapi

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"go-learn/internal/store"
	"go-learn/internal/tokens"
)

func TestCreateBridgeUploadAndQRCode(t *testing.T) {
	s, err := store.NewInMemoryStore(store.NewParams{MaxFiles: 10, MaxTotalBytes: 1024 * 1024})
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
		BridgeTTL:      300 * time.Second,
		UploadSem:      NewSemaphore(1),
		MaxFileBytes:   1024,
	})

	req := httptest.NewRequest(http.MethodPost, "/api/bridge/upload", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rr.Code, rr.Body.String())
	}

	var out bridgeCreateResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &out); err != nil {
		t.Fatalf("unmarshal: %v body=%s", err, rr.Body.String())
	}
	if out.BridgeToken == "" || !strings.HasPrefix(out.PageURL, "/m/upload/") || !strings.HasPrefix(out.QRURL, "/qrcode/") {
		t.Fatalf("unexpected response: %#v", out)
	}

	reqQR := httptest.NewRequest(http.MethodGet, out.QRURL, nil)
	rrQR := httptest.NewRecorder()
	h.ServeHTTP(rrQR, reqQR)
	if rrQR.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rrQR.Code, rrQR.Body.String())
	}
	if ct := rrQR.Header().Get("Content-Type"); ct != "image/png" {
		t.Fatalf("unexpected content-type: %q", ct)
	}
	if rrQR.Body.Len() == 0 {
		t.Fatal("expected non-empty QR png")
	}
}

func TestBridgeUploadOneTime(t *testing.T) {
	s, err := store.NewInMemoryStore(store.NewParams{MaxFiles: 10, MaxTotalBytes: 1024 * 1024})
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
		BridgeTTL:      300 * time.Second,
		UploadSem:      NewSemaphore(1),
		MaxFileBytes:   1024,
	})

	createReq := httptest.NewRequest(http.MethodPost, "/api/bridge/upload", nil)
	createRR := httptest.NewRecorder()
	h.ServeHTTP(createRR, createReq)
	if createRR.Code != http.StatusOK {
		t.Fatalf("create bridge expected 200, got %d body=%s", createRR.Code, createRR.Body.String())
	}
	var bridge bridgeCreateResponse
	if err := json.Unmarshal(createRR.Body.Bytes(), &bridge); err != nil {
		t.Fatalf("unmarshal: %v body=%s", err, createRR.Body.String())
	}

	body, contentType := newMultipartBody(t, "from-phone.txt", []byte("hello"))
	upReq := httptest.NewRequest(http.MethodPost, "/api/bridge/"+bridge.BridgeToken+"/upload", bytes.NewReader(body))
	upReq.Header.Set("Content-Type", contentType)
	upReq.ContentLength = int64(len(body))
	upRR := httptest.NewRecorder()
	h.ServeHTTP(upRR, upReq)
	if upRR.Code != http.StatusCreated {
		t.Fatalf("upload expected 201, got %d body=%s", upRR.Code, upRR.Body.String())
	}

	againReq := httptest.NewRequest(http.MethodPost, "/api/bridge/"+bridge.BridgeToken+"/upload", bytes.NewReader(body))
	againReq.Header.Set("Content-Type", contentType)
	againReq.ContentLength = int64(len(body))
	againRR := httptest.NewRecorder()
	h.ServeHTTP(againRR, againReq)
	if againRR.Code != http.StatusGone {
		t.Fatalf("second upload expected 410, got %d body=%s", againRR.Code, againRR.Body.String())
	}
}

func TestBridgeDownloadTokenOneTime(t *testing.T) {
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
		BridgeTTL:      300 * time.Second,
		UploadSem:      NewSemaphore(1),
		MaxFileBytes:   1024,
	})

	reqBody := []byte(`{"fileId":"` + meta.ID + `"}`)
	createReq := httptest.NewRequest(http.MethodPost, "/api/bridge/download", bytes.NewReader(reqBody))
	createReq.Header.Set("Content-Type", "application/json")
	createRR := httptest.NewRecorder()
	h.ServeHTTP(createRR, createReq)
	if createRR.Code != http.StatusOK {
		t.Fatalf("create bridge expected 200, got %d body=%s", createRR.Code, createRR.Body.String())
	}
	var bridge bridgeCreateResponse
	if err := json.Unmarshal(createRR.Body.Bytes(), &bridge); err != nil {
		t.Fatalf("unmarshal: %v body=%s", err, createRR.Body.String())
	}
	if !strings.HasPrefix(bridge.PageURL, "/m/download/") {
		t.Fatalf("unexpected pageURL: %q", bridge.PageURL)
	}

	pageReq := httptest.NewRequest(http.MethodGet, bridge.PageURL, nil)
	pageRR := httptest.NewRecorder()
	h.ServeHTTP(pageRR, pageReq)
	if pageRR.Code != http.StatusOK {
		t.Fatalf("mobile page expected 200, got %d body=%s", pageRR.Code, pageRR.Body.String())
	}

	dlTokenReq := httptest.NewRequest(http.MethodPost, "/api/bridge/"+bridge.BridgeToken+"/download-token", nil)
	dlTokenRR := httptest.NewRecorder()
	h.ServeHTTP(dlTokenRR, dlTokenReq)
	if dlTokenRR.Code != http.StatusOK {
		t.Fatalf("bridge download-token expected 200, got %d body=%s", dlTokenRR.Code, dlTokenRR.Body.String())
	}
	var dlResp downloadTokenResponse
	if err := json.Unmarshal(dlTokenRR.Body.Bytes(), &dlResp); err != nil {
		t.Fatalf("unmarshal: %v body=%s", err, dlTokenRR.Body.String())
	}
	if dlResp.URL == "" {
		t.Fatalf("unexpected response: %#v", dlResp)
	}

	downloadReq := httptest.NewRequest(http.MethodGet, dlResp.URL, nil)
	downloadRR := httptest.NewRecorder()
	h.ServeHTTP(downloadRR, downloadReq)
	if downloadRR.Code != http.StatusOK {
		t.Fatalf("download expected 200, got %d body=%s", downloadRR.Code, downloadRR.Body.String())
	}
	if got := downloadRR.Body.String(); got != "hello" {
		t.Fatalf("unexpected download body: %q", got)
	}

	againReq := httptest.NewRequest(http.MethodPost, "/api/bridge/"+bridge.BridgeToken+"/download-token", nil)
	againRR := httptest.NewRecorder()
	h.ServeHTTP(againRR, againReq)
	if againRR.Code != http.StatusGone {
		t.Fatalf("second bridge download-token expected 410, got %d body=%s", againRR.Code, againRR.Body.String())
	}
}

