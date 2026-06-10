package main

import (
	"os"
	"testing"
)

func TestLoad_Defaults(t *testing.T) {
	// Clear all relevant env vars.
	for _, k := range []string{
		"ORCHESTRATOR_URL", "CRAWLER_TOKEN",
		"POLL_INTERVAL_SECONDS", "MAX_CONCURRENT",
		"ADAPTER_TIMEOUT_SECONDS", "HTTP_TIMEOUT_SECONDS",
		"RETRY_MAX_ATTEMPTS", "LOG_LEVEL",
	} {
		os.Unsetenv(k) //nolint:errcheck
	}

	cfg := Load()

	if cfg.PollIntervalSeconds != 10 {
		t.Errorf("PollIntervalSeconds default = %d, want 10", cfg.PollIntervalSeconds)
	}
	if cfg.MaxConcurrent != 3 {
		t.Errorf("MaxConcurrent default = %d, want 3", cfg.MaxConcurrent)
	}
	if cfg.AdapterTimeoutSeconds != 600 {
		t.Errorf("AdapterTimeoutSeconds default = %d, want 600", cfg.AdapterTimeoutSeconds)
	}
	if cfg.HTTPTimeoutSeconds != 15 {
		t.Errorf("HTTPTimeoutSeconds default = %d, want 15", cfg.HTTPTimeoutSeconds)
	}
	if cfg.RetryMaxAttempts != 3 {
		t.Errorf("RetryMaxAttempts default = %d, want 3", cfg.RetryMaxAttempts)
	}
	if cfg.LogLevel != "info" {
		t.Errorf("LogLevel default = %q, want info", cfg.LogLevel)
	}
}

func TestLoad_EnvOverride(t *testing.T) {
	t.Setenv("ORCHESTRATOR_URL", "https://orch.example.com")
	t.Setenv("CRAWLER_TOKEN", "tok-abc123")
	t.Setenv("POLL_INTERVAL_SECONDS", "20")
	t.Setenv("MAX_CONCURRENT", "5")
	t.Setenv("ADAPTER_TIMEOUT_SECONDS", "300")
	t.Setenv("HTTP_TIMEOUT_SECONDS", "10")
	t.Setenv("RETRY_MAX_ATTEMPTS", "5")
	t.Setenv("LOG_LEVEL", "debug")

	cfg := Load()

	if cfg.OrchestratorURL != "https://orch.example.com" {
		t.Errorf("OrchestratorURL = %q, want https://orch.example.com", cfg.OrchestratorURL)
	}
	if cfg.CrawlerToken != "tok-abc123" {
		t.Errorf("CrawlerToken = %q, want tok-abc123", cfg.CrawlerToken)
	}
	if cfg.PollIntervalSeconds != 20 {
		t.Errorf("PollIntervalSeconds = %d, want 20", cfg.PollIntervalSeconds)
	}
	if cfg.MaxConcurrent != 5 {
		t.Errorf("MaxConcurrent = %d, want 5", cfg.MaxConcurrent)
	}
	if cfg.AdapterTimeoutSeconds != 300 {
		t.Errorf("AdapterTimeoutSeconds = %d, want 300", cfg.AdapterTimeoutSeconds)
	}
	if cfg.HTTPTimeoutSeconds != 10 {
		t.Errorf("HTTPTimeoutSeconds = %d, want 10", cfg.HTTPTimeoutSeconds)
	}
	if cfg.RetryMaxAttempts != 5 {
		t.Errorf("RetryMaxAttempts = %d, want 5", cfg.RetryMaxAttempts)
	}
	if cfg.LogLevel != "debug" {
		t.Errorf("LogLevel = %q, want debug", cfg.LogLevel)
	}
}

func TestValidate_MissingOrchestratorURL(t *testing.T) {
	cfg := &Config{
		CrawlerToken:          "tok",
		PollIntervalSeconds:   10,
		MaxConcurrent:         3,
		AdapterTimeoutSeconds: 600,
		HTTPTimeoutSeconds:    15,
		RetryMaxAttempts:      3,
		LogLevel:              "info",
	}
	if err := cfg.Validate(); err == nil {
		t.Error("expected error for missing OrchestratorURL, got nil")
	}
}

