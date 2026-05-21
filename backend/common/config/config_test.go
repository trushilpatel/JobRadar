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

// ---------- AuthConfig.Validate ----------

func TestAuthConfig_Validate(t *testing.T) {
	t.Run("both secret and JWKS set", func(t *testing.T) {
		c := config.AuthConfig{SupabaseProjectURL: "x", SupabaseJWTSecret: "s", SupabaseJWKSURL: "u", JWKSCacheTTLSeconds: 1}
		if err := c.Validate(); err == nil || !strings.Contains(err.Error(), "only one") {
			t.Fatalf("want both-set error, got %v", err)
		}
	})
	t.Run("neither set", func(t *testing.T) {
		c := config.AuthConfig{SupabaseProjectURL: "x", JWKSCacheTTLSeconds: 1}
		if err := c.Validate(); err == nil || !strings.Contains(err.Error(), "exactly one") {
			t.Fatalf("want neither-set error, got %v", err)
		}
	})
	t.Run("missing project URL", func(t *testing.T) {
		c := config.AuthConfig{SupabaseJWTSecret: "s", JWKSCacheTTLSeconds: 1}
		if err := c.Validate(); err == nil || !strings.Contains(err.Error(), "SUPABASE_PROJECT_URL") {
			t.Fatalf("want project-url error, got %v", err)
		}
	})
	t.Run("negative clock skew", func(t *testing.T) {
		c := config.AuthConfig{SupabaseProjectURL: "x", SupabaseJWTSecret: "s", ClockSkewSeconds: -1, JWKSCacheTTLSeconds: 1}
		if err := c.Validate(); err == nil || !strings.Contains(err.Error(), "JWT_CLOCK_SKEW_SECONDS") {
			t.Fatalf("want clock-skew error, got %v", err)
		}
	})
	t.Run("zero JWKS cache TTL", func(t *testing.T) {
		c := config.AuthConfig{SupabaseProjectURL: "x", SupabaseJWTSecret: "s", JWKSCacheTTLSeconds: 0}
		if err := c.Validate(); err == nil || !strings.Contains(err.Error(), "JWKS_CACHE_TTL_SECONDS") {
			t.Fatalf("want jwks-ttl error, got %v", err)
		}
	})
	t.Run("happy path with JWT secret", func(t *testing.T) {
		c := config.AuthConfig{SupabaseProjectURL: "x", SupabaseJWTSecret: "s", JWKSCacheTTLSeconds: 1}
		if err := c.Validate(); err != nil {
			t.Fatalf("happy path returned error: %v", err)
		}
	})
	t.Run("happy path with JWKS URL", func(t *testing.T) {
		c := config.AuthConfig{SupabaseProjectURL: "x", SupabaseJWKSURL: "u", ClockSkewSeconds: 0, JWKSCacheTTLSeconds: 1}
		if err := c.Validate(); err != nil {
			t.Fatalf("happy path returned error: %v", err)
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
		"METRICS_ADDR", "JAEGER_ENDPOINT",
		"SUPABASE_PROJECT_URL", "SUPABASE_JWKS_URL", "SUPABASE_JWT_SECRET",
		"JWT_CLOCK_SKEW_SECONDS", "JWKS_CACHE_TTL_SECONDS",
		"API_MAX_BODY_BYTES", "DEV_TOKEN",
		"SUPABASE_URL", "SUPABASE_SERVICE_ROLE_KEY",
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
	if cfg.Kafka.Brokers != "localhost:9092" || cfg.Kafka.User != "" || cfg.Kafka.Password != "" {
		t.Errorf("Kafka defaults wrong: %+v", cfg.Kafka)
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
	if cfg.Observability.MetricsAddr != "127.0.0.1:9100" || cfg.Observability.JaegerEndpoint != "http://localhost:4318" {
		t.Errorf("Observability defaults wrong: %+v", cfg.Observability)
	}
	if cfg.Auth.ClockSkewSeconds != 60 || cfg.Auth.JWKSCacheTTLSeconds != 600 {
		t.Errorf("Auth defaults wrong: %+v", cfg.Auth)
	}
	if cfg.API.MaxBodyBytes != 1048576 || cfg.API.DevToken != "" {
		t.Errorf("API defaults wrong: %+v", cfg.API)
	}
	if cfg.MultiUser.RateLimitReadRPM != 30 || cfg.MultiUser.RateLimitWriteRPM != 20 ||
		cfg.MultiUser.RateLimitAdminRPM != 120 || cfg.MultiUser.CacheTTLUserSeconds != 60 ||
		cfg.MultiUser.CacheTTLCompanySeconds != 60 {
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
	t.Setenv("KAFKA_BROKERS", "kafka:1")
	t.Setenv("KAFKA_USER", "kuser")
	t.Setenv("KAFKA_PASSWORD", "kpw")
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
	t.Setenv("JAEGER_ENDPOINT", "http://otel:4318")
	t.Setenv("SUPABASE_PROJECT_URL", "https://x.supabase.co")
	t.Setenv("SUPABASE_JWT_SECRET", "secret")
	t.Setenv("JWT_CLOCK_SKEW_SECONDS", "30")
	t.Setenv("JWKS_CACHE_TTL_SECONDS", "120")
	t.Setenv("API_MAX_BODY_BYTES", "2000000")
	t.Setenv("DEV_TOKEN", "devtoken")
	t.Setenv("SUPABASE_URL", "https://x.supabase.co")
	t.Setenv("SUPABASE_SERVICE_ROLE_KEY", "srk")
	t.Setenv("RATE_LIMIT_READ_RPM", "31")
	t.Setenv("RATE_LIMIT_WRITE_RPM", "32")
	t.Setenv("RATE_LIMIT_ADMIN_RPM", "33")
	t.Setenv("CACHE_TTL_USER_SECONDS", "61")
	t.Setenv("CACHE_TTL_COMPANY_SECONDS", "62")

	cfg := config.Load()

	if cfg.DB.Host != "db.example" || cfg.DB.Port != "6543" || cfg.DB.SSLMode != "require" {
		t.Errorf("DB overrides wrong: %+v", cfg.DB)
	}
	if cfg.Kafka.User != "kuser" || cfg.Kafka.Password != "kpw" {
		t.Errorf("Kafka overrides wrong: %+v", cfg.Kafka)
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
	if cfg.Auth.SupabaseProjectURL != "https://x.supabase.co" || cfg.Auth.SupabaseJWTSecret != "secret" {
		t.Errorf("Auth overrides wrong: %+v", cfg.Auth)
	}
	// JWT secret set → derived JWKS URL stays empty.
	if cfg.Auth.SupabaseJWKSURL != "" {
		t.Errorf("expected JWKS URL empty when JWT secret set, got %q", cfg.Auth.SupabaseJWKSURL)
	}
	if cfg.Auth.ClockSkewSeconds != 30 || cfg.Auth.JWKSCacheTTLSeconds != 120 {
		t.Errorf("Auth numeric overrides wrong: %+v", cfg.Auth)
	}
	if cfg.API.MaxBodyBytes != 2000000 || cfg.API.DevToken != "devtoken" {
		t.Errorf("API overrides wrong: %+v", cfg.API)
	}
	if cfg.MultiUser.RateLimitReadRPM != 31 || cfg.MultiUser.RateLimitWriteRPM != 32 ||
		cfg.MultiUser.RateLimitAdminRPM != 33 || cfg.MultiUser.CacheTTLUserSeconds != 61 || cfg.MultiUser.CacheTTLCompanySeconds != 62 {
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

func TestLoad_DerivesJWKSURLFromProjectURL(t *testing.T) {
	clearConfigEnv(t)
	// Neither JWT secret nor explicit JWKS URL → derive from project URL.
	t.Setenv("SUPABASE_PROJECT_URL", "https://x.supabase.co/")
	cfg := config.Load()
	want := "https://x.supabase.co/auth/v1/.well-known/jwks.json"
	if cfg.Auth.SupabaseJWKSURL != want {
		t.Errorf("derived JWKS URL wrong: got %q want %q", cfg.Auth.SupabaseJWKSURL, want)
	}
}

func TestLoad_ExplicitJWKSURLWinsOverProjectURL(t *testing.T) {
	clearConfigEnv(t)
	t.Setenv("SUPABASE_PROJECT_URL", "https://x.supabase.co")
	t.Setenv("SUPABASE_JWKS_URL", "https://override.example/jwks.json")
	cfg := config.Load()
	if cfg.Auth.SupabaseJWKSURL != "https://override.example/jwks.json" {
		t.Errorf("explicit JWKS_URL did not win: %q", cfg.Auth.SupabaseJWKSURL)
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
