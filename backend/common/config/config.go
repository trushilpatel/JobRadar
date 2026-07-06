package config

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/joho/godotenv"
)

type Config struct {
	DB            DatabaseConfig
	NATS          NATSConfig
	Redis         RedisConfig
	Cooldown      CooldownConfig
	LLM           LLMConfig
	Filter        FilterConfig
	Enricher      EnricherConfig
	Crawler       CrawlerConfig
	Scheduler     SchedulerConfig
	Observability ObservabilityConfig
	Auth          AuthConfig
	API           APIConfig
	MultiUser     MultiUserConfig
	ObjectStorage ObjectStorageConfig
}

// MultiUserConfig holds multi-user knobs: Clerk admin client credentials,
// per-scope rate limits, and identity cache TTL. All values have sensible
// defaults so a stock dev environment boots without needing to set them.
type MultiUserConfig struct {
	ClerkSecretKey         string
	RateLimitReadRPM       int
	RateLimitWriteRPM      int
	RateLimitAdminRPM      int
	CacheTTLUserSeconds    int
	CacheTTLCompanySeconds int
	// IdentityCacheLocalTTLSeconds is the upper bound on how long the API
	// process keeps an identity bundle (or any other key) in its in-process
	// LRU before re-reading Redis. Set to 0 to disable the local layer.
	IdentityCacheLocalTTLSeconds int
	// IdentityCacheLocalMaxEntries caps the number of keys the local layer
	// holds. Tune up if the API serves many distinct users per minute.
	IdentityCacheLocalMaxEntries int
}

// AuthConfig holds Clerk auth verification settings. Both CLERK_JWKS_URL and
// CLERK_ISSUER_URL must be set for the API to start.
type AuthConfig struct {
	ClerkJWKSURL   string
	ClerkIssuerURL string
}

// Validate enforces boot-time auth invariants. Both Clerk JWKS and issuer
// URLs must be set.
func (c AuthConfig) Validate() error {
	if c.ClerkJWKSURL == "" {
		return fmt.Errorf("config: CLERK_JWKS_URL must be set")
	}
	if c.ClerkIssuerURL == "" {
		return fmt.Errorf("config: CLERK_ISSUER_URL must be set")
	}
	return nil
}

// APIConfig holds HTTP-server-level settings for the public API listener.
type APIConfig struct {
	MaxBodyBytes int64
	DevToken     string
}

// ObjectStorageConfig holds connection details for the S3-compatible object
// storage bucket (OCI Object Storage in production). All fields are required
// in production but left empty-safe here so tests that don't touch storage
// don't need to set them.
type ObjectStorageConfig struct {
	Endpoint  string
	Region    string
	Bucket    string
	AccessKey string
	SecretKey string
}

type DatabaseConfig struct {
	Host     string
	Port     string
	User     string
	Password string
	Name     string
	SSLMode  string
	// ConnMaxIdleSeconds bounds how long an idle pooled connection is kept
	// before being closed. It must stay below the upstream pooler's own idle
	// timeout (Supabase Supavisor) so the client never hands out a connection
	// the pooler has already dropped — the root cause of mid-flight
	// "unexpected EOF" errors. Sourced from DB_CONN_MAX_IDLE_SECONDS.
	ConnMaxIdleSeconds int
	// ConnMaxLifetimeMinutes bounds the total lifetime of a pooled connection.
	// Sourced from DB_CONN_MAX_LIFETIME_MINUTES.
	ConnMaxLifetimeMinutes int
}

func (c DatabaseConfig) DSN() string {
	return fmt.Sprintf(
		"host=%s port=%s user=%s password=%s dbname=%s sslmode=%s statement_cache_capacity=0 default_query_exec_mode=simple_protocol",
		c.Host, c.Port, c.User, c.Password, c.Name, c.SSLMode,
	)
}

// Validate enforces boot-time database pool invariants. Both timeouts must be
// positive and the idle timeout must not exceed the connection lifetime (an
// idle window longer than the lifetime is meaningless — the connection is
// recycled first). Kept as a method so worker main() functions that already
// validate config can call it before Open.
func (c DatabaseConfig) Validate() error {
	if c.ConnMaxIdleSeconds <= 0 {
		return fmt.Errorf("config: DB_CONN_MAX_IDLE_SECONDS must be > 0, got %d", c.ConnMaxIdleSeconds)
	}
	if c.ConnMaxLifetimeMinutes <= 0 {
		return fmt.Errorf("config: DB_CONN_MAX_LIFETIME_MINUTES must be > 0, got %d", c.ConnMaxLifetimeMinutes)
	}
	if c.ConnMaxIdleSeconds > c.ConnMaxLifetimeMinutes*60 {
		return fmt.Errorf(
			"config: DB_CONN_MAX_IDLE_SECONDS (%d) must be <= DB_CONN_MAX_LIFETIME_MINUTES*60 (%d)",
			c.ConnMaxIdleSeconds, c.ConnMaxLifetimeMinutes*60)
	}
	return nil
}