func TestValidate_MissingCrawlerToken(t *testing.T) {
	cfg := &Config{
		OrchestratorURL:       "https://orch.example.com",
		PollIntervalSeconds:   10,
		MaxConcurrent:         3,
		AdapterTimeoutSeconds: 600,
		HTTPTimeoutSeconds:    15,
		RetryMaxAttempts:      3,
		LogLevel:              "info",
	}
	if err := cfg.Validate(); err == nil {
		t.Error("expected error for missing CrawlerToken, got nil")
	}
}

func TestValidate_PollIntervalBelowMin(t *testing.T) {
	cfg := validConfig()
	cfg.PollIntervalSeconds = 0
	if err := cfg.Validate(); err == nil {
		t.Error("expected error for PollIntervalSeconds < 1, got nil")
	}
}

func TestValidate_MaxConcurrentBelowMin(t *testing.T) {
	cfg := validConfig()
	cfg.MaxConcurrent = 0
	if err := cfg.Validate(); err == nil {
		t.Error("expected error for MaxConcurrent < 1, got nil")
	}
}

func TestValidate_MaxConcurrentAboveMax(t *testing.T) {
	cfg := validConfig()
	cfg.MaxConcurrent = 21
	if err := cfg.Validate(); err == nil {
		t.Error("expected error for MaxConcurrent > 20, got nil")
	}
}

func TestValidate_AdapterTimeoutBelowMin(t *testing.T) {
	cfg := validConfig()
	cfg.AdapterTimeoutSeconds = 29
	if err := cfg.Validate(); err == nil {
		t.Error("expected error for AdapterTimeoutSeconds < 30, got nil")
	}
}

func TestValidate_HTTPTimeoutBelowMin(t *testing.T) {
	cfg := validConfig()
	cfg.HTTPTimeoutSeconds = 4
	if err := cfg.Validate(); err == nil {
		t.Error("expected error for HTTPTimeoutSeconds < 5, got nil")
	}
}

func TestValidate_RetryMaxAttemptsBelowMin(t *testing.T) {
	cfg := validConfig()
	cfg.RetryMaxAttempts = 0
	if err := cfg.Validate(); err == nil {
		t.Error("expected error for RetryMaxAttempts < 1, got nil")
	}
}

func TestValidate_RetryMaxAttemptsAboveMax(t *testing.T) {
	cfg := validConfig()
	cfg.RetryMaxAttempts = 11
	if err := cfg.Validate(); err == nil {
		t.Error("expected error for RetryMaxAttempts > 10, got nil")
	}
}

func TestValidate_Valid(t *testing.T) {
	cfg := validConfig()
	if err := cfg.Validate(); err != nil {
		t.Errorf("unexpected error for valid config: %v", err)
	}
}

func TestValidate_BoundaryValues(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*Config)
		wantErr bool
	}{
		{"poll=1 ok", func(c *Config) { c.PollIntervalSeconds = 1 }, false},
		{"maxconcurrent=1 ok", func(c *Config) { c.MaxConcurrent = 1 }, false},
		{"maxconcurrent=20 ok", func(c *Config) { c.MaxConcurrent = 20 }, false},
		{"adapter=30 ok", func(c *Config) { c.AdapterTimeoutSeconds = 30 }, false},
		{"http=5 ok", func(c *Config) { c.HTTPTimeoutSeconds = 5 }, false},
		{"retry=1 ok", func(c *Config) { c.RetryMaxAttempts = 1 }, false},
		{"retry=10 ok", func(c *Config) { c.RetryMaxAttempts = 10 }, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := validConfig()
			tc.mutate(cfg)
			err := cfg.Validate()
			if (err != nil) != tc.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tc.wantErr)
			}
		})
	}
}

func validConfig() *Config {
	return &Config{
		OrchestratorURL:       "https://orch.example.com",
		CrawlerToken:          "tok",
		PollIntervalSeconds:   10,
		MaxConcurrent:         3,
		AdapterTimeoutSeconds: 600,
		HTTPTimeoutSeconds:    15,
		RetryMaxAttempts:      3,
		LogLevel:              "info",
	}
}
