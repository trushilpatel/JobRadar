# Config Restructure Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the flat `Config` struct with nested sub-structs grouped by concern, and update all consumers to accept only the sub-struct they need.

**Architecture:** A single `config.Load()` returns `*Config` with typed sub-structs (`DB`, `Kafka`, `LLM`, `Filter`, `Enricher`, `Crawler`, `Scheduler`, `Observability`). Infra packages accept their specific sub-struct instead of `*Config`. Worker `main.go` files pass the relevant sub-structs. No behavior change — pure structural refactor.

**Tech Stack:** Go, `github.com/joho/godotenv`, `github.com/google/wire`

---

### Task 1: Restructure config/config.go

**Files:**

- Modify: `backend/config/config.go`

- [ ] **Step 1: Replace the entire file** with the nested sub-struct version:

```go
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
// All values are 0 = unlimited.
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
	CompanyDelaySeconds int
	Google              GoogleScraperConfig
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
	ScanIntervalMinutes int
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
			CompanyDelaySeconds: getEnvInt("SCAN_COMPANY_DELAY_SECONDS", 15),
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
			ScanIntervalMinutes: getEnvInt("SCAN_INTERVAL_MINUTES", 1440),
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
```

---

### Task 2: Update infra/db

**Files:**

- Modify: `backend/infra/db/db.go`

- [ ] **Step 1: Change `Open` to accept `config.DatabaseConfig`**

```go
package db

import (
	"github.com/trushilpatel/jobradar/config"
	"github.com/uptrace/opentelemetry-go-extra/otelgorm"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func Open(cfg config.DatabaseConfig) (*gorm.DB, error) {
	db, err := gorm.Open(postgres.Open(cfg.DSN()), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Info),
	})
	if err != nil {
		return nil, err
	}

	if err := db.Use(otelgorm.NewPlugin()); err != nil {
		return nil, err
	}

	sqlDB, err := db.DB()
	if err != nil {
		return nil, err
	}

	sqlDB.SetMaxOpenConns(10)
	sqlDB.SetMaxIdleConns(5)

	return db, nil
}

func Ping(database *gorm.DB) error {
	sqlDB, err := database.DB()
	if err != nil {
		return err
	}
	return sqlDB.Ping()
}
```

---

### Task 3: Update infra/kafka

**Files:**

- Modify: `backend/infra/kafka/producer.go`
- Modify: `backend/infra/kafka/consumer.go`
- Modify: `backend/infra/kafka/admin.go`
- Modify: `backend/infra/kafka/lag.go`

- [ ] **Step 1: Update producer.go — accept `config.KafkaConfig`**

```go
package kafka

import (
	"context"
	"strings"

	"github.com/trushilpatel/jobradar/config"
	"github.com/trushilpatel/jobradar/infra/tracing"
	"github.com/twmb/franz-go/pkg/kgo"
	"github.com/twmb/franz-go/pkg/sasl/plain"
)

type Producer struct {
	client *kgo.Client
}

func NewProducer(cfg config.KafkaConfig) (*Producer, error) {
	opts := []kgo.Opt{
		kgo.SeedBrokers(strings.Split(cfg.Brokers, ",")...),
	}

	if cfg.User != "" && cfg.Password != "" {
		opts = append(opts, kgo.SASL(plain.Auth{
			User: cfg.User,
			Pass: cfg.Password,
		}.AsMechanism()))
	}

	client, err := kgo.NewClient(opts...)
	if err != nil {
		return nil, err
	}

	return &Producer{client: client}, nil
}

func (p *Producer) Publish(ctx context.Context, topic string, value []byte) error {
	record := &kgo.Record{
		Topic:   topic,
		Value:   value,
		Headers: tracing.InjectKafka(ctx, nil),
	}
	return p.client.ProduceSync(ctx, record).FirstErr()
}

func (p *Producer) Close() {
	p.client.Close()
}
```

- [ ] **Step 2: Update consumer.go — accept `config.KafkaConfig`**

