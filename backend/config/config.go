package config

import (
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"

	"github.com/joho/godotenv"
)

type Config struct {
	DBHost       string
	DBPort       string
	DBUser       string
	DBPassword   string
	DBName       string
	DBSSLMode    string
	KafkaBrokers  string
	KafkaUser     string
	KafkaPassword string
	LLMProvider   string // anthropic | openai
	LLMAPIKey     string
	TargetRoles   []string // e.g. ["Backend Engineer", "Platform Engineer"]
	YearsExp      int
}

func Load() *Config {
	if err := godotenv.Load(); err != nil {
		log.Println("no .env file found, falling back to environment variables")
	}

	return &Config{
		DBHost:     getEnv("DB_HOST", "localhost"),
		DBPort:     getEnv("DB_PORT", "5432"),
		DBUser:     getEnv("DB_USER", "postgres"),
		DBPassword: getEnv("DB_PASSWORD", "postgres"),
		DBName:     getEnv("DB_NAME", "jobradar"),
		DBSSLMode:    getEnv("DB_SSLMODE", "disable"),
		KafkaBrokers:  getEnv("KAFKA_BROKERS", "localhost:9092"),
		KafkaUser:     getEnv("KAFKA_USER", ""),
		KafkaPassword: getEnv("KAFKA_PASSWORD", ""),
		LLMProvider:   getEnv("LLM_PROVIDER", "anthropic"),
		LLMAPIKey:     getEnv("LLM_API_KEY", ""),
		TargetRoles:   splitCSV(getEnv("TARGET_ROLES", "Software Engineer")),
		YearsExp:      getEnvInt("YEARS_EXP", 0),
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
