package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/joho/godotenv"
)

// LLMRateLimits holds the three quota dimensions for the active LLM model.
// All values are 0 = unlimited. Set these to match your model's free/paid tier.
type LLMRateLimits struct {
	RPM   int // requests per minute
	RPD   int // requests per day (resets at UTC midnight)
	TPMIn int // input tokens per minute
}

type Config struct {
	DBHost        string
	DBPort        string
	DBUser        string
	DBPassword    string
	DBName        string
	DBSSLMode     string
	KafkaBrokers  string
	KafkaUser     string
	KafkaPassword string
	LLMProvider   string // "gemini"
	LLMAPIKey     string
	LLMModel      string // e.g. "gemini-3.1-flash-lite", "claude-haiku-4-5-20251001"
	LLMRateLimits LLMRateLimits

	ScanIntervalMinutes       int // how often the scan worker runs a full sweep
	ScanCompanyDelaySeconds   int // sleep between company job-list fetches to avoid being blocked
	ScorerBatchSize           int // number of job titles sent to LLM per ScoreBatch call
	ScorerFlushTimeoutSeconds int // flush a partial batch after this many seconds of inactivity

	TargetRoles []string // e.g. ["Backend Engineer", "Platform Engineer"]
	YearsExp    int
	MaxExpYears int // DISCARD_JOB_WITH_EXP_GT — drop jobs requiring more than this many years (0 = disabled)

	// Google Careers scraper (Playwright / Node.js)
	GoogleScraperMinDelayMs  int // min delay between page navigations in ms (default: 30000)
	GoogleScraperMaxDelayMs  int // max delay between page navigations in ms (default: 60000)
	GoogleScraperTimeoutMs   int // overall scraper timeout in ms (default: 2700000 = 45 min)
	GoogleScraperMaxPages    int // max pages to paginate, 0 = all (default: 0)
	GoogleScraperEnrichMinMs int // min delay between detail page visits in ms (default: 15000)
	GoogleScraperEnrichMaxMs int // max delay between detail page visits in ms (default: 25000)

	// Prometheus
	MetricsAddr string // address for metrics/health HTTP server (workers); API uses :8080

	// Tracing
	JaegerEndpoint string // OTLP HTTP endpoint, e.g. http://localhost:4318
}

func Load() *Config {
	if err := godotenv.Load(); err != nil {
		fmt.Fprintf(os.Stderr, "config: no .env file found, falling back to environment variables\n")
	}

	return &Config{
		DBHost:        getEnv("DB_HOST", "localhost"),
		DBPort:        getEnv("DB_PORT", "5432"),
		DBUser:        getEnv("DB_USER", "postgres"),
		DBPassword:    getEnv("DB_PASSWORD", "postgres"),
		DBName:        getEnv("DB_NAME", "jobradar"),
		DBSSLMode:     getEnv("DB_SSLMODE", "disable"),
		KafkaBrokers:  getEnv("KAFKA_BROKERS", "localhost:9092"),
		KafkaUser:     getEnv("KAFKA_USER", ""),
		KafkaPassword: getEnv("KAFKA_PASSWORD", ""),
		LLMProvider:   getEnv("LLM_PROVIDER", "gemini"),
		LLMAPIKey:     getEnv("LLM_API_KEY", ""),
		LLMModel:      getEnv("LLM_MODEL", "gemini-3.1-flash-lite-preview"),
		LLMRateLimits: LLMRateLimits{
			RPM:   getEnvInt("LLM_RPM", 15),
			RPD:   getEnvInt("LLM_RPD", 1500),
			TPMIn: getEnvInt("LLM_TPM", 1000000),
		},
		ScanIntervalMinutes:       getEnvInt("SCAN_INTERVAL_MINUTES", 1440), // default: 24h
		ScanCompanyDelaySeconds:   getEnvInt("SCAN_COMPANY_DELAY_SECONDS", 60),
		ScorerBatchSize:           getEnvInt("SCORER_BATCH_SIZE", 50),
		ScorerFlushTimeoutSeconds: getEnvInt("SCORER_FLUSH_TIMEOUT_SECONDS", 10),

		TargetRoles: splitCSV(getEnv("TARGET_ROLES", "Software Engineer")),
		YearsExp:    getEnvInt("YEARS_EXP", 0),
		MaxExpYears: getEnvInt("DISCARD_JOB_WITH_EXP_GT", 0),

		GoogleScraperMinDelayMs:  getEnvInt("GOOGLE_SCRAPER_MIN_DELAY", 120000),
		GoogleScraperMaxDelayMs:  getEnvInt("GOOGLE_SCRAPER_MAX_DELAY", 240000),
		GoogleScraperTimeoutMs:   getEnvInt("GOOGLE_SCRAPER_TIMEOUT_MS", 14400000),
		GoogleScraperMaxPages:    getEnvInt("GOOGLE_SCRAPER_MAX_PAGES", 0),
		GoogleScraperEnrichMinMs: getEnvInt("GOOGLE_SCRAPER_ENRICH_MIN", 60000),
		GoogleScraperEnrichMaxMs: getEnvInt("GOOGLE_SCRAPER_ENRICH_MAX", 120000),

		MetricsAddr:    getEnv("METRICS_ADDR", ":9090"),
		JaegerEndpoint: getEnv("JAEGER_ENDPOINT", "http://localhost:4318"),
	}
}

func (c *Config) DSN() string {
	return fmt.Sprintf(
		"host=%s port=%s user=%s password=%s dbname=%s sslmode=%s",
		c.DBHost, c.DBPort, c.DBUser, c.DBPassword, c.DBName, c.DBSSLMode,
	)
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