```go
package kafka

import (
	"context"
	"strings"

	"github.com/trushilpatel/jobradar/config"
	"github.com/trushilpatel/jobradar/infra/logger"
	"github.com/twmb/franz-go/pkg/kgo"
	"github.com/twmb/franz-go/pkg/sasl/plain"
)

type Consumer struct {
	client *kgo.Client
	log    logger.Logger
}

func NewConsumer(cfg config.KafkaConfig, log logger.Logger, groupID string, topics ...string) (*Consumer, error) {
	opts := []kgo.Opt{
		kgo.SeedBrokers(strings.Split(cfg.Brokers, ",")...),
		kgo.ConsumerGroup(groupID),
		kgo.ConsumeTopics(topics...),
		kgo.ConsumeResetOffset(kgo.NewOffset().AtStart()),
	}

	if cfg.User != "" && cfg.Password != "" {
		opts = append(opts, kgo.SASL(plain.Auth{
			User: cfg.User,
			Pass: cfg.Password,
		}.AsMechanism()))
	}

	client, err := kgo.NewClient(opts...)
	if err != nil {
		return nil, err
	}

	return &Consumer{client: client, log: log}, nil
}

func (c *Consumer) Poll(ctx context.Context, fn func(topic string, value []byte, headers []kgo.RecordHeader) error) error {
	for {
		fetches := c.client.PollFetches(ctx)
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if err := fetches.Err(); err != nil {
			c.log.Warn("fetch error, retrying", logger.Err(err))
			continue
		}

		var lastErr error
		fetches.EachRecord(func(r *kgo.Record) {
			if err := fn(r.Topic, r.Value, r.Headers); err != nil {
				lastErr = err
			}
		})

		if lastErr != nil {
			return lastErr
		}

		if err := c.client.CommitUncommittedOffsets(ctx); err != nil {
			return err
		}
	}
}

func (c *Consumer) Close() {
	c.client.Close()
}
```

- [ ] **Step 3: Update admin.go — accept `config.KafkaConfig`**

```go
package kafka

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/trushilpatel/jobradar/config"
	"github.com/twmb/franz-go/pkg/kadm"
	"github.com/twmb/franz-go/pkg/kgo"
	"github.com/twmb/franz-go/pkg/sasl/plain"
)

var ensureTopicsBackoff = []time.Duration{2 * time.Second, 5 * time.Second, 10 * time.Second, 20 * time.Second, 30 * time.Second}

func EnsureTopics(ctx context.Context, cfg config.KafkaConfig, topics ...string) error {
	var lastErr error
	for i, delay := range append([]time.Duration{0}, ensureTopicsBackoff...) {
		if i > 0 {
			select {
			case <-time.After(delay):
			case <-ctx.Done():
				return ctx.Err()
			}
		}
		if lastErr = ensureTopicsOnce(ctx, cfg, topics...); lastErr == nil {
			return nil
		}
	}
	return lastErr
}

func ensureTopicsOnce(ctx context.Context, cfg config.KafkaConfig, topics ...string) error {
	opts := []kgo.Opt{
		kgo.SeedBrokers(strings.Split(cfg.Brokers, ",")...),
	}

	if cfg.User != "" && cfg.Password != "" {
		opts = append(opts, kgo.SASL(plain.Auth{
			User: cfg.User,
			Pass: cfg.Password,
		}.AsMechanism()))
	}

	client, err := kgo.NewClient(opts...)
	if err != nil {
		return fmt.Errorf("kafka admin: create client: %w", err)
	}
	defer client.Close()

	admin := kadm.NewClient(client)

	for _, topic := range topics {
		resp, err := admin.CreateTopics(ctx, 1, 1, nil, topic)
		if err != nil {
			return fmt.Errorf("kafka admin: create topic %q: %w", topic, err)
		}
		for _, detail := range resp {
			if detail.Err != nil && !isTopicExists(detail.Err.Error()) {
				return fmt.Errorf("kafka admin: topic %q: %w", topic, detail.Err)
			}
		}
	}

	return nil
}

func isTopicExists(errMsg string) bool {
	return strings.Contains(errMsg, "TOPIC_ALREADY_EXISTS") ||
		strings.Contains(errMsg, "already exists")
}
```

- [ ] **Step 4: Update lag.go — accept `config.KafkaConfig`**

