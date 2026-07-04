package config_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/trushilpatel/jobradar/common/config"
)

// ---------- DatabaseConfig.DSN ----------

func TestDatabaseConfig_DSN(t *testing.T) {
	db := config.DatabaseConfig{
		Host:     "h",
		Port:     "1234",
		User:     "u",
		Password: "p",
		Name:     "n",
		SSLMode:  "disable",
	}
	got := db.DSN()
	want := "host=h port=1234 user=u password=p dbname=n sslmode=disable statement_cache_capacity=0 default_query_exec_mode=simple_protocol"
	if got != want {
		t.Fatalf("DSN mismatch:\n got:  %q\nwant: %q", got, want)
	}
}

// ---------- DatabaseConfig.Validate (pool timeouts) ----------

func TestDatabaseConfig_Validate(t *testing.T) {
	base := func() config.DatabaseConfig {
		return config.DatabaseConfig{ConnMaxIdleSeconds: 30, ConnMaxLifetimeMinutes: 15}
	}

	t.Run("valid defaults", func(t *testing.T) {
		if err := base().Validate(); err != nil {
			t.Fatalf("valid config returned error: %v", err)
		}
	})
	t.Run("idle equal to lifetime boundary", func(t *testing.T) {
		// idle == lifetime*60 is the accepted upper boundary.
		cfg := config.DatabaseConfig{ConnMaxIdleSeconds: 900, ConnMaxLifetimeMinutes: 15}
		if err := cfg.Validate(); err != nil {
			t.Fatalf("idle==lifetime*60 boundary should pass: %v", err)
		}
	})
	t.Run("idle one over lifetime boundary", func(t *testing.T) {
		cfg := config.DatabaseConfig{ConnMaxIdleSeconds: 901, ConnMaxLifetimeMinutes: 15}
		err := cfg.Validate()
		if err == nil || !strings.Contains(err.Error(), "DB_CONN_MAX_IDLE_SECONDS") {
			t.Fatalf("want idle>lifetime error, got %v", err)
		}
	})
	t.Run("zero idle", func(t *testing.T) {
		cfg := base()
		cfg.ConnMaxIdleSeconds = 0
		err := cfg.Validate()
		if err == nil || !strings.Contains(err.Error(), "DB_CONN_MAX_IDLE_SECONDS") {
			t.Fatalf("want idle>0 error, got %v", err)
		}
	})
	t.Run("negative idle", func(t *testing.T) {
		cfg := base()
		cfg.ConnMaxIdleSeconds = -1
		if err := cfg.Validate(); err == nil {
			t.Fatal("expected error for negative idle")
		}
	})
	t.Run("zero lifetime", func(t *testing.T) {
		cfg := base()
		cfg.ConnMaxLifetimeMinutes = 0
		err := cfg.Validate()
		if err == nil || !strings.Contains(err.Error(), "DB_CONN_MAX_LIFETIME_MINUTES") {
			t.Fatalf("want lifetime>0 error, got %v", err)
		}
	})
	t.Run("negative lifetime", func(t *testing.T) {
		cfg := base()
		cfg.ConnMaxLifetimeMinutes = -5
		if err := cfg.Validate(); err == nil {
			t.Fatal("expected error for negative lifetime")
		}
	})
}

// ---------- AuthConfig.Validate (Clerk) ----------

func TestAuthConfig_ClerkValidate(t *testing.T) {
	t.Run("valid clerk config", func(t *testing.T) {
		cfg := config.AuthConfig{
			ClerkJWKSURL:   "https://example.clerk.accounts.dev/.well-known/jwks.json",
			ClerkIssuerURL: "https://example.clerk.accounts.dev",
		}
		if err := cfg.Validate(); err != nil {
			t.Fatalf("valid clerk config returned error: %v", err)
		}
	})
	t.Run("missing JWKS URL", func(t *testing.T) {
		cfg := config.AuthConfig{ClerkIssuerURL: "https://example.clerk.accounts.dev"}
		err := cfg.Validate()
		if err == nil || !strings.Contains(err.Error(), "CLERK_JWKS_URL") {
			t.Fatalf("want CLERK_JWKS_URL error, got %v", err)
		}
	})
	t.Run("missing issuer URL", func(t *testing.T) {
		cfg := config.AuthConfig{ClerkJWKSURL: "https://x.clerk.accounts.dev/.well-known/jwks.json"}
		err := cfg.Validate()
		if err == nil || !strings.Contains(err.Error(), "CLERK_ISSUER_URL") {
			t.Fatalf("want CLERK_ISSUER_URL error, got %v", err)
		}
	})
	t.Run("both empty", func(t *testing.T) {
		cfg := config.AuthConfig{}
		if err := cfg.Validate(); err == nil {
			t.Fatal("expected error when both Clerk fields are empty")
		}
	})
}