type NATSConfig struct {
	URL       string
	CredsFile string
}

type LLMConfig struct {
	Provider    string
	TargetRoles []string
	YearsExp    int
}

type FilterConfig struct {
	BatchSize              int
	FlushTimeoutSeconds    int
	LLMCallIntervalSeconds int
	Slots                  []LLMSlotConfig
}

type EnricherConfig struct {
	FetchConcurrency       int
	LLMBatchSize           int
	FlushTimeoutSeconds    int
	LLMCallIntervalSeconds int
	DefaultRPS             float64
	DefaultBurst           int
	JitterMs               int
	MaxExpYears            int
	Slots                  []LLMSlotConfig
}

type CrawlerConfig struct {
	CompanyDelaySeconds      int
	PollIntervalSeconds      int
	StuckTimeoutSeconds      int
	HeartbeatIntervalSeconds int
	// CrawlerTokenSecret is the HMAC-SHA256 signing key used by the orchestrator
	// to verify bearer tokens from distributed crawler nodes. Required when the
	// orchestrator is running.
	CrawlerTokenSecret string
	Google             GoogleScraperConfig
}

type GoogleScraperConfig struct {
	MinDelayMs int
	MaxDelayMs int
	TimeoutMs  int
	MaxPages   int
}

type SchedulerConfig struct {
	ScanIntervalMinutes  int
	StuckTimeoutMinutes  int
	CheckIntervalSeconds int
	// SweeperMaxRetries is the maximum number of times the sweeper will
	// re-publish a retry job before escalating to reason='error'.
	// Defaults to 10. Configurable via SWEEPER_MAX_RETRIES.
	SweeperMaxRetries int
}

// Validate returns an error if any required scheduler config values are invalid.
func (c SchedulerConfig) Validate() error {
	if c.ScanIntervalMinutes <= 0 {
		return fmt.Errorf("config: SCAN_INTERVAL_MINUTES must be > 0, got %d", c.ScanIntervalMinutes)
	}
	if c.StuckTimeoutMinutes <= 0 {
		return fmt.Errorf("config: SCHEDULER_STUCK_TIMEOUT_MINUTES must be > 0, got %d", c.StuckTimeoutMinutes)
	}
	if c.StuckTimeoutMinutes > 1440 {
		return fmt.Errorf("config: SCHEDULER_STUCK_TIMEOUT_MINUTES must be <= 1440 (24h), got %d", c.StuckTimeoutMinutes)
	}
	if c.CheckIntervalSeconds <= 0 {
		return fmt.Errorf("config: SCHEDULER_CHECK_INTERVAL_SECONDS must be > 0, got %d", c.CheckIntervalSeconds)
	}
	return nil
}

type ObservabilityConfig struct {
	MetricsAddr  string
	OTelEndpoint string
}

type RedisConfig struct {
	URL string
}

type CooldownConfig struct {
	BaseSeconds int
	MaxSeconds  int
	Jitter      float64 // 0.0 – 1.0
}

// LLMSlotConfig describes a single (API key, model) slot for routed LLM calls.
type LLMSlotConfig struct {
	KeyID  string `json:"key_id"  yaml:"key_id"`
	APIKey string `json:"api_key" yaml:"api_key"`
	Model  string `json:"model"   yaml:"model"`
	RPM    int    `json:"rpm"     yaml:"rpm"`
	RPD    int    `json:"rpd"     yaml:"rpd"`
	TPMIn  int    `json:"tpm_in"  yaml:"tpm_in"`
}

// ParseLLMSlots parses a JSON array of slot configs from a single env var.
// Returns nil on empty input (not an error — enables legacy single-slot mode).
func ParseLLMSlots(raw string) ([]LLMSlotConfig, error) {
	if raw == "" {
		return nil, nil
	}
	var slots []LLMSlotConfig
	if err := json.Unmarshal([]byte(raw), &slots); err != nil {
		return nil, fmt.Errorf("config: parse LLM slots: %w", err)
	}
	return slots, nil
}

