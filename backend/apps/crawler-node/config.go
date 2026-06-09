package main

import (
	"fmt"
	"os"
	"strconv"
)

// Config holds all runtime configuration for the crawler-node binary.
type Config struct {
	OrchestratorURL       string
	CrawlerToken          string
	PollIntervalSeconds   int
	MaxConcurrent         int
	AdapterTimeoutSeconds int
	HTTPTimeoutSeconds    int
	RetryMaxAttempts      int
	LogLevel              string
}

// Load reads configuration from environment variables with sensible defaults.
func Load() *Config {
	return &Config{
		OrchestratorURL:       os.Getenv("ORCHESTRATOR_URL"),
		CrawlerToken:          os.Getenv("CRAWLER_TOKEN"),
		PollIntervalSeconds:   envInt("POLL_INTERVAL_SECONDS", 10),
		MaxConcurrent:         envInt("MAX_CONCURRENT", 3),
		AdapterTimeoutSeconds: envInt("ADAPTER_TIMEOUT_SECONDS", 600),
		HTTPTimeoutSeconds:    envInt("HTTP_TIMEOUT_SECONDS", 15),
		RetryMaxAttempts:      envInt("RETRY_MAX_ATTEMPTS", 3),
		LogLevel:              envStr("LOG_LEVEL", "info"),
	}
}

// Validate returns an error if any required field is missing or any numeric
// field falls outside its allowed range.
func (c *Config) Validate() error {
	if c.OrchestratorURL == "" {
		return fmt.Errorf("config: ORCHESTRATOR_URL is required")
	}
	if c.CrawlerToken == "" {
		return fmt.Errorf("config: CRAWLER_TOKEN is required")
	}
	if c.PollIntervalSeconds < 1 {
		return fmt.Errorf("config: POLL_INTERVAL_SECONDS must be >= 1, got %d", c.PollIntervalSeconds)
	}
	if c.MaxConcurrent < 1 || c.MaxConcurrent > 20 {
		return fmt.Errorf("config: MAX_CONCURRENT must be in [1, 20], got %d", c.MaxConcurrent)
	}
	if c.AdapterTimeoutSeconds < 30 {
		return fmt.Errorf("config: ADAPTER_TIMEOUT_SECONDS must be >= 30, got %d", c.AdapterTimeoutSeconds)
	}
	if c.HTTPTimeoutSeconds < 5 {
		return fmt.Errorf("config: HTTP_TIMEOUT_SECONDS must be >= 5, got %d", c.HTTPTimeoutSeconds)
	}
	if c.RetryMaxAttempts < 1 || c.RetryMaxAttempts > 10 {
		return fmt.Errorf("config: RETRY_MAX_ATTEMPTS must be in [1, 10], got %d", c.RetryMaxAttempts)
	}
	return nil
}

func envInt(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return fallback
}

func envStr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