// ---------- SchedulerConfig.Validate ----------

func TestSchedulerConfig_Validate(t *testing.T) {
	tests := []struct {
		name    string
		cfg     config.SchedulerConfig
		wantSub string
	}{
		{"zero ScanIntervalMinutes", config.SchedulerConfig{ScanIntervalMinutes: 0, StuckTimeoutMinutes: 30, CheckIntervalSeconds: 60}, "SCAN_INTERVAL_MINUTES"},
		{"negative ScanIntervalMinutes", config.SchedulerConfig{ScanIntervalMinutes: -1, StuckTimeoutMinutes: 30, CheckIntervalSeconds: 60}, "SCAN_INTERVAL_MINUTES"},
		{"zero StuckTimeoutMinutes", config.SchedulerConfig{ScanIntervalMinutes: 60, StuckTimeoutMinutes: 0, CheckIntervalSeconds: 60}, "SCHEDULER_STUCK_TIMEOUT_MINUTES must be > 0"},
		{"StuckTimeoutMinutes too large", config.SchedulerConfig{ScanIntervalMinutes: 60, StuckTimeoutMinutes: 1441, CheckIntervalSeconds: 60}, "must be <= 1440"},
		{"zero CheckIntervalSeconds", config.SchedulerConfig{ScanIntervalMinutes: 60, StuckTimeoutMinutes: 30, CheckIntervalSeconds: 0}, "SCHEDULER_CHECK_INTERVAL_SECONDS"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cfg.Validate()
			if err == nil || !strings.Contains(err.Error(), tt.wantSub) {
				t.Fatalf("want error containing %q, got %v", tt.wantSub, err)
			}
		})
	}
	t.Run("happy path", func(t *testing.T) {
		c := config.SchedulerConfig{ScanIntervalMinutes: 60, StuckTimeoutMinutes: 30, CheckIntervalSeconds: 60}
		if err := c.Validate(); err != nil {
			t.Fatalf("happy path returned error: %v", err)
		}
	})
	t.Run("StuckTimeoutMinutes at boundary 1440", func(t *testing.T) {
		c := config.SchedulerConfig{ScanIntervalMinutes: 60, StuckTimeoutMinutes: 1440, CheckIntervalSeconds: 60}
		if err := c.Validate(); err != nil {
			t.Fatalf("boundary value should pass, got: %v", err)
		}
	})
}

// ---------- ParseLLMSlots ----------

func TestParseLLMSlots(t *testing.T) {
	t.Run("empty returns nil without error", func(t *testing.T) {
		got, err := config.ParseLLMSlots("")
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if got != nil {
			t.Fatalf("expected nil, got %+v", got)
		}
	})
	t.Run("valid JSON", func(t *testing.T) {
		raw := `[{"key_id":"k1","api_key":"sk-A","model":"m1","rpm":10,"rpd":100,"tpm_in":1000}]`
		got, err := config.ParseLLMSlots(raw)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(got) != 1 || got[0].KeyID != "k1" || got[0].APIKey != "sk-A" || got[0].Model != "m1" || got[0].RPM != 10 || got[0].RPD != 100 || got[0].TPMIn != 1000 {
			t.Fatalf("slot mismatch: %+v", got)
		}
	})
	t.Run("malformed JSON returns error", func(t *testing.T) {
		_, err := config.ParseLLMSlots("not-json")
		if err == nil {
			t.Fatal("expected error for malformed JSON")
		}
		if !strings.Contains(err.Error(), "parse LLM slots") {
			t.Fatalf("expected wrapped error, got %v", err)
		}
	})
	t.Run("empty array is empty slice not nil", func(t *testing.T) {
		got, err := config.ParseLLMSlots("[]")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got == nil {
			t.Fatal("expected non-nil empty slice")
		}
		if len(got) != 0 {
			t.Fatalf("expected empty slice, got %d entries", len(got))
		}
	})
}