```go
package kafka

import (
	"context"
	"fmt"
	"strings"

	"github.com/trushilpatel/jobradar/config"
	"github.com/twmb/franz-go/pkg/kadm"
	"github.com/twmb/franz-go/pkg/kgo"
	"github.com/twmb/franz-go/pkg/sasl/plain"
)

type LagReader interface {
	GetLag(ctx context.Context, groupID, topic string) (int64, error)
}

type lagReader struct {
	admin *kadm.Client
}

func NewLagReader(cfg config.KafkaConfig) (LagReader, error) {
	opts := []kgo.Opt{
		kgo.SeedBrokers(strings.Split(cfg.Brokers, ",")...),
	}
	if cfg.User != "" && cfg.Password != "" {
		opts = append(opts, kgo.SASL(plain.Auth{
			User: cfg.User,
			Pass: cfg.Password,
		}.AsMechanism()))
	}
	client, err := kgo.NewClient(opts...)
	if err != nil {
		return nil, fmt.Errorf("kafka lag reader: create client: %w", err)
	}
	return &lagReader{admin: kadm.NewClient(client)}, nil
}

func (r *lagReader) GetLag(ctx context.Context, groupID, topic string) (int64, error) {
	committed, err := r.admin.FetchOffsets(ctx, groupID)
	if err != nil {
		return 0, fmt.Errorf("fetch offsets for group %q: %w", groupID, err)
	}

	endOffsets, err := r.admin.ListEndOffsets(ctx, topic)
	if err != nil {
		return 0, fmt.Errorf("list end offsets for topic %q: %w", topic, err)
	}

	var total int64
	for partition, endOff := range endOffsets[topic] {
		if endOff.Err != nil {
			continue
		}
		committedAt := committed[topic][partition].At
		if committedAt < 0 {
			committedAt = 0
		}
		if lag := endOff.Offset - committedAt; lag > 0 {
			total += lag
		}
	}
	return total, nil
}
```

---

### Task 4: Update cmd/migrate

**Files:**

- Modify: `backend/cmd/migrate/main.go`

- [ ] **Step 1: Change `cfg.DSN()` to `cfg.DB.DSN()`**

```go
package main

import (
	"database/sql"
	"fmt"
	"log"
	"os"

	"github.com/pressly/goose/v3"
	"github.com/trushilpatel/jobradar/config"
	_ "gorm.io/driver/postgres"

	_ "github.com/jackc/pgx/v5/stdlib"
)

func main() {
	cfg := config.Load()

	db, err := sql.Open("pgx", cfg.DB.DSN())
	if err != nil {
		log.Fatalf("failed to open db: %v", err)
	}
	defer func() { _ = db.Close() }()

	if err := db.Ping(); err != nil {
		log.Fatalf("failed to connect to db: %v", err)
	}

	migrationsDir := "migrations"
	if len(os.Args) > 1 {
		migrationsDir = os.Args[1]
	}

	if err := goose.SetDialect("postgres"); err != nil {
		log.Fatalf("failed to set dialect: %v", err)
	}

	if err := goose.Up(db, migrationsDir); err != nil {
		log.Fatalf("failed to run migrations: %v", err)
	}

	fmt.Println("migrations applied successfully")
}
```

---

### Task 5: Update cmd/api/wire.go and wire_gen.go

**Files:**

- Modify: `backend/cmd/api/wire.go`
- Modify: `backend/cmd/api/wire_gen.go`

- [ ] **Step 1: Update wire.go** — add `provideKafkaConfig`, update existing providers to use nested fields

```go
//go:build wireinject

package main

import (
	"context"

	"github.com/google/wire"
	"github.com/trushilpatel/jobradar/api"
	"github.com/trushilpatel/jobradar/config"
	"github.com/trushilpatel/jobradar/service/application"
	"github.com/trushilpatel/jobradar/service/company"
	"github.com/trushilpatel/jobradar/service/dashboard"
	"github.com/trushilpatel/jobradar/service/insights"
	"github.com/trushilpatel/jobradar/service/job"
	"github.com/trushilpatel/jobradar/service/scan"
	"gorm.io/gorm"
)

func provideKafkaConfig(cfg *config.Config) config.KafkaConfig {
	return cfg.Kafka
}

func provideCompanyConfig(cfg *config.Config) company.Config {
	return company.Config{ScanIntervalMinutes: cfg.Scheduler.ScanIntervalMinutes}
}

func provideScanConfig(cfg *config.Config) scan.Config {
	return scan.Config{ScanIntervalMinutes: cfg.Scheduler.ScanIntervalMinutes}
}

func provideScanLogReader(s scan.ScanLogStore) scan.ScanLogReader {
	return s
}

func provideSeenLedgerWriter(s job.SeenLedgerStore) job.SeenLedgerWriter {
	return s
}

type jobCountReaderAdapter struct{ store job.JobStore }

func (a *jobCountReaderAdapter) GetJobCounts() (int64, int64, int64, error) {
	counts, err := a.store.GetJobCounts(context.Background())
	if err != nil {
		return 0, 0, 0, err
	}
	return counts.New, counts.Saved, counts.Applied, nil
}

func provideJobCountReader(s job.JobStore) dashboard.JobCountReader {
	return &jobCountReaderAdapter{store: s}
}

func InitializeApp(db *gorm.DB, cfg *config.Config) (*api.App, error) {
	wire.Build(
		provideKafkaConfig,
		provideCompanyConfig,
		provideScanConfig,
		provideScanLogReader,
		provideSeenLedgerWriter,
		provideJobCountReader,
		company.ProviderSet,
		scan.ProviderSet,
		job.ProviderSet,
		application.ProviderSet,
		dashboard.ProviderSet,
		insights.ProviderSet,
		api.NewApp,
	)
	return nil, nil
}
```

