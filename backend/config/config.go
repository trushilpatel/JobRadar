package config

import (
	"fmt"
	"log"
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
	LLMProvider   string // anthropic | gemini
	LLMAPIKey     string
	LLMModel      string // e.g. "gemini-3.1-flash-lite", "claude-haiku-4-5-20251001"
	LLMRateLimits LLMRateLimits

	ScanIntervalMinutes       int // how often the scan worker runs a full sweep
	ScanCompanyDelaySeconds   int // sleep between company job-list fetches to avoid being blocked
	ScorerBatchSize           int // number of job titles sent to LLM per ScoreBatch call
	ScorerFlushTimeoutSeconds int // flush a partial batch after this many seconds of inactivity

	TargetRoles []string // e.g. ["Backend Engineer", "Platform Engineer"]
	YearsExp    int
}

func Load() *Config {
	if err := godotenv.Load(); err != nil {
		log.Println("no .env file found, falling back to environment variables")
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
			RPD:   getEnvInt("LLM_RPD", 500),
			TPMIn: getEnvInt("LLM_TPM", 250000),
		},
		ScanIntervalMinutes:       getEnvInt("SCAN_INTERVAL_MINUTES", 1440), // default: 24h
		ScanCompanyDelaySeconds:   getEnvInt("SCAN_COMPANY_DELAY_SECONDS", 60),
		ScorerBatchSize:           getEnvInt("SCORER_BATCH_SIZE", 50),
		ScorerFlushTimeoutSeconds: getEnvInt("SCORER_FLUSH_TIMEOUT_SECONDS", 10),

		TargetRoles: splitCSV(getEnv("TARGET_ROLES", "Software Engineer")),
		YearsExp:    getEnvInt("YEARS_EXP", 0),
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