// ---------- LoadLLMSlotsYAML (extensions) ----------

func TestLoadLLMSlotsYAML_MalformedYAML(t *testing.T) {
	bad := "filter:\n  slots: [: not yaml"
	path := filepath.Join(t.TempDir(), "bad.yaml")
	if err := os.WriteFile(path, []byte(bad), 0o600); err != nil {
		t.Fatal(err)
	}
	_, _, err := config.LoadLLMSlotsYAML(path)
	if err == nil {
		t.Fatal("expected error for malformed YAML")
	}
	if !strings.Contains(err.Error(), "parse llm slots yaml") {
		t.Fatalf("expected wrapped error, got %v", err)
	}
}

func TestLoadLLMSlotsYAML_MultipleMissingVarsAreReported(t *testing.T) {
	// Ensure both vars are unset.
	_ = os.Unsetenv("MISSING_A_XYZ")
	_ = os.Unsetenv("MISSING_B_XYZ")
	yaml := `filter:
  slots:
    - api_key: ${MISSING_A_XYZ}
enricher:
  slots:
    - api_key: ${MISSING_B_XYZ}
`
	path := filepath.Join(t.TempDir(), "two.yaml")
	if err := os.WriteFile(path, []byte(yaml), 0o600); err != nil {
		t.Fatal(err)
	}
	_, _, err := config.LoadLLMSlotsYAML(path)
	if err == nil {
		t.Fatal("expected error for unresolved env refs")
	}
	if !strings.Contains(err.Error(), "MISSING_A_XYZ") || !strings.Contains(err.Error(), "MISSING_B_XYZ") {
		t.Fatalf("expected both names in error, got: %v", err)
	}
}

func TestLoadLLMSlotsYAML_EmptyStringEnvVarIsResolved(t *testing.T) {
	// Set var to empty string explicitly; should NOT be reported as missing.
	t.Setenv("RESOLVES_TO_EMPTY", "")
	yaml := `filter:
  slots:
    - key_id: k
      api_key: "${RESOLVES_TO_EMPTY}"
enricher:
  slots: []
`
	path := filepath.Join(t.TempDir(), "empty.yaml")
	if err := os.WriteFile(path, []byte(yaml), 0o600); err != nil {
		t.Fatal(err)
	}
	filter, _, err := config.LoadLLMSlotsYAML(path)
	if err != nil {
		t.Fatalf("expected no error for empty-string env, got %v", err)
	}
	if len(filter) != 1 || filter[0].APIKey != "" {
		t.Fatalf("expected one slot with empty api key, got %+v", filter)
	}
}

// ---------- Load (defaults vs overrides) ----------