- [ ] **Step 2: Update wire_gen.go** — reflect the provider changes and updated `kafka.NewLagReader` call

```go
// Code generated by Wire. DO NOT EDIT.

//go:generate go run -mod=mod github.com/google/wire/cmd/wire
//go:build !wireinject
// +build !wireinject

package main

import (
	"context"

	"github.com/trushilpatel/jobradar/api"
	"github.com/trushilpatel/jobradar/config"
	"github.com/trushilpatel/jobradar/infra/kafka"
	"github.com/trushilpatel/jobradar/service/application"
	"github.com/trushilpatel/jobradar/service/company"
	"github.com/trushilpatel/jobradar/service/dashboard"
	"github.com/trushilpatel/jobradar/service/insights"
	"github.com/trushilpatel/jobradar/service/job"
	"github.com/trushilpatel/jobradar/service/scan"
	"gorm.io/gorm"
)

// Injectors from wire.go:

func InitializeApp(db *gorm.DB, cfg *config.Config) (*api.App, error) {
	validator := company.NewValidator()
	store := company.NewStore(db)
	companyConfig := provideCompanyConfig(cfg)
	scanQueueStore := scan.NewQueueStore(db)
	scanLogStore := scan.NewLogStore(db)
	scanLogReader := provideScanLogReader(scanLogStore)
	service := company.NewService(store, companyConfig, scanLogStore)
	handler := company.NewHandler(validator, service)
	scanValidator := scan.NewValidator()
	scanConfig := provideScanConfig(cfg)
	scanService := scan.NewService(scanQueueStore, scanLogReader, service, scanConfig)
	scanHandler := scan.NewHandler(scanValidator, scanService)
	jobValidator := job.NewValidator()
	jobStore := job.NewJobStore(db)
	seenLedgerStore := job.NewSeenLedgerStore(db)
	seenLedgerWriter := provideSeenLedgerWriter(seenLedgerStore)
	discardStore := job.NewDiscardStore(db)
	snapshotWriter := job.NewSnapshotStore(db)
	applicationStore := application.NewStore(db)
	jobAppWriter := provideJobApplicationWriter(applicationStore)
	jobService := job.NewService(jobStore, seenLedgerWriter, discardStore, snapshotWriter, jobAppWriter)
	jobHandler := job.NewHandler(jobValidator, jobService)
	applicationValidator := application.NewValidator()
	applicationService := application.NewService(applicationStore, jobService)
	applicationHandler := application.NewHandler(applicationValidator, applicationService)
	dashboardValidator := dashboard.NewValidator()
	insightsStore := insights.NewStore(db)
	insightsService := insights.NewService(insightsStore)
	jobCountReader := provideJobCountReader(jobStore)
	dashboardService := dashboard.NewService(scanLogReader, insightsService, jobCountReader)
	userDashboardStore := dashboard.NewUserDashboardStore(db)
	userDashboardService := dashboard.NewUserDashboardService(userDashboardStore, applicationStore)
	kafkaConfig := provideKafkaConfig(cfg)
	lagReader, err := kafka.NewLagReader(kafkaConfig)
	if err != nil {
		return nil, err
	}
	pipelineStore := dashboard.NewPipelineStore(db)
	pipelineService := dashboard.NewPipelineService(pipelineStore, lagReader)
	dashboardHandler := dashboard.NewHandler(dashboardValidator, dashboardService, userDashboardService, pipelineService)
	insightsValidator := insights.NewValidator()
	insightsHandler := insights.NewHandler(insightsValidator, insightsService)
	app := api.NewApp(handler, scanHandler, jobHandler, applicationHandler, dashboardHandler, insightsHandler)
	return app, nil
}

// wire.go:

func provideKafkaConfig(cfg *config.Config) config.KafkaConfig {
	return cfg.Kafka
}

func provideCompanyConfig(cfg *config.Config) company.Config {
	return company.Config{ScanIntervalMinutes: cfg.Scheduler.ScanIntervalMinutes}
}

func provideScanConfig(cfg *config.Config) scan.Config {
	return scan.Config{ScanIntervalMinutes: cfg.Scheduler.ScanIntervalMinutes}
}

func provideScanLogReader(s scan.ScanLogStore) scan.ScanLogReader {
	return s
}

func provideSeenLedgerWriter(s job.SeenLedgerStore) job.SeenLedgerWriter {
	return s
}

type jobCountReaderAdapter struct{ store job.JobStore }

func (a *jobCountReaderAdapter) GetJobCounts() (int64, int64, int64, error) {
	counts, err := a.store.GetJobCounts(context.Background())
	if err != nil {
		return 0, 0, 0, err
	}
	return counts.New, counts.Saved, counts.Applied, nil
}

func provideJobCountReader(s job.JobStore) dashboard.JobCountReader {
	return &jobCountReaderAdapter{store: s}
}

func provideJobApplicationWriter(s application.Store) job.ApplicationWriter {
	return s
}
```

