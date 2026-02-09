package main

import (
	"flag"
	"log"
	"net/http"
	"os"
	"time"

	"go-learn/internal/config"
	"go-learn/internal/httpapi"
	"go-learn/internal/store"
	"go-learn/internal/tokens"
)

func main() {
	configPath := flag.String("config", "./config.yaml", "config file path (YAML)")
	flag.Parse()

	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Printf("config error: %v", err)
		os.Exit(2)
	}

	origin, err := cfg.ExternalOrigin()
	if err != nil {
		log.Printf("config error: %v", err)
		os.Exit(2)
	}

	log.Printf("config loaded: listen=%s base_url=%s external_origin=%s", cfg.Server.Listen, cfg.Server.BaseURL, origin)
	log.Printf("limits: max_file_size_mb=%d max_files=%d max_total_size_mb=%d upload_concurrency=%d transcode_concurrency=%d",
		cfg.Limits.MaxFileSizeMB, cfg.Limits.MaxFiles, cfg.Limits.MaxTotalSizeMB, cfg.Limits.UploadConcurrency, cfg.Limits.TranscodeConcurrency)
	log.Printf("tokens: download_ttl_seconds=%d bridge_ttl_seconds=%d", cfg.Tokens.DownloadTTLSeconds, cfg.Tokens.BridgeTTLSeconds)

	memStore, err := store.NewInMemoryStore(store.NewParams{
		MaxFiles:      cfg.Limits.MaxFiles,
		MaxTotalBytes: int64(cfg.Limits.MaxTotalSizeMB) * 1024 * 1024,
	})
	if err != nil {
		log.Printf("store init error: %v", err)
		os.Exit(2)
	}

	tokenStore := tokens.NewStore(tokens.Options{
		CleanupInterval: 30 * time.Second,
	})
	defer tokenStore.Close()

	handler := httpapi.NewRouter(httpapi.RouterDeps{
		ExternalOrigin:  origin,
		Store:           memStore,
		Tokens:          tokenStore,
		DownloadTTL:     time.Duration(cfg.Tokens.DownloadTTLSeconds) * time.Second,
		UploadSem:       httpapi.NewSemaphore(cfg.Limits.UploadConcurrency),
		MaxFileBytes:    int64(cfg.Limits.MaxFileSizeMB) * 1024 * 1024,
		MaxRequestBytes: int64(cfg.Limits.MaxFileSizeMB)*1024*1024 + 2*1024*1024,
	})

	srv := &http.Server{
		Addr:              cfg.Server.Listen,
		Handler:           handler,
		ReadHeaderTimeout: cfg.Server.Timeouts.ReadHeader(),
		ReadTimeout:       cfg.Server.Timeouts.Read(),
		WriteTimeout:      cfg.Server.Timeouts.Write(),
		IdleTimeout:       cfg.Server.Timeouts.Idle(),
	}

	log.Printf("listening on %s", cfg.Server.Listen)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Printf("server error: %v", err)
		os.Exit(1)
	}
}