// clearConfigEnv removes every env var Load() reads so that a test starts from
// a known-clean baseline. Uses t.Setenv with empty so the framework restores
// the prior value on test cleanup.
func clearConfigEnv(t *testing.T) {
	t.Helper()
	keys := []string{
		"LLM_SLOTS_CONFIG", "FILTER_LLM_SLOTS", "ENRICHER_LLM_SLOTS",
		"DB_HOST", "DB_PORT", "DB_USER", "DB_PASSWORD", "DB_NAME", "DB_SSLMODE",
		"DB_CONN_MAX_IDLE_SECONDS", "DB_CONN_MAX_LIFETIME_MINUTES",
		"KAFKA_BROKERS", "KAFKA_USER", "KAFKA_PASSWORD",
		"REDIS_URL",
		"LLM_COOLDOWN_BASE_SECONDS", "LLM_COOLDOWN_MAX_SECONDS", "LLM_COOLDOWN_JITTER",
		"LLM_PROVIDER", "TARGET_ROLES", "YEARS_EXP",
		"FILTER_BATCH_SIZE", "FILTER_FLUSH_TIMEOUT_SECONDS", "FILTER_LLM_CALL_INTERVAL_SECONDS",
		"ENRICHER_FETCH_CONCURRENCY", "ENRICHER_LLM_BATCH_SIZE", "ENRICHER_FLUSH_TIMEOUT_SECONDS",
		"ENRICHER_LLM_CALL_INTERVAL_SECONDS", "ENRICHER_DEFAULT_RPS", "ENRICHER_DEFAULT_BURST",
		"ENRICHER_JITTER_MS", "DISCARD_JOB_WITH_EXP_GT",
		"SCAN_COMPANY_DELAY_SECONDS", "CRAWLER_POLL_INTERVAL_SECONDS",
		"CRAWLER_STUCK_TIMEOUT_SECONDS", "CRAWLER_HEARTBEAT_INTERVAL_SECONDS",
		"GOOGLE_SCRAPER_MIN_DELAY", "GOOGLE_SCRAPER_MAX_DELAY",
		"GOOGLE_SCRAPER_TIMEOUT_MS", "GOOGLE_SCRAPER_MAX_PAGES",
		"SCAN_INTERVAL_MINUTES", "SCHEDULER_STUCK_TIMEOUT_MINUTES", "SCHEDULER_CHECK_INTERVAL_SECONDS",
		"METRICS_ADDR", "OTEL_EXPORTER_OTLP_ENDPOINT",
		"CLERK_JWKS_URL", "CLERK_ISSUER_URL", "CLERK_SECRET_KEY",
		"API_MAX_BODY_BYTES", "DEV_TOKEN",
		"RATE_LIMIT_READ_RPM", "RATE_LIMIT_WRITE_RPM", "RATE_LIMIT_ADMIN_RPM",
		"CACHE_TTL_USER_SECONDS", "CACHE_TTL_COMPANY_SECONDS",
	}
	for _, k := range keys {
		t.Setenv(k, "")
	}
	// godotenv.Load looks for .env in cwd — point the test at an empty cwd to
	// avoid picking up a real .env file shipped in backend/.
	dir := t.TempDir()
	prev, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(prev) })
}

func TestLoad_DefaultsApplied(t *testing.T) {
	clearConfigEnv(t)

	cfg := config.Load()

	if cfg.DB.Host != "localhost" || cfg.DB.Port != "5432" || cfg.DB.User != "postgres" || cfg.DB.Password != "postgres" || cfg.DB.Name != "jobradar" || cfg.DB.SSLMode != "disable" {
		t.Errorf("DB defaults wrong: %+v", cfg.DB)
	}
	if cfg.DB.ConnMaxIdleSeconds != 30 || cfg.DB.ConnMaxLifetimeMinutes != 15 {
		t.Errorf("DB pool-timeout defaults wrong: idle=%d lifetime=%d (want 30/15)",
			cfg.DB.ConnMaxIdleSeconds, cfg.DB.ConnMaxLifetimeMinutes)
	}
	if cfg.Redis.URL != "redis://localhost:6379/0" {
		t.Errorf("Redis defaults wrong: %+v", cfg.Redis)
	}
	if cfg.Cooldown.BaseSeconds != 60 || cfg.Cooldown.MaxSeconds != 3600 || cfg.Cooldown.Jitter != 0.2 {
		t.Errorf("Cooldown defaults wrong: %+v", cfg.Cooldown)
	}
	if cfg.LLM.Provider != "gemini" || len(cfg.LLM.TargetRoles) != 1 || cfg.LLM.TargetRoles[0] != "Software Engineer" || cfg.LLM.YearsExp != 0 {
		t.Errorf("LLM defaults wrong: %+v", cfg.LLM)
	}
	if cfg.Filter.BatchSize != 50 || cfg.Filter.FlushTimeoutSeconds != 30 || cfg.Filter.LLMCallIntervalSeconds != 5 {
		t.Errorf("Filter defaults wrong: %+v", cfg.Filter)
	}
	if cfg.Enricher.FetchConcurrency != 15 || cfg.Enricher.LLMBatchSize != 5 || cfg.Enricher.FlushTimeoutSeconds != 120 ||
		cfg.Enricher.LLMCallIntervalSeconds != 5 || cfg.Enricher.DefaultRPS != 2 || cfg.Enricher.DefaultBurst != 3 ||
		cfg.Enricher.JitterMs != 200 || cfg.Enricher.MaxExpYears != 0 {
		t.Errorf("Enricher defaults wrong: %+v", cfg.Enricher)
	}
	if cfg.Crawler.CompanyDelaySeconds != 30 || cfg.Crawler.PollIntervalSeconds != 60 ||
		cfg.Crawler.StuckTimeoutSeconds != 120 || cfg.Crawler.HeartbeatIntervalSeconds != 30 {
		t.Errorf("Crawler defaults wrong: %+v", cfg.Crawler)
	}
	if cfg.Crawler.Google.MinDelayMs != 20000 || cfg.Crawler.Google.MaxDelayMs != 40000 ||
		cfg.Crawler.Google.TimeoutMs != 7200000 || cfg.Crawler.Google.MaxPages != 0 {
		t.Errorf("Google scraper defaults wrong: %+v", cfg.Crawler.Google)
	}
	if cfg.Scheduler.ScanIntervalMinutes != 1440 || cfg.Scheduler.StuckTimeoutMinutes != 30 || cfg.Scheduler.CheckIntervalSeconds != 60 {
		t.Errorf("Scheduler defaults wrong: %+v", cfg.Scheduler)
	}
	if cfg.Observability.MetricsAddr != "127.0.0.1:9100" || cfg.Observability.OTelEndpoint != "http://localhost:4318" {
		t.Errorf("Observability defaults wrong: %+v", cfg.Observability)
	}
	if cfg.Auth.ClerkJWKSURL != "" || cfg.Auth.ClerkIssuerURL != "" {
		t.Errorf("Auth defaults should be empty strings: %+v", cfg.Auth)
	}
	if cfg.API.MaxBodyBytes != 3145728 || cfg.API.DevToken != "" {
		t.Errorf("API defaults wrong: %+v", cfg.API)
	}
	if cfg.MultiUser.RateLimitReadRPM != 30 || cfg.MultiUser.RateLimitWriteRPM != 20 ||
		cfg.MultiUser.RateLimitAdminRPM != 120 || cfg.MultiUser.CacheTTLUserSeconds != 60 ||
		cfg.MultiUser.CacheTTLCompanySeconds != 60 ||
		cfg.MultiUser.IdentityCacheLocalTTLSeconds != 30 ||
		cfg.MultiUser.IdentityCacheLocalMaxEntries != 10000 {
		t.Errorf("MultiUser defaults wrong: %+v", cfg.MultiUser)
	}
	if cfg.Filter.Slots != nil || cfg.Enricher.Slots != nil {
		t.Errorf("expected nil slots without env / yaml, got filter=%+v enricher=%+v", cfg.Filter.Slots, cfg.Enricher.Slots)
	}
}