---

### Task 6: Update cmd/api/main.go

**Files:**

- Modify: `backend/cmd/api/main.go`

- [ ] **Step 1: Update to use sub-structs**

```go
package main

import (
	"context"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/trushilpatel/jobradar/api"
	"github.com/trushilpatel/jobradar/config"
	"github.com/trushilpatel/jobradar/infra/db"
	"github.com/trushilpatel/jobradar/infra/logger"
	"github.com/trushilpatel/jobradar/infra/tracing"
)

func main() {
	cfg := config.Load()
	log := logger.Init("api")

	shutdown := tracing.Init("jobradar-api", cfg.Observability.JaegerEndpoint)
	defer shutdown()

	database, err := db.Open(cfg.DB)
	if err != nil {
		log.Error("db open failed", err)
		os.Exit(1)
	}

	app, err := InitializeApp(database, cfg)
	if err != nil {
		log.Error("wire initialization failed", err)
		os.Exit(1)
	}

	router := api.NewRouter(log, database, app.Handlers())

	srv := &http.Server{
		Addr:         ":8080",
		Handler:      router,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	go func() {
		log.Info("listening", logger.Str("addr", srv.Addr))
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Error("server error", err)
			os.Exit(1)
		}
	}()

	<-ctx.Done()
	log.Info("shutting down")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	srv.Shutdown(shutdownCtx) //nolint:errcheck
}
```

---

### Task 7: Update cmd/job_listing_relevance_filter/main.go

**Files:**

- Modify: `backend/cmd/job_listing_relevance_filter/main.go`

- [ ] **Step 1: Update to use sub-structs**