func Load() *Config {
	loaded := false
	for _, f := range []string{".env", "../.env"} {
		if err := godotenv.Load(f); err == nil {
			loaded = true
			break
		}
	}
	if !loaded {
		fmt.Fprintf(os.Stderr, "config: no .env file found, falling back to environment variables\n")
	}

	// Prefer YAML when LLM_SLOTS_CONFIG points at a file; otherwise fall back
	// to the JSON env vars (FILTER_LLM_SLOTS / ENRICHER_LLM_SLOTS).
	filterSlots, enricherSlots, err := LoadLLMSlotsYAML(getEnv("LLM_SLOTS_CONFIG", ""))
	if err != nil {
		fmt.Fprintf(os.Stderr, "config: %v — falling back to env JSON\n", err)
	}
	if filterSlots == nil {
		filterSlots, err = ParseLLMSlots(getEnv("FILTER_LLM_SLOTS", ""))
		if err != nil {
			fmt.Fprintf(os.Stderr, "config: %v — falling back to single slot\n", err)
		}
	}
	if enricherSlots == nil {
		enricherSlots, err = ParseLLMSlots(getEnv("ENRICHER_LLM_SLOTS", ""))
		if err != nil {
			fmt.Fprintf(os.Stderr, "config: %v — falling back to single slot\n", err)
		}
	}

	return &Config{
		DB: DatabaseConfig{
			Host:     getEnv("DB_HOST", "localhost"),
			Port:     getEnv("DB_PORT", "5432"),
			User:     getEnv("DB_USER", "postgres"),
			Password: getEnv("DB_PASSWORD", "postgres"),
			Name:     getEnv("DB_NAME", "jobradar"),
			SSLMode:  getEnv("DB_SSLMODE", "disable"),
			// Defaults tuned below Supabase Supavisor's idle timeout so the
			// client never reuses a connection the pooler already closed.
			ConnMaxIdleSeconds:     getEnvInt("DB_CONN_MAX_IDLE_SECONDS", 30),
			ConnMaxLifetimeMinutes: getEnvInt("DB_CONN_MAX_LIFETIME_MINUTES", 15),
		},
		NATS: NATSConfig{
			URL:       getEnv("NATS_URL", "nats://localhost:4222"),
			CredsFile: getEnv("NATS_CREDS_FILE", ""),
		},
		Redis: RedisConfig{
			URL: getEnv("REDIS_URL", "redis://localhost:6379/0"),
		},
		Cooldown: CooldownConfig{
			BaseSeconds: getEnvInt("LLM_COOLDOWN_BASE_SECONDS", 60),
			MaxSeconds:  getEnvInt("LLM_COOLDOWN_MAX_SECONDS", 3600),
			Jitter:      getEnvFloat("LLM_COOLDOWN_JITTER", 0.2),
		},
		LLM: LLMConfig{
			Provider:    getEnv("LLM_PROVIDER", "gemini"),
			TargetRoles: splitCSV(getEnv("TARGET_ROLES", "Software Engineer")),
			YearsExp:    getEnvInt("YEARS_EXP", 0),
		},
		Filter: FilterConfig{
			BatchSize:              getEnvInt("FILTER_BATCH_SIZE", 50),
			FlushTimeoutSeconds:    getEnvInt("FILTER_FLUSH_TIMEOUT_SECONDS", 30),
			LLMCallIntervalSeconds: getEnvInt("FILTER_LLM_CALL_INTERVAL_SECONDS", 5),
			Slots:                  filterSlots,
		},
		Enricher: EnricherConfig{
			FetchConcurrency:       getEnvInt("ENRICHER_FETCH_CONCURRENCY", 15),
			LLMBatchSize:           getEnvInt("ENRICHER_LLM_BATCH_SIZE", 5),
			FlushTimeoutSeconds:    getEnvInt("ENRICHER_FLUSH_TIMEOUT_SECONDS", 120),
			LLMCallIntervalSeconds: getEnvInt("ENRICHER_LLM_CALL_INTERVAL_SECONDS", 5),
			DefaultRPS:             getEnvFloat("ENRICHER_DEFAULT_RPS", 2),
			DefaultBurst:           getEnvInt("ENRICHER_DEFAULT_BURST", 3),
			JitterMs:               getEnvInt("ENRICHER_JITTER_MS", 200),
			MaxExpYears:            getEnvInt("DISCARD_JOB_WITH_EXP_GT", 0),
			Slots:                  enricherSlots,
		},
		Crawler: CrawlerConfig{
			CompanyDelaySeconds:      getEnvInt("SCAN_COMPANY_DELAY_SECONDS", 30),
			PollIntervalSeconds:      getEnvInt("CRAWLER_POLL_INTERVAL_SECONDS", 60),
			StuckTimeoutSeconds:      getEnvInt("CRAWLER_STUCK_TIMEOUT_SECONDS", 120),
			HeartbeatIntervalSeconds: getEnvInt("CRAWLER_HEARTBEAT_INTERVAL_SECONDS", 30),
			CrawlerTokenSecret:       os.Getenv("CRAWLER_TOKEN_SECRET"),
			Google: GoogleScraperConfig{
				// Listing pacing for the HTTP-based Google scraper.
				MinDelayMs: getEnvInt("GOOGLE_SCRAPER_MIN_DELAY", 20000),
				MaxDelayMs: getEnvInt("GOOGLE_SCRAPER_MAX_DELAY", 40000),
				TimeoutMs:  getEnvInt("GOOGLE_SCRAPER_TIMEOUT_MS", 7200000),
				MaxPages:   getEnvInt("GOOGLE_SCRAPER_MAX_PAGES", 0),
			},
		},
		Scheduler: SchedulerConfig{
			ScanIntervalMinutes:  getEnvInt("SCAN_INTERVAL_MINUTES", 1440),
			StuckTimeoutMinutes:  getEnvInt("SCHEDULER_STUCK_TIMEOUT_MINUTES", 30),
			CheckIntervalSeconds: getEnvInt("SCHEDULER_CHECK_INTERVAL_SECONDS", 60),
			SweeperMaxRetries:    getEnvInt("SWEEPER_MAX_RETRIES", 10),
		},
		Observability: ObservabilityConfig{
			MetricsAddr:  getEnv("METRICS_ADDR", "127.0.0.1:9100"),
			OTelEndpoint: getEnv("OTEL_EXPORTER_OTLP_ENDPOINT", "http://localhost:4318"),
		},
		Auth: AuthConfig{
			ClerkJWKSURL:   getEnv("CLERK_JWKS_URL", ""),
			ClerkIssuerURL: getEnv("CLERK_ISSUER_URL", ""),
		},
		API: APIConfig{
			// 3MB covers the 2MB resume-upload cap plus multipart overhead
			// (boundary, headers, label field). Global because BodyLimit wraps
			// the whole /api tree — no other endpoint sends bodies anywhere
			// near this size, so raising the ceiling here is safe.
			MaxBodyBytes: int64(getEnvInt("API_MAX_BODY_BYTES", 3145728)),
			DevToken:     os.Getenv("DEV_TOKEN"),
		},
		MultiUser: MultiUserConfig{
			ClerkSecretKey:               getEnv("CLERK_SECRET_KEY", ""),
			RateLimitReadRPM:             getEnvInt("RATE_LIMIT_READ_RPM", 30),
			RateLimitWriteRPM:            getEnvInt("RATE_LIMIT_WRITE_RPM", 20),
			RateLimitAdminRPM:            getEnvInt("RATE_LIMIT_ADMIN_RPM", 120),
			CacheTTLUserSeconds:          getEnvInt("CACHE_TTL_USER_SECONDS", 60),
			CacheTTLCompanySeconds:       getEnvInt("CACHE_TTL_COMPANY_SECONDS", 60),
			IdentityCacheLocalTTLSeconds: getEnvInt("IDENTITY_CACHE_LOCAL_TTL_SECONDS", 30),
			IdentityCacheLocalMaxEntries: getEnvInt("IDENTITY_CACHE_LOCAL_MAX_ENTRIES", 10000),
		},
		ObjectStorage: ObjectStorageConfig{
			Endpoint:  getEnv("OCI_S3_ENDPOINT", ""),
			Region:    getEnv("OCI_S3_REGION", ""),
			Bucket:    getEnv("OCI_S3_BUCKET", ""),
			AccessKey: getEnv("OCI_S3_ACCESS_KEY", ""),
			SecretKey: getEnv("OCI_S3_SECRET_KEY", ""),
		},
	}
}

func getEnv(key, fallback string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return fallback
}

func getEnvInt(key string, fallback int) int {
	if val := os.Getenv(key); val != "" {
		if n, err := strconv.Atoi(val); err == nil {
			return n
		}
	}
	return fallback
}

func getEnvFloat(key string, fallback float64) float64 {
	if val := os.Getenv(key); val != "" {
		if f, err := strconv.ParseFloat(val, 64); err == nil {
			return f
		}
	}
	return fallback
}

func splitCSV(s string) []string {
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if t := strings.TrimSpace(p); t != "" {
			out = append(out, t)
		}
	}
	return out
}
