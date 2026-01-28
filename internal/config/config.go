package config

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"go.yaml.in/yaml/v3"
)

type Config struct {
	Server ServerConfig `yaml:"server"`
	Limits LimitsConfig `yaml:"limits"`
	Tokens TokensConfig `yaml:"tokens"`
}

type ServerConfig struct {
	Listen   string         `yaml:"listen"`
	BaseURL  string         `yaml:"base_url"`
	Timeouts TimeoutsConfig `yaml:"timeouts"`
}

type TimeoutsConfig struct {
	ReadHeaderSeconds int `yaml:"read_header_seconds"`
	ReadSeconds       int `yaml:"read_seconds"`
	WriteSeconds      int `yaml:"write_seconds"`
	IdleSeconds       int `yaml:"idle_seconds"`
}

func (t TimeoutsConfig) ReadHeader() time.Duration { return time.Duration(t.ReadHeaderSeconds) * time.Second }
func (t TimeoutsConfig) Read() time.Duration       { return time.Duration(t.ReadSeconds) * time.Second }
func (t TimeoutsConfig) Write() time.Duration      { return time.Duration(t.WriteSeconds) * time.Second }
func (t TimeoutsConfig) Idle() time.Duration       { return time.Duration(t.IdleSeconds) * time.Second }

type LimitsConfig struct {
	MaxFileSizeMB        int `yaml:"max_file_size_mb"`
	MaxFiles             int `yaml:"max_files"`
	MaxTotalSizeMB       int `yaml:"max_total_size_mb"`
	UploadConcurrency    int `yaml:"upload_concurrency"`
	TranscodeConcurrency int `yaml:"transcode_concurrency"`
}

type TokensConfig struct {
	DownloadTTLSeconds int `yaml:"download_ttl_seconds"`
	BridgeTTLSeconds   int `yaml:"bridge_ttl_seconds"`
}

func Load(path string) (Config, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return Config{}, fmt.Errorf("read config file: %w", err)
	}

	var cfg Config
	if err := yaml.Unmarshal(b, &cfg); err != nil {
		return Config{}, fmt.Errorf("parse yaml: %w", err)
	}
	cfg.applyDefaults()
	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func (c *Config) applyDefaults() {
	if strings.TrimSpace(c.Server.Listen) == "" {
		c.Server.Listen = "0.0.0.0:8080"
	}

	if c.Server.Timeouts.ReadHeaderSeconds == 0 {
		c.Server.Timeouts.ReadHeaderSeconds = 5
	}
	if c.Server.Timeouts.ReadSeconds == 0 {
		c.Server.Timeouts.ReadSeconds = 300
	}
	if c.Server.Timeouts.WriteSeconds == 0 {
		c.Server.Timeouts.WriteSeconds = 300
	}
	if c.Server.Timeouts.IdleSeconds == 0 {
		c.Server.Timeouts.IdleSeconds = 60
	}

	if c.Limits.MaxFileSizeMB == 0 {
		c.Limits.MaxFileSizeMB = 100
	}
	if c.Limits.MaxFiles == 0 {
		c.Limits.MaxFiles = 10000
	}
	if c.Limits.MaxTotalSizeMB == 0 {
		c.Limits.MaxTotalSizeMB = 300
	}
	if c.Limits.UploadConcurrency == 0 {
		c.Limits.UploadConcurrency = 16
	}
	if c.Limits.TranscodeConcurrency == 0 {
		c.Limits.TranscodeConcurrency = 2
	}

	if c.Tokens.DownloadTTLSeconds == 0 {
		c.Tokens.DownloadTTLSeconds = 60
	}
	if c.Tokens.BridgeTTLSeconds == 0 {
		c.Tokens.BridgeTTLSeconds = 300
	}
}