```go
package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/trushilpatel/jobradar/config"
	"github.com/trushilpatel/jobradar/infra/db"
	"github.com/trushilpatel/jobradar/infra/kafka"
	"github.com/trushilpatel/jobradar/infra/logger"
	"github.com/trushilpatel/jobradar/infra/metrics"
	"github.com/trushilpatel/jobradar/infra/tracing"
	"github.com/trushilpatel/jobradar/pipeline/filter"
	"github.com/trushilpatel/jobradar/pipeline/llm"
	jobsvc "github.com/trushilpatel/jobradar/service/job"
)

func main() {
	cfg := config.Load()
	log := logger.Init("filter")

	shutdown := tracing.Init("jobradar-filter", cfg.Observability.JaegerEndpoint)
	defer shutdown()

	database, err := db.Open(cfg.DB)
	if err != nil {
		log.Error("db open failed", err)
		os.Exit(1)
	}

	go metrics.Serve(cfg.Observability.MetricsAddr, func() error { return db.Ping(database) })

	if err := kafka.EnsureTopics(context.Background(), cfg.Kafka, kafka.TopicJobListingsRaw, kafka.TopicJobListingsFiltered); err != nil {
		log.Error("kafka topics failed", err)
		os.Exit(1)
	}

	consumer, err := kafka.NewConsumer(cfg.Kafka, log, "job-listing-relevance-filter", kafka.TopicJobListingsRaw)
	if err != nil {
		log.Error("kafka consumer failed", err)
		os.Exit(1)
	}
	defer consumer.Close()

	producer, err := kafka.NewProducer(cfg.Kafka)
	if err != nil {
		log.Error("kafka producer failed", err)
		os.Exit(1)
	}
	defer producer.Close()

	scorer, err := llm.NewScorer(llm.ScorerOptions{
		Provider:    cfg.LLM.Provider,
		APIKey:      cfg.LLM.APIKey,
		Model:       cfg.Filter.Model,
		TargetRoles: cfg.LLM.TargetRoles,
		YearsExp:    cfg.LLM.YearsExp,
		Log:         log,
		Limits: llm.RateLimits{
			RPM:   cfg.Filter.RateLimits.RPM,
			RPD:   cfg.Filter.RateLimits.RPD,
			TPMIn: cfg.Filter.RateLimits.TPMIn,
		},
	})
	if err != nil {
		log.Error("llm init failed", err)
		os.Exit(1)
	}

	ledger := jobsvc.NewSeenLedgerStore(database)
	worker := filter.NewWorker(consumer, producer, scorer, ledger,
		cfg.Filter.BatchSize, cfg.Filter.FlushTimeoutSeconds, cfg.Filter.LLMParallelism, log)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if err := worker.Run(ctx); err != nil && ctx.Err() == nil {
		log.Error("worker exited with error", err)
		os.Exit(1)
	}
	log.Info("shutting down")
}
```

---

### Task 8: Update cmd/job_post_analyzer/main.go

**Files:**

- Modify: `backend/cmd/job_post_analyzer/main.go`

- [ ] **Step 1: Update to use sub-structs**

```go
package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/trushilpatel/jobradar/config"
	"github.com/trushilpatel/jobradar/infra/db"
	"github.com/trushilpatel/jobradar/infra/kafka"
	"github.com/trushilpatel/jobradar/infra/logger"
	"github.com/trushilpatel/jobradar/infra/metrics"
	"github.com/trushilpatel/jobradar/infra/tracing"
	"github.com/trushilpatel/jobradar/pipeline/analyzer"
	"github.com/trushilpatel/jobradar/pipeline/llm"
	jobsvc "github.com/trushilpatel/jobradar/service/job"
)

func main() {
	cfg := config.Load()
	log := logger.Init("enricher")

	shutdown := tracing.Init("jobradar-enricher", cfg.Observability.JaegerEndpoint)
	defer shutdown()

	database, err := db.Open(cfg.DB)
	if err != nil {
		log.Error("db open failed", err)
		os.Exit(1)
	}

	go metrics.Serve(cfg.Observability.MetricsAddr, func() error { return db.Ping(database) })

	consumer, err := kafka.NewConsumer(cfg.Kafka, log, "job-post-analyzer", kafka.TopicJobListingsFiltered)
	if err != nil {
		log.Error("kafka consumer failed", err)
		os.Exit(1)
	}
	defer consumer.Close()

	scorer, err := llm.NewScorer(llm.ScorerOptions{
		Provider:    cfg.LLM.Provider,
		APIKey:      cfg.LLM.APIKey,
		Model:       cfg.Enricher.Model,
		TargetRoles: cfg.LLM.TargetRoles,
		YearsExp:    cfg.LLM.YearsExp,
		Log:         log,
		Limits: llm.RateLimits{
			RPM:   cfg.Enricher.RateLimits.RPM,
			RPD:   cfg.Enricher.RateLimits.RPD,
			TPMIn: cfg.Enricher.RateLimits.TPMIn,
		},
	})
	if err != nil {
		log.Error("llm init failed", err)
		os.Exit(1)
	}

	jobStore := jobsvc.NewJobStore(database)
	ledgerStore := jobsvc.NewSeenLedgerStore(database)
	snapshotStore := jobsvc.NewSnapshotStore(database)
	techTagStore := jobsvc.NewTechTagStore(database)
	pipeline := jobsvc.NewPipelineService(jobStore, ledgerStore, snapshotStore, techTagStore)
	worker := analyzer.NewWorker(consumer, scorer, pipeline, log, cfg.Enricher.MaxExpYears, cfg.Enricher.Concurrency)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if err := worker.Run(ctx); err != nil && ctx.Err() == nil {
		log.Error("worker exited with error", err)
		os.Exit(1)
	}
	log.Info("shutting down")
}
```