func TestLoad_OverridesApplied(t *testing.T) {
	clearConfigEnv(t)
	t.Setenv("DB_HOST", "db.example")
	t.Setenv("DB_PORT", "6543")
	t.Setenv("DB_USER", "u")
	t.Setenv("DB_PASSWORD", "p")
	t.Setenv("DB_NAME", "d")
	t.Setenv("DB_SSLMODE", "require")
	t.Setenv("DB_CONN_MAX_IDLE_SECONDS", "45")
	t.Setenv("DB_CONN_MAX_LIFETIME_MINUTES", "20")
	t.Setenv("REDIS_URL", "rediss://default:secret@redis.example:6380/5")
	t.Setenv("LLM_COOLDOWN_BASE_SECONDS", "10")
	t.Setenv("LLM_COOLDOWN_MAX_SECONDS", "100")
	t.Setenv("LLM_COOLDOWN_JITTER", "0.5")
	t.Setenv("LLM_PROVIDER", "openai")
	t.Setenv("TARGET_ROLES", "SRE, Platform, Data")
	t.Setenv("YEARS_EXP", "4")
	t.Setenv("FILTER_BATCH_SIZE", "12")
	t.Setenv("FILTER_FLUSH_TIMEOUT_SECONDS", "13")
	t.Setenv("FILTER_LLM_CALL_INTERVAL_SECONDS", "14")
	t.Setenv("ENRICHER_FETCH_CONCURRENCY", "7")
	t.Setenv("ENRICHER_LLM_BATCH_SIZE", "8")
	t.Setenv("ENRICHER_FLUSH_TIMEOUT_SECONDS", "9")
	t.Setenv("ENRICHER_LLM_CALL_INTERVAL_SECONDS", "11")
	t.Setenv("ENRICHER_DEFAULT_RPS", "1.5")
	t.Setenv("ENRICHER_DEFAULT_BURST", "6")
	t.Setenv("ENRICHER_JITTER_MS", "50")
	t.Setenv("DISCARD_JOB_WITH_EXP_GT", "8")
	t.Setenv("SCAN_COMPANY_DELAY_SECONDS", "33")
	t.Setenv("CRAWLER_POLL_INTERVAL_SECONDS", "44")
	t.Setenv("CRAWLER_STUCK_TIMEOUT_SECONDS", "55")
	t.Setenv("CRAWLER_HEARTBEAT_INTERVAL_SECONDS", "66")
	t.Setenv("GOOGLE_SCRAPER_MIN_DELAY", "77")
	t.Setenv("GOOGLE_SCRAPER_MAX_DELAY", "88")
	t.Setenv("GOOGLE_SCRAPER_TIMEOUT_MS", "99")
	t.Setenv("GOOGLE_SCRAPER_MAX_PAGES", "100")
	t.Setenv("SCAN_INTERVAL_MINUTES", "111")
	t.Setenv("SCHEDULER_STUCK_TIMEOUT_MINUTES", "22")
	t.Setenv("SCHEDULER_CHECK_INTERVAL_SECONDS", "33")
	t.Setenv("METRICS_ADDR", ":9999")
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "http://otel:4318")
	t.Setenv("CLERK_JWKS_URL", "https://x.clerk.accounts.dev/.well-known/jwks.json")
	t.Setenv("CLERK_ISSUER_URL", "https://x.clerk.accounts.dev")
	t.Setenv("CLERK_SECRET_KEY", "sk_test_abc123")
	t.Setenv("API_MAX_BODY_BYTES", "2000000")
	t.Setenv("DEV_TOKEN", "devtoken")
	t.Setenv("RATE_LIMIT_READ_RPM", "31")
	t.Setenv("RATE_LIMIT_WRITE_RPM", "32")
	t.Setenv("RATE_LIMIT_ADMIN_RPM", "33")
	t.Setenv("CACHE_TTL_USER_SECONDS", "61")
	t.Setenv("CACHE_TTL_COMPANY_SECONDS", "62")
	t.Setenv("IDENTITY_CACHE_LOCAL_TTL_SECONDS", "11")
	t.Setenv("IDENTITY_CACHE_LOCAL_MAX_ENTRIES", "1234")

	cfg := config.Load()

	if cfg.DB.Host != "db.example" || cfg.DB.Port != "6543" || cfg.DB.SSLMode != "require" {
		t.Errorf("DB overrides wrong: %+v", cfg.DB)
	}
	if cfg.DB.ConnMaxIdleSeconds != 45 || cfg.DB.ConnMaxLifetimeMinutes != 20 {
		t.Errorf("DB pool-timeout overrides wrong: idle=%d lifetime=%d (want 45/20)",
			cfg.DB.ConnMaxIdleSeconds, cfg.DB.ConnMaxLifetimeMinutes)
	}
	if cfg.Redis.URL != "rediss://default:secret@redis.example:6380/5" {
		t.Errorf("REDIS_URL override wrong: %q", cfg.Redis.URL)
	}
	if cfg.Cooldown.Jitter != 0.5 || cfg.Cooldown.BaseSeconds != 10 || cfg.Cooldown.MaxSeconds != 100 {
		t.Errorf("Cooldown overrides wrong: %+v", cfg.Cooldown)
	}
	if cfg.LLM.Provider != "openai" || cfg.LLM.YearsExp != 4 {
		t.Errorf("LLM overrides wrong: %+v", cfg.LLM)
	}
	if len(cfg.LLM.TargetRoles) != 3 || cfg.LLM.TargetRoles[0] != "SRE" || cfg.LLM.TargetRoles[1] != "Platform" || cfg.LLM.TargetRoles[2] != "Data" {
		t.Errorf("TARGET_ROLES split wrong: %+v", cfg.LLM.TargetRoles)
	}
	if cfg.Filter.BatchSize != 12 || cfg.Filter.FlushTimeoutSeconds != 13 || cfg.Filter.LLMCallIntervalSeconds != 14 {
		t.Errorf("Filter overrides wrong: %+v", cfg.Filter)
	}
	if cfg.Enricher.DefaultRPS != 1.5 || cfg.Enricher.DefaultBurst != 6 || cfg.Enricher.MaxExpYears != 8 {
		t.Errorf("Enricher overrides wrong: %+v", cfg.Enricher)
	}
	if cfg.Crawler.PollIntervalSeconds != 44 || cfg.Crawler.Google.MaxPages != 100 {
		t.Errorf("Crawler overrides wrong: %+v", cfg.Crawler)
	}
	if cfg.Scheduler.ScanIntervalMinutes != 111 || cfg.Scheduler.CheckIntervalSeconds != 33 {
		t.Errorf("Scheduler overrides wrong: %+v", cfg.Scheduler)
	}
	if cfg.Auth.ClerkJWKSURL != "https://x.clerk.accounts.dev/.well-known/jwks.json" {
		t.Errorf("Auth JWKS URL override wrong: %+v", cfg.Auth)
	}
	if cfg.Auth.ClerkIssuerURL != "https://x.clerk.accounts.dev" {
		t.Errorf("Auth issuer URL override wrong: %+v", cfg.Auth)
	}
	if cfg.API.MaxBodyBytes != 2000000 || cfg.API.DevToken != "devtoken" {
		t.Errorf("API overrides wrong: %+v", cfg.API)
	}
	if cfg.MultiUser.ClerkSecretKey != "sk_test_abc123" {
		t.Errorf("MultiUser ClerkSecretKey override wrong: %+v", cfg.MultiUser)
	}
	if cfg.MultiUser.RateLimitReadRPM != 31 || cfg.MultiUser.RateLimitWriteRPM != 32 ||
		cfg.MultiUser.RateLimitAdminRPM != 33 || cfg.MultiUser.CacheTTLUserSeconds != 61 || cfg.MultiUser.CacheTTLCompanySeconds != 62 ||
		cfg.MultiUser.IdentityCacheLocalTTLSeconds != 11 || cfg.MultiUser.IdentityCacheLocalMaxEntries != 1234 {
		t.Errorf("MultiUser overrides wrong: %+v", cfg.MultiUser)
	}
}

