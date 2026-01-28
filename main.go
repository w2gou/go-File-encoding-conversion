package main

import (
	"flag"
	"log"
	"os"

	"go-learn/internal/config"
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
}
