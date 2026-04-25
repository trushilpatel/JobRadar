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
	Kafka         KafkaConfig
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
}

// AuthConfig holds Supabase auth verification settings. Exactly one of
// SupabaseJWTSecret (HS256) or SupabaseJWKSURL (ES256 via JWKS) must be set.
type AuthConfig struct {
	SupabaseProjectURL  string
	SupabaseJWKSURL     string
	SupabaseJWTSecret   string
	AllowedEmail        string
	ClockSkewSeconds    int
	JWKSCacheTTLSeconds int
}

// Validate enforces boot-time auth invariants. Refuses to start if both JWT
// secret and JWKS URL are set, or if neither is set, or if AllowedEmail is
// empty.
func (c AuthConfig) Validate() error {
	if c.AllowedEmail == "" {
		return fmt.Errorf("config: ALLOWED_EMAIL must be set")
	}
	hasSecret := c.SupabaseJWTSecret != ""
	hasJWKS := c.SupabaseJWKSURL != ""
	if hasSecret && hasJWKS {
		return fmt.Errorf("config: only one of SUPABASE_JWT_SECRET or SUPABASE_JWKS_URL may be set, not both")
	}
	if !hasSecret && !hasJWKS {
		return fmt.Errorf("config: exactly one of SUPABASE_JWT_SECRET or SUPABASE_JWKS_URL must be set")
	}
	if c.SupabaseProjectURL == "" {
		return fmt.Errorf("config: SUPABASE_PROJECT_URL must be set")
	}
	if c.ClockSkewSeconds < 0 {
		return fmt.Errorf("config: JWT_CLOCK_SKEW_SECONDS must be >= 0, got %d", c.ClockSkewSeconds)
	}
	if c.JWKSCacheTTLSeconds <= 0 {
		return fmt.Errorf("config: JWKS_CACHE_TTL_SECONDS must be > 0, got %d", c.JWKSCacheTTLSeconds)
	}
	return nil
}

// APIConfig holds HTTP-server-level settings for the public API listener.
type APIConfig struct {
	MaxBodyBytes int64
	DevToken     string
}

type DatabaseConfig struct {
	Host     string
	Port     string
	User     string
	Password string
	Name     string
	SSLMode  string
}

func (c DatabaseConfig) DSN() string {
	return fmt.Sprintf(
		"host=%s port=%s user=%s password=%s dbname=%s sslmode=%s",
		c.Host, c.Port, c.User, c.Password, c.Name, c.SSLMode,
	)
}

type KafkaConfig struct {
	Brokers  string
	User     string
	Password string
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
	Google                   GoogleScraperConfig
}

type GoogleScraperConfig struct {
	MinDelayMs    int
	MaxDelayMs    int
	TimeoutMs     int
	MaxPages      int
	EnrichMinMs   int
	EnrichMaxMs   int
	MaxDetailJobs int
	// ChromiumPath is used by the shared browser launcher for the (still
	// rod-based) Meta adapter. The Google adapter itself is HTTP-only and
	// does not consume this field.
	ChromiumPath string
}

type SchedulerConfig struct {
	ScanIntervalMinutes  int
	StuckTimeoutMinutes  int
	CheckIntervalSeconds int
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
	MetricsAddr    string
	JaegerEndpoint string
}

type RedisConfig struct {
	Addr string
	DB   int
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
	if err := godotenv.Load(); err != nil {
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
		},
		Kafka: KafkaConfig{
			Brokers:  getEnv("KAFKA_BROKERS", "localhost:9092"),
			User:     getEnv("KAFKA_USER", ""),
			Password: getEnv("KAFKA_PASSWORD", ""),
		},
		Redis: RedisConfig{
			Addr: getEnv("REDIS_ADDR", "localhost:6379"),
			DB:   getEnvInt("REDIS_DB", 0),
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
			Google: GoogleScraperConfig{
				// Listing pacing — defaults to 100-120 s for the HTTP-based scraper.
				MinDelayMs:    getEnvInt("GOOGLE_SCRAPER_MIN_DELAY", 100000),
				MaxDelayMs:    getEnvInt("GOOGLE_SCRAPER_MAX_DELAY", 120000),
				TimeoutMs:     getEnvInt("GOOGLE_SCRAPER_TIMEOUT_MS", 18000000),
				MaxPages:      getEnvInt("GOOGLE_SCRAPER_MAX_PAGES", 0),
				EnrichMinMs:   getEnvInt("GOOGLE_SCRAPER_ENRICH_MIN", 5000),
				EnrichMaxMs:   getEnvInt("GOOGLE_SCRAPER_ENRICH_MAX", 10000),
				MaxDetailJobs: getEnvInt("GOOGLE_SCRAPER_MAX_DETAIL_JOBS", 0),
				ChromiumPath:  getEnv("CHROMIUM_PATH", ""),
			},
		},
		Scheduler: SchedulerConfig{
			ScanIntervalMinutes:  getEnvInt("SCAN_INTERVAL_MINUTES", 1440),
			StuckTimeoutMinutes:  getEnvInt("SCHEDULER_STUCK_TIMEOUT_MINUTES", 30),
			CheckIntervalSeconds: getEnvInt("SCHEDULER_CHECK_INTERVAL_SECONDS", 60),
		},
		Observability: ObservabilityConfig{
			MetricsAddr:    getEnv("METRICS_ADDR", "127.0.0.1:9100"),
			JaegerEndpoint: getEnv("JAEGER_ENDPOINT", "http://localhost:4318"),
		},
		Auth: AuthConfig{
			SupabaseProjectURL:  getEnv("SUPABASE_PROJECT_URL", ""),
			SupabaseJWKSURL:     deriveJWKSURL(getEnv("SUPABASE_PROJECT_URL", ""), getEnv("SUPABASE_JWKS_URL", ""), getEnv("SUPABASE_JWT_SECRET", "")),
			SupabaseJWTSecret:   getEnv("SUPABASE_JWT_SECRET", ""),
			AllowedEmail:        getEnv("ALLOWED_EMAIL", ""),
			ClockSkewSeconds:    getEnvInt("JWT_CLOCK_SKEW_SECONDS", 60),
			JWKSCacheTTLSeconds: getEnvInt("JWKS_CACHE_TTL_SECONDS", 600),
		},
		API: APIConfig{
			MaxBodyBytes: int64(getEnvInt("API_MAX_BODY_BYTES", 1048576)),
			DevToken:     os.Getenv("DEV_TOKEN"),
		},
	}
}

// deriveJWKSURL returns the explicit JWKS URL when set, otherwise derives it
// from the project URL. Returns "" when HS256 mode is selected (JWT secret is
// set) so the "exactly one of secret/JWKS" invariant stays satisfied.
func deriveJWKSURL(projectURL, explicit, jwtSecret string) string {
	if explicit != "" {
		return explicit
	}
	if jwtSecret != "" {
		return ""
	}
	if projectURL == "" {
		return ""
	}
	return strings.TrimRight(projectURL, "/") + "/auth/v1/.well-known/jwks.json"
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