func TestLoad_InvalidIntFallsBackToDefault(t *testing.T) {
	clearConfigEnv(t)
	t.Setenv("FILTER_BATCH_SIZE", "abc")
	t.Setenv("LLM_COOLDOWN_JITTER", "not-a-float")

	cfg := config.Load()

	if cfg.Filter.BatchSize != 50 {
		t.Errorf("invalid FILTER_BATCH_SIZE should fall back to default 50, got %d", cfg.Filter.BatchSize)
	}
	if cfg.Cooldown.Jitter != 0.2 {
		t.Errorf("invalid LLM_COOLDOWN_JITTER should fall back to default 0.2, got %v", cfg.Cooldown.Jitter)
	}
}

func TestLoad_ClerkURLsFromEnv(t *testing.T) {
	clearConfigEnv(t)
	t.Setenv("CLERK_JWKS_URL", "https://example.clerk.accounts.dev/.well-known/jwks.json")
	t.Setenv("CLERK_ISSUER_URL", "https://example.clerk.accounts.dev")
	cfg := config.Load()
	if cfg.Auth.ClerkJWKSURL != "https://example.clerk.accounts.dev/.well-known/jwks.json" {
		t.Errorf("CLERK_JWKS_URL not loaded: %q", cfg.Auth.ClerkJWKSURL)
	}
	if cfg.Auth.ClerkIssuerURL != "https://example.clerk.accounts.dev" {
		t.Errorf("CLERK_ISSUER_URL not loaded: %q", cfg.Auth.ClerkIssuerURL)
	}
}

