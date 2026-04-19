package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/joho/godotenv"
)

type Config struct {
	DB            DatabaseConfig
	Kafka         KafkaConfig
	LLM           LLMConfig
	Filter        FilterConfig
	Enricher      EnricherConfig
	Crawler       CrawlerConfig
	Scheduler     SchedulerConfig
	Observability ObservabilityConfig
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
	APIKey      string
	BaseModel   string
	TargetRoles []string
	YearsExp    int
}

// RateLimits holds the three quota dimensions for an LLM model.
// All values are 0 = unlimited. Set these to match your model's free/paid tier.
type RateLimits struct {
	RPM   int
	RPD   int
	TPMIn int
}

type FilterConfig struct {
	Model               string
	BatchSize           int
	FlushTimeoutSeconds int
	LLMParallelism      int
	RateLimits          RateLimits
}

type EnricherConfig struct {
	Model       string
	Concurrency int
	MaxExpYears int
	RateLimits  RateLimits
}

type CrawlerConfig struct {
	CompanyDelaySeconds      int
	PollIntervalSeconds      int
	StuckTimeoutSeconds      int
	HeartbeatIntervalSeconds int
	Google                   GoogleScraperConfig
}

type GoogleScraperConfig struct {
	MinDelayMs  int
	MaxDelayMs  int
	TimeoutMs   int
	MaxPages    int
	EnrichMinMs int
	EnrichMaxMs int
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
	if c.CheckIntervalSeconds <= 0 {
		return fmt.Errorf("config: SCHEDULER_CHECK_INTERVAL_SECONDS must be > 0, got %d", c.CheckIntervalSeconds)
	}
	return nil
}

type ObservabilityConfig struct {
	MetricsAddr    string
	JaegerEndpoint string
}

func Load() *Config {
	if err := godotenv.Load(); err != nil {
		fmt.Fprintf(os.Stderr, "config: no .env file found, falling back to environment variables\n")
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
		LLM: LLMConfig{
			Provider:    getEnv("LLM_PROVIDER", "gemini"),
			APIKey:      getEnv("LLM_API_KEY", ""),
			BaseModel:   getEnv("LLM_MODEL", "gemini-3.1-flash-lite-preview"),
			TargetRoles: splitCSV(getEnv("TARGET_ROLES", "Software Engineer")),
			YearsExp:    getEnvInt("YEARS_EXP", 0),
		},
		Filter: FilterConfig{
			Model:               getEnvWithFallback("FILTER_LLM_MODEL", "LLM_MODEL", "gemini-3.1-flash-lite-preview"),
			BatchSize:           getEnvInt("FILTER_BATCH_SIZE", 50),
			FlushTimeoutSeconds: getEnvInt("FILTER_FLUSH_TIMEOUT_SECONDS", 30),
			LLMParallelism:      getEnvInt("FILTER_LLM_PARALLELISM", 5),
			RateLimits: RateLimits{
				RPM:   getEnvInt("FILTER_RPM", 15),
				RPD:   getEnvInt("FILTER_RPD", 1500),
				TPMIn: getEnvInt("FILTER_TPM", 1000000),
			},
		},
		Enricher: EnricherConfig{
			Model:       getEnvWithFallback("ENRICHER_LLM_MODEL", "LLM_MODEL", "gemma-4-31b-it"),
			Concurrency: getEnvInt("ENRICHER_CONCURRENCY", 5),
			MaxExpYears: getEnvInt("DISCARD_JOB_WITH_EXP_GT", 0),
			RateLimits: RateLimits{
				RPM:   getEnvInt("ENRICHER_RPM", 15),
				RPD:   getEnvInt("ENRICHER_RPD", 1500),
				TPMIn: getEnvInt("ENRICHER_TPM", 1000000),
			},
		},
		Crawler: CrawlerConfig{
			CompanyDelaySeconds:      getEnvInt("SCAN_COMPANY_DELAY_SECONDS", 30),
			PollIntervalSeconds:      getEnvInt("CRAWLER_POLL_INTERVAL_SECONDS", 60),
			StuckTimeoutSeconds:      getEnvInt("CRAWLER_STUCK_TIMEOUT_SECONDS", 120),
			HeartbeatIntervalSeconds: getEnvInt("CRAWLER_HEARTBEAT_INTERVAL_SECONDS", 30),
			Google: GoogleScraperConfig{
				MinDelayMs:  getEnvInt("GOOGLE_SCRAPER_MIN_DELAY", 120000),
				MaxDelayMs:  getEnvInt("GOOGLE_SCRAPER_MAX_DELAY", 240000),
				TimeoutMs:   getEnvInt("GOOGLE_SCRAPER_TIMEOUT_MS", 14400000),
				MaxPages:    getEnvInt("GOOGLE_SCRAPER_MAX_PAGES", 0),
				EnrichMinMs: getEnvInt("GOOGLE_SCRAPER_ENRICH_MIN", 60000),
				EnrichMaxMs: getEnvInt("GOOGLE_SCRAPER_ENRICH_MAX", 120000),
			},
		},
		Scheduler: SchedulerConfig{
			ScanIntervalMinutes:  getEnvInt("SCAN_INTERVAL_MINUTES", 1440),
			StuckTimeoutMinutes:  getEnvInt("SCHEDULER_STUCK_TIMEOUT_MINUTES", 30),
			CheckIntervalSeconds: getEnvInt("SCHEDULER_CHECK_INTERVAL_SECONDS", 60),
		},
		Observability: ObservabilityConfig{
			MetricsAddr:    getEnv("METRICS_ADDR", ":9090"),
			JaegerEndpoint: getEnv("JAEGER_ENDPOINT", "http://localhost:4318"),
		},
	}
}

func getEnv(key, fallback string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return fallback
}

func getEnvWithFallback(key, fallbackKey, defaultVal string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	if val := os.Getenv(fallbackKey); val != "" {
		return val
	}
	return defaultVal
}

func getEnvInt(key string, fallback int) int {
	if val := os.Getenv(key); val != "" {
		if n, err := strconv.Atoi(val); err == nil {
			return n
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