func (c Config) Validate() error {
	var errs []error

	if strings.TrimSpace(c.Server.Listen) == "" {
		errs = append(errs, errors.New("server.listen is required"))
	} else if _, _, err := net.SplitHostPort(c.Server.Listen); err != nil {
		errs = append(errs, fmt.Errorf("server.listen must be host:port: %w", err))
	}

	if strings.TrimSpace(c.Server.BaseURL) == "" {
		errs = append(errs, errors.New("server.base_url is required (phone-reachable LAN origin, e.g. http://192.168.1.10)"))
	} else if _, err := parseBaseURL(c.Server.BaseURL); err != nil {
		errs = append(errs, fmt.Errorf("server.base_url invalid: %w", err))
	}

	if c.Server.Timeouts.ReadHeaderSeconds <= 0 {
		errs = append(errs, errors.New("server.timeouts.read_header_seconds must be > 0"))
	}
	if c.Server.Timeouts.ReadSeconds <= 0 {
		errs = append(errs, errors.New("server.timeouts.read_seconds must be > 0"))
	}
	if c.Server.Timeouts.WriteSeconds <= 0 {
		errs = append(errs, errors.New("server.timeouts.write_seconds must be > 0"))
	}
	if c.Server.Timeouts.IdleSeconds <= 0 {
		errs = append(errs, errors.New("server.timeouts.idle_seconds must be > 0"))
	}

	if c.Limits.MaxFileSizeMB <= 0 {
		errs = append(errs, errors.New("limits.max_file_size_mb must be > 0"))
	}
	if c.Limits.MaxFileSizeMB > 100 {
		errs = append(errs, errors.New("limits.max_file_size_mb must be <= 100 (requirement)"))
	}
	if c.Limits.MaxFiles <= 0 {
		errs = append(errs, errors.New("limits.max_files must be > 0"))
	}
	if c.Limits.MaxTotalSizeMB <= 0 {
		errs = append(errs, errors.New("limits.max_total_size_mb must be > 0"))
	}
	if c.Limits.UploadConcurrency <= 0 {
		errs = append(errs, errors.New("limits.upload_concurrency must be > 0"))
	}
	if c.Limits.TranscodeConcurrency <= 0 {
		errs = append(errs, errors.New("limits.transcode_concurrency must be > 0"))
	}

	if c.Tokens.DownloadTTLSeconds <= 0 {
		errs = append(errs, errors.New("tokens.download_ttl_seconds must be > 0"))
	}
	if c.Tokens.BridgeTTLSeconds <= 0 {
		errs = append(errs, errors.New("tokens.bridge_ttl_seconds must be > 0"))
	}

	return errors.Join(errs...)
}

func (c Config) ExternalOrigin() (string, error) {
	u, err := parseBaseURL(c.Server.BaseURL)
	if err != nil {
		return "", err
	}

	host := u.Host
	if _, _, err := net.SplitHostPort(host); err == nil {
		return u.Scheme + "://" + host, nil
	}

	_, listenPort, err := net.SplitHostPort(c.Server.Listen)
	if err != nil {
		return "", fmt.Errorf("parse server.listen: %w", err)
	}
	if listenPort == "80" {
		return u.Scheme + "://" + host, nil
	}
	return u.Scheme + "://" + host + ":" + listenPort, nil
}

func parseBaseURL(raw string) (*url.URL, error) {
	raw = strings.TrimSpace(raw)
	u, err := url.Parse(raw)
	if err != nil {
		return nil, err
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return nil, errors.New("scheme must be http or https")
	}
	if u.Host == "" {
		return nil, errors.New("host is required")
	}
	if u.User != nil {
		return nil, errors.New("userinfo is not allowed")
	}
	if u.RawQuery != "" || u.Fragment != "" {
		return nil, errors.New("query/fragment is not allowed")
	}
	if u.Path != "" && u.Path != "/" {
		return nil, errors.New("path is not allowed; base_url should be an origin like http://192.168.1.10")
	}

	u.Path = ""
	u.RawPath = ""
	u.ForceQuery = false

	if _, port, err := net.SplitHostPort(u.Host); err == nil {
		if port == "" {
			return nil, errors.New("port is empty")
		}
		if _, err := strconv.Atoi(port); err != nil {
			return nil, errors.New("port must be numeric")
		}
	}

	return u, nil
}