func TestLoad_TargetRolesEmptyProducesEmptySlice(t *testing.T) {
	clearConfigEnv(t)
	t.Setenv("TARGET_ROLES", " , ,")
	cfg := config.Load()
	if len(cfg.LLM.TargetRoles) != 0 {
		t.Errorf("expected empty TargetRoles, got %+v", cfg.LLM.TargetRoles)
	}
}

func TestLoad_FilterAndEnricherSlotsFromJSONEnv(t *testing.T) {
	clearConfigEnv(t)
	filter := []config.LLMSlotConfig{{KeyID: "f1", APIKey: "k", Model: "m", RPM: 1}}
	enricher := []config.LLMSlotConfig{{KeyID: "e1", APIKey: "k2", Model: "m2", RPM: 2}}
	fJSON, _ := json.Marshal(filter)
	eJSON, _ := json.Marshal(enricher)
	t.Setenv("FILTER_LLM_SLOTS", string(fJSON))
	t.Setenv("ENRICHER_LLM_SLOTS", string(eJSON))

	cfg := config.Load()
	if len(cfg.Filter.Slots) != 1 || cfg.Filter.Slots[0].KeyID != "f1" {
		t.Errorf("filter slots not picked up: %+v", cfg.Filter.Slots)
	}
	if len(cfg.Enricher.Slots) != 1 || cfg.Enricher.Slots[0].KeyID != "e1" {
		t.Errorf("enricher slots not picked up: %+v", cfg.Enricher.Slots)
	}
}

