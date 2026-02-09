package httpapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"go-learn/internal/store"
	"go-learn/internal/tokens"
)

func TestRootPageAndAssets(t *testing.T) {
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

	rootReq := httptest.NewRequest(http.MethodGet, "/", nil)
	rootRR := httptest.NewRecorder()
	h.ServeHTTP(rootRR, rootReq)
	if rootRR.Code != http.StatusOK {
		t.Fatalf("root status=%d body=%s", rootRR.Code, rootRR.Body.String())
	}
	if !strings.Contains(rootRR.Body.String(), "File Encoding Conversion") {
		t.Fatalf("unexpected root body=%s", rootRR.Body.String())
	}

	assetReq := httptest.NewRequest(http.MethodGet, "/assets/app.js", nil)
	assetRR := httptest.NewRecorder()
	h.ServeHTTP(assetRR, assetReq)
	if assetRR.Code != http.StatusOK {
		t.Fatalf("asset status=%d body=%s", assetRR.Code, assetRR.Body.String())
	}
}