---

### Task 9: Update cmd/job_listing_crawler/main.go

**Files:**

- Modify: `backend/cmd/job_listing_crawler/main.go`

- [ ] **Step 1: Update to use sub-structs**

```go
package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/trushilpatel/jobradar/config"
	"github.com/trushilpatel/jobradar/infra/db"
	"github.com/trushilpatel/jobradar/infra/kafka"
	"github.com/trushilpatel/jobradar/infra/logger"
	"github.com/trushilpatel/jobradar/infra/metrics"
	"github.com/trushilpatel/jobradar/infra/tracing"
	"github.com/trushilpatel/jobradar/pipeline/ats/google"
	"github.com/trushilpatel/jobradar/pipeline/crawler"
	companysvc "github.com/trushilpatel/jobradar/service/company"
	jobsvc "github.com/trushilpatel/jobradar/service/job"
	scansvc "github.com/trushilpatel/jobradar/service/scan"
)

const pollInterval = 10 * time.Second
const stuckTimeout = 120 * time.Second
const heartbeatInterval = 30 * time.Second

func main() {
	cfg := config.Load()
	log := logger.Init("crawler")

	shutdown := tracing.Init("jobradar-crawler", cfg.Observability.JaegerEndpoint)
	defer shutdown()

	database, err := db.Open(cfg.DB)
	if err != nil {
		log.Error("db open failed", err)
		os.Exit(1)
	}

	go metrics.Serve(cfg.Observability.MetricsAddr, func() error { return db.Ping(database) })

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if err := kafka.EnsureTopics(ctx, cfg.Kafka, kafka.TopicJobListingsRaw); err != nil {
		log.Error("kafka topics failed", err)
		os.Exit(1)
	}

	producer, err := kafka.NewProducer(cfg.Kafka)
	if err != nil {
		log.Error("kafka producer failed", err)
		os.Exit(1)
	}
	defer producer.Close()

	companies := companysvc.NewStore(database)
	ledger := jobsvc.NewSeenLedgerStore(database)
	scanLog := scansvc.NewLogStore(database)
	queue := scansvc.NewQueueStore(database)
	recorder := scansvc.NewCrawlerService(scanLog)

	companyDelay := time.Duration(cfg.Crawler.CompanyDelaySeconds) * time.Second
	googleCfg := google.Config{
		MinDelayMs:  cfg.Crawler.Google.MinDelayMs,
		MaxDelayMs:  cfg.Crawler.Google.MaxDelayMs,
		TimeoutMs:   cfg.Crawler.Google.TimeoutMs,
		MaxPages:    cfg.Crawler.Google.MaxPages,
		EnrichMinMs: cfg.Crawler.Google.EnrichMinMs,
		EnrichMaxMs: cfg.Crawler.Google.EnrichMaxMs,
	}
	engine := crawler.NewEngine(companies, ledger, recorder, producer, googleCfg, log)

	if err := queue.ResetStuck(ctx, stuckTimeout); err != nil {
		log.Error("failed to reset stuck queue items", err)
	}

	log.Info("worker started", logger.Dur("poll_interval", pollInterval))

	pollAndProcess(ctx, log, queue, engine, companyDelay)

	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			if err := queue.ResetStuck(ctx, stuckTimeout); err != nil {
				log.Error("failed to reset stuck queue items", err)
			}
			pollAndProcess(ctx, log, queue, engine, companyDelay)
		case <-ctx.Done():
			log.Info("shutting down")
			return
		}
	}
}

func pollAndProcess(ctx context.Context, log logger.Logger, queue scansvc.ClaimNexter, engine *crawler.Engine, companyDelay time.Duration) {
	item, err := queue.ClaimNext(ctx)
	if err != nil {
		log.Error("failed to claim queue item", err)
		return
	}
	if item == nil {
		log.Debug("queue empty")
		return
	}

	log.Info("processing company from queue",
		logger.Str(logger.FieldCompany, item.CompanyID),
		logger.Str("source", item.Source),
	)

	stopHeartbeat := startHeartbeat(ctx, queue, item.ID, heartbeatInterval, log)
	defer stopHeartbeat()

	engine.RunForCompany(ctx, item.CompanyID)

	if err := queue.Complete(ctx, item.ID); err != nil {
		log.Error("failed to complete queue item", err, logger.Str(logger.FieldCompany, item.CompanyID))
	}

	if companyDelay > 0 {
		log.Debug("pacing before next claim", logger.Dur("delay", companyDelay))
		select {
		case <-time.After(companyDelay):
		case <-ctx.Done():
		}
	}
}

type heartbeater interface {
	UpdateHeartbeat(ctx context.Context, id string) error
}

func startHeartbeat(ctx context.Context, q heartbeater, id string, interval time.Duration, log logger.Logger) func() {
	hbCtx, cancel := context.WithCancel(ctx)
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				if err := q.UpdateHeartbeat(hbCtx, id); err != nil {
					log.Warn("heartbeat update failed", logger.Str(logger.FieldQueueID, id), logger.Err(err))
				}
			case <-hbCtx.Done():
				return
			}
		}
	}()
	return cancel
}
```