func TestLoad_YAMLSlotsPreferredOverJSONEnv(t *testing.T) {
	clearConfigEnv(t)
	t.Setenv("GEMINI_KEY_X", "sk-yaml")

	yaml := `filter:
  slots:
    - key_id: yaml-f
      api_key: ${GEMINI_KEY_X}
      model: yaml-model
      rpm: 1
      rpd: 2
      tpm_in: 3
enricher:
  slots:
    - key_id: yaml-e
      api_key: ${GEMINI_KEY_X}
      model: yaml-enr-model
      rpm: 4
      rpd: 5
      tpm_in: 6
`
	path := filepath.Join(t.TempDir(), "slots.yaml")
	if err := os.WriteFile(path, []byte(yaml), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("LLM_SLOTS_CONFIG", path)

	// JSON env vars also set — YAML should win.
	t.Setenv("FILTER_LLM_SLOTS", `[{"key_id":"json-f","api_key":"x","model":"m","rpm":1,"rpd":1,"tpm_in":1}]`)
	t.Setenv("ENRICHER_LLM_SLOTS", `[{"key_id":"json-e","api_key":"x","model":"m","rpm":1,"rpd":1,"tpm_in":1}]`)

	cfg := config.Load()
	if len(cfg.Filter.Slots) != 1 || cfg.Filter.Slots[0].KeyID != "yaml-f" || cfg.Filter.Slots[0].APIKey != "sk-yaml" {
		t.Errorf("YAML filter slot should win: %+v", cfg.Filter.Slots)
	}
	if len(cfg.Enricher.Slots) != 1 || cfg.Enricher.Slots[0].KeyID != "yaml-e" {
		t.Errorf("YAML enricher slot should win: %+v", cfg.Enricher.Slots)
	}
}

func TestLoad_MalformedYAMLFallsBackToJSONEnv(t *testing.T) {
	clearConfigEnv(t)
	bad := "filter:\n  slots: [: not yaml"
	path := filepath.Join(t.TempDir(), "bad.yaml")
	if err := os.WriteFile(path, []byte(bad), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("LLM_SLOTS_CONFIG", path)
	t.Setenv("FILTER_LLM_SLOTS", `[{"key_id":"json-f","api_key":"x","model":"m","rpm":1,"rpd":1,"tpm_in":1}]`)
	t.Setenv("ENRICHER_LLM_SLOTS", `[{"key_id":"json-e","api_key":"x","model":"m","rpm":1,"rpd":1,"tpm_in":1}]`)

	cfg := config.Load()
	if len(cfg.Filter.Slots) != 1 || cfg.Filter.Slots[0].KeyID != "json-f" {
		t.Errorf("expected JSON fallback to win when YAML malformed, got %+v", cfg.Filter.Slots)
	}
	if len(cfg.Enricher.Slots) != 1 || cfg.Enricher.Slots[0].KeyID != "json-e" {
		t.Errorf("expected JSON fallback to win when YAML malformed, got %+v", cfg.Enricher.Slots)
	}
}

func TestLoad_MalformedJSONLeavesSlotsNil(t *testing.T) {
	clearConfigEnv(t)
	t.Setenv("FILTER_LLM_SLOTS", "not-json")
	t.Setenv("ENRICHER_LLM_SLOTS", "also-not-json")
	cfg := config.Load()
	if cfg.Filter.Slots != nil {
		t.Errorf("expected nil filter slots on bad JSON, got %+v", cfg.Filter.Slots)
	}
	if cfg.Enricher.Slots != nil {
		t.Errorf("expected nil enricher slots on bad JSON, got %+v", cfg.Enricher.Slots)
	}
}