---

### Task 10: Update cmd/company_scan_scheduler/main.go

**Files:**

- Modify: `backend/cmd/company_scan_scheduler/main.go`

- [ ] **Step 1: Update to use sub-structs**

```go
package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/trushilpatel/jobradar/config"
	"github.com/trushilpatel/jobradar/infra/db"
	"github.com/trushilpatel/jobradar/infra/logger"
	"github.com/trushilpatel/jobradar/infra/metrics"
	"github.com/trushilpatel/jobradar/infra/tracing"
	"github.com/trushilpatel/jobradar/model"
	companysvc "github.com/trushilpatel/jobradar/service/company"
	scansvc "github.com/trushilpatel/jobradar/service/scan"
)

const checkInterval = 1 * time.Minute

func main() {
	cfg := config.Load()
	log := logger.Init("scheduler")

	shutdown := tracing.Init("jobradar-scheduler", cfg.Observability.JaegerEndpoint)
	defer shutdown()

	database, err := db.Open(cfg.DB)
	if err != nil {
		log.Error("db open failed", err)
		os.Exit(1)
	}

	go metrics.Serve(cfg.Observability.MetricsAddr, func() error { return db.Ping(database) })

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	companies := companysvc.NewStore(database)
	queue := scansvc.NewQueueStore(database)

	log.Info("started",
		logger.Int("scan_interval_minutes", cfg.Scheduler.ScanIntervalMinutes),
		logger.Dur("check_interval", checkInterval),
	)

	enqueuedue(ctx, log, companies, queue, cfg.Scheduler.ScanIntervalMinutes)

	ticker := time.NewTicker(checkInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			enqueuedue(ctx, log, companies, queue, cfg.Scheduler.ScanIntervalMinutes)
		case <-ctx.Done():
			log.Info("shutting down")
			return
		}
	}
}

func enqueuedue(ctx context.Context, log logger.Logger, companies companysvc.SchedulerService, queue scansvc.ScanQueueStore, intervalMinutes int) {
	due, err := companies.GetCompaniesDueForScan(ctx, intervalMinutes)
	if err != nil {
		log.Error("failed to query due companies", err)
		return
	}
	if len(due) == 0 {
		return
	}

	log.Info("companies due for scan", logger.Int("count", len(due)))

	for _, c := range due {
		if err := queue.Enqueue(ctx, c.ID, model.QueueSourceScheduler); err != nil {
			log.Error("enqueue failed", err, logger.Str(logger.FieldCompany, c.ID))
			continue
		}
		metrics.SchedulerCompaniesEnqueuedTotal.Inc()
		log.Debug("queued company", logger.Str(logger.FieldCompany, c.ID), logger.Str("name", c.Name))
	}
}
```

---

### Task 11: Build verification

**Files:** none

- [ ] **Step 1: Run full build**

```bash
cd backend && go build ./...
```

Expected: no output (clean build)

- [ ] **Step 2: Run tests**

```bash
cd backend && go test ./...
```

Expected: all tests pass
