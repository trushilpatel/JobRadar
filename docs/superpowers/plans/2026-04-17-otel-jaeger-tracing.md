# OpenTelemetry + Jaeger Distributed Tracing Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add end-to-end distributed tracing across all 5 JobRadar services using OpenTelemetry Go SDK with Jaeger as the trace backend, so a single job processed through Crawler → Filter → Enricher appears as one unified trace.

**Architecture:** `context.Context` carries the active span through every function call within a process. Across Kafka boundaries, the W3C `traceparent` header is injected into message headers by the producer and extracted by the consumer to continue the same trace. A single `infra/tracing` package owns all OTel bootstrap and helpers.

**Tech Stack:** `go.opentelemetry.io/otel` SDK, OTLP HTTP exporter, `otelgorm` plugin for automatic DB spans, `otelhttp` middleware for automatic HTTP spans, `jaegertracing/all-in-one:1.57` Docker image.

---

## File Map

**New files:**

- `backend/infra/tracing/tracer.go` — OTel bootstrap, Init/Shutdown
- `backend/infra/tracing/span.go` — Start/End helpers used everywhere
- `backend/infra/tracing/kafka.go` — W3C traceparent inject/extract for franz-go

**Modified files (infra/config):**

- `backend/config/config.go` — add `JaegerEndpoint` field
- `backend/infra/db/db.go` — register GORM OTel plugin
- `backend/infra/kafka/producer.go` — inject trace headers on publish
- `backend/infra/kafka/consumer.go` — expose headers in Poll callback + extract trace
- `backend/api/router.go` — mount OTel HTTP middleware
- `docker-compose.yml` — add Jaeger, uncomment Prometheus + Grafana

**Modified files (service layer — add ctx + tracing):**

- `backend/service/job/store.go` — all interfaces + GORM impls
- `backend/service/job/service.go` — interface + impl
- `backend/service/job/pipeline_service.go` — interface + impl
- `backend/service/job/handler.go` — pass r.Context() to all service calls
- `backend/service/company/store.go`
- `backend/service/company/service.go`
- `backend/service/company/handler.go`
- `backend/service/application/store.go`
- `backend/service/application/service.go`
- `backend/service/application/handler.go`
- `backend/service/insights/store.go`
- `backend/service/insights/service.go`
- `backend/service/insights/handler.go`
- `backend/service/dashboard/store.go`
- `backend/service/dashboard/service.go`
- `backend/service/dashboard/userdashboard.go`
- `backend/service/dashboard/handler.go`
- `backend/service/scan/store.go`
- `backend/service/scan/service.go`
- `backend/service/scan/crawler_service.go`
- `backend/service/scan/handler.go`

**Modified files (pipeline):**

- `backend/pipeline/crawler/interfaces.go`
- `backend/pipeline/crawler/engine.go`
- `backend/pipeline/filter/interfaces.go`
- `backend/pipeline/filter/worker.go`
- `backend/pipeline/analyzer/worker.go`

**Modified files (cmd):**

- `backend/cmd/api/main.go`
- `backend/cmd/company_scan_scheduler/main.go`
- `backend/cmd/job_listing_crawler/main.go`
- `backend/cmd/job_listing_relevance_filter/main.go`
- `backend/cmd/job_post_analyzer/main.go`

**Modified files (test updates — add context.Background()):**

- All `*_test.go` files in affected service packages

---

## Task 1: Add Go Dependencies

**Files:**

- Modify: `backend/go.mod`, `backend/go.sum`

- [ ] **Step 1: Add OTel packages**

```bash
cd backend
go get go.opentelemetry.io/otel@latest
go get go.opentelemetry.io/otel/trace@latest
go get go.opentelemetry.io/otel/sdk@latest
go get go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp@latest
go get go.opentelemetry.io/otel/propagation@latest
go get go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp@latest
go get go.opentelemetry.io/contrib/instrumentation/gorm.io/gorm/otelgorm@latest
go mod tidy
```

- [ ] **Step 2: Verify build still compiles**

```bash
cd backend
go build ./...
```

Expected: no errors (no code changed yet, just deps added).

- [ ] **Step 3: Commit**

```bash
git add backend/go.mod backend/go.sum
git commit -m "deps: add OpenTelemetry SDK and contrib packages"
```

---

## Task 2: Create `infra/tracing/` Package

**Files:**

- Create: `backend/infra/tracing/tracer.go`
- Create: `backend/infra/tracing/span.go`
- Create: `backend/infra/tracing/kafka.go`

- [ ] **Step 1: Create `tracer.go`**

```go
// backend/infra/tracing/tracer.go
package tracing

import (
	"context"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.24.0"
)

// Init bootstraps the global OTel tracer provider and propagator.
// endpoint is the OTLP HTTP endpoint, e.g. "http://jaeger:4318".
// Returns a shutdown function that flushes pending spans — call it with defer.
func Init(serviceName, endpoint string) func() {
	ctx := context.Background()

	exp, err := otlptracehttp.New(ctx,
		otlptracehttp.WithEndpointURL(endpoint),
		otlptracehttp.WithInsecure(),
	)
	if err != nil {
		panic("tracing: failed to create OTLP exporter: " + err.Error())
	}

	res, _ := resource.Merge(
		resource.Default(),
		resource.NewWithAttributes(
			semconv.SchemaURL,
			semconv.ServiceName(serviceName),
		),
	)

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exp),
		sdktrace.WithResource(res),
	)

	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	return func() {
		tp.Shutdown(context.Background()) //nolint:errcheck
	}
}
```

- [ ] **Step 2: Create `span.go`**

```go
// backend/infra/tracing/span.go
package tracing

import (
	"context"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

// Start begins a named span as a child of whatever span is in ctx.
// The returned ctx carries the new span — pass it to all downstream calls.
// Always pair with End: defer tracing.End(span, &err)
func Start(ctx context.Context, name string) (context.Context, trace.Span) {
	return otel.Tracer("jobradar").Start(ctx, name)
}

// End closes span and records err if non-nil.
// Pass a pointer to the named return error so defer captures the final value:
//
//	func Foo(ctx context.Context) (err error) {
//	    ctx, span := tracing.Start(ctx, "pkg.Foo")
//	    defer tracing.End(span, &err)
//	    ...
//	}
func End(span trace.Span, errPtr *error) {
	if errPtr != nil && *errPtr != nil {
		span.RecordError(*errPtr)
		span.SetStatus(codes.Error, (*errPtr).Error())
	}
	span.End()
}
```

- [ ] **Step 3: Create `kafka.go`**

```go
// backend/infra/tracing/kafka.go
package tracing

import (
	"context"

	"github.com/twmb/franz-go/pkg/kgo"
	"go.opentelemetry.io/otel"
)

// kafkaCarrier adapts []kgo.RecordHeader to propagation.TextMapCarrier.
type kafkaCarrier []kgo.RecordHeader

func (c kafkaCarrier) Get(key string) string {
	for _, h := range c {
		if h.Key == key {
			return string(h.Value)
		}
	}
	return ""
}

func (c *kafkaCarrier) Set(key, value string) {
	*c = append(*c, kgo.RecordHeader{Key: key, Value: []byte(value)})
}

func (c kafkaCarrier) Keys() []string {
	keys := make([]string, len(c))
	for i, h := range c {
		keys[i] = h.Key
	}
	return keys
}

// InjectKafka injects the W3C traceparent from ctx into the provided headers slice
// and returns the updated slice. Call before Producer.Publish.
func InjectKafka(ctx context.Context, headers []kgo.RecordHeader) []kgo.RecordHeader {
	carrier := kafkaCarrier(headers)
	otel.GetTextMapPropagator().Inject(ctx, &carrier)
	return []kgo.RecordHeader(carrier)
}

// ExtractKafka extracts the W3C traceparent from message headers and returns
// a new ctx carrying the remote span as the parent. Call after Consumer.Poll delivers a record.
func ExtractKafka(ctx context.Context, headers []kgo.RecordHeader) context.Context {
	carrier := kafkaCarrier(headers)
	return otel.GetTextMapPropagator().Extract(ctx, carrier)
}
```

- [ ] **Step 4: Verify package compiles**

```bash
cd backend
go build ./infra/tracing/...
```

Expected: no errors.

- [ ] **Step 5: Commit**

```bash
git add backend/infra/tracing/
git commit -m "feat(tracing): add infra/tracing package with OTel bootstrap and kafka helpers"
```

---

## Task 3: Docker Compose + Config

**Files:**

- Modify: `docker-compose.yml`
- Modify: `backend/config/config.go`

- [ ] **Step 1: Add `JaegerEndpoint` to Config**

In `backend/config/config.go`, add the field to the struct:

```go
// After MetricsAddr field:
JaegerEndpoint string // OTLP HTTP endpoint for Jaeger, e.g. http://localhost:4318
```

In the `Load()` return statement, add:

```go
JaegerEndpoint: getEnv("JAEGER_ENDPOINT", "http://localhost:4318"),
```

- [ ] **Step 2: Add Jaeger and uncomment observability stack in docker-compose.yml**

In the `# ── Observability ─────────────────────────────────────────────────────────────` section, replace the entire commented block with:

```yaml
jaeger:
  image: jaegertracing/all-in-one:1.57
  container_name: jobradar_jaeger
  ports:
    - '16686:16686' # Jaeger UI
    - '4318:4318' # OTLP HTTP receiver
  environment:
    - COLLECTOR_OTLP_ENABLED=true
  restart: unless-stopped

prometheus:
  image: prom/prometheus:v2.51.0
  container_name: jobradar_prometheus
  ports:
    - '9090:9090'
  volumes:
    - ./backend/deploy/prometheus/prometheus.yml:/etc/prometheus/prometheus.yml
    - prometheusdata:/prometheus
  command:
    - '--config.file=/etc/prometheus/prometheus.yml'
    - '--storage.tsdb.retention.time=15d'
  restart: unless-stopped

grafana:
  image: grafana/grafana:10.4.0
  container_name: jobradar_grafana
  ports:
    - '3001:3000'
  volumes:
    - ./backend/deploy/prometheus/grafana-prometheus-datasource.yml:/etc/grafana/provisioning/datasources/prometheus.yml
    - grafanadata:/var/lib/grafana
  environment:
    - GF_AUTH_ANONYMOUS_ENABLED=true
    - GF_AUTH_ANONYMOUS_ORG_ROLE=Admin
  depends_on:
    - prometheus
  restart: unless-stopped
```

- [ ] **Step 3: Add `JAEGER_ENDPOINT` env var to all 5 services in docker-compose**

For each of `api`, `scheduler`, `crawler`, `filter`, `enricher` services, add to their `environment:` block:

```yaml
JAEGER_ENDPOINT: http://jaeger:4318
```

- [ ] **Step 4: Verify docker-compose is valid**

```bash
docker compose config --quiet
```

Expected: no errors.

- [ ] **Step 5: Commit**

```bash
git add docker-compose.yml backend/config/config.go
git commit -m "feat(tracing): add Jaeger to docker-compose, JaegerEndpoint to config"
```

---

## Task 4: Update Infra Layer (DB, Kafka, Router)

**Files:**

- Modify: `backend/infra/db/db.go`
- Modify: `backend/infra/kafka/producer.go`
- Modify: `backend/infra/kafka/consumer.go`
- Modify: `backend/api/router.go`

- [ ] **Step 1: Register GORM OTel plugin in `infra/db/db.go`**

```go
package db

import (
	"github.com/trushilpatel/jobradar/config"
	"go.opentelemetry.io/contrib/instrumentation/gorm.io/gorm/otelgorm"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func Open(cfg *config.Config) (*gorm.DB, error) {
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

// Ping checks that the database connection is alive.
func Ping(database *gorm.DB) error {
	sqlDB, err := database.DB()
	if err != nil {
		return err
	}
	return sqlDB.Ping()
}
```

- [ ] **Step 2: Update `infra/kafka/producer.go` to inject trace headers**

```go
package kafka

import (
	"context"
	"strings"

	"github.com/twmb/franz-go/pkg/kgo"
	"github.com/twmb/franz-go/pkg/sasl/plain"
	"github.com/trushilpatel/jobradar/config"
	"github.com/trushilpatel/jobradar/infra/tracing"
)

type Producer struct {
	client *kgo.Client
}

func NewProducer(cfg *config.Config) (*Producer, error) {
	opts := []kgo.Opt{
		kgo.SeedBrokers(strings.Split(cfg.KafkaBrokers, ",")...),
	}

	if cfg.KafkaUser != "" && cfg.KafkaPassword != "" {
		opts = append(opts, kgo.SASL(plain.Auth{
			User: cfg.KafkaUser,
			Pass: cfg.KafkaPassword,
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

- [ ] **Step 3: Update `infra/kafka/consumer.go` to expose headers in Poll callback**

The callback signature changes from `func(topic string, value []byte) error` to `func(topic string, value []byte, headers []kgo.RecordHeader) error`. This threads headers to workers so they can extract the trace context.

```go
package kafka

import (
	"context"
	"strings"

	"github.com/twmb/franz-go/pkg/kgo"
	"github.com/twmb/franz-go/pkg/sasl/plain"
	"github.com/trushilpatel/jobradar/config"
	"github.com/trushilpatel/jobradar/infra/logger"
)

type Consumer struct {
	client *kgo.Client
	log    logger.Logger
}

func NewConsumer(cfg *config.Config, log logger.Logger, groupID string, topics ...string) (*Consumer, error) {
	opts := []kgo.Opt{
		kgo.SeedBrokers(strings.Split(cfg.KafkaBrokers, ",")...),
		kgo.ConsumerGroup(groupID),
		kgo.ConsumeTopics(topics...),
		kgo.ConsumeResetOffset(kgo.NewOffset().AtEnd()),
	}

	if cfg.KafkaUser != "" && cfg.KafkaPassword != "" {
		opts = append(opts, kgo.SASL(plain.Auth{
			User: cfg.KafkaUser,
			Pass: cfg.KafkaPassword,
		}.AsMechanism()))
	}

	client, err := kgo.NewClient(opts...)
	if err != nil {
		return nil, err
	}

	return &Consumer{client: client, log: log}, nil
}

// Poll fetches the next batch of records and calls fn for each one.
// fn now receives the Kafka record headers so workers can extract trace context.
// Commits offsets after fn returns nil. Transient fetch errors are logged and skipped.
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

- [ ] **Step 4: Add OTel HTTP middleware to `api/router.go`**

```go
package api

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/trushilpatel/jobradar/api/handler"
	"github.com/trushilpatel/jobradar/api/middleware"
	"github.com/trushilpatel/jobradar/infra/db"
	"github.com/trushilpatel/jobradar/infra/logger"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"gorm.io/gorm"
)

func NewRouter(log logger.Logger, database *gorm.DB, groups []handler.RouteGroup) *chi.Mux {
	r := chi.NewRouter()

	r.Use(otelhttp.NewMiddleware("jobradar-api")) // creates root span per request
	r.Use(middleware.Logger(log))
	r.Use(middleware.Metrics())
	r.Use(chimiddleware.Recoverer)

	r.Handle("/metrics", promhttp.Handler())
	r.Get("/health", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	r.Get("/ready", func(w http.ResponseWriter, _ *http.Request) {
		if err := db.Ping(database); err != nil {
			http.Error(w, err.Error(), http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
	})

	for _, g := range groups {
		g.Register(r)
	}

	return r
}
```

- [ ] **Step 5: Build to catch compile errors from the Poll signature change**

```bash
cd backend
go build ./...
```

Expected: compile errors in `pipeline/filter/worker.go` and `pipeline/analyzer/worker.go` because their Poll callbacks don't accept headers yet. That's fine — they'll be fixed in Tasks 12 and 13.

- [ ] **Step 6: Commit infra changes**

```bash
git add backend/infra/db/db.go backend/infra/kafka/producer.go backend/infra/kafka/consumer.go backend/api/router.go
git commit -m "feat(tracing): wire OTel into DB (GORM plugin), Kafka (inject/extract), HTTP router"
```

---

## Task 5: `service/job` — Thread ctx + Tracing

**Files:**

- Modify: `backend/service/job/store.go`
- Modify: `backend/service/job/service.go`
- Modify: `backend/service/job/pipeline_service.go`
- Modify: `backend/service/job/handler.go`
- Modify: all `*_test.go` in `backend/service/job/`

This task establishes the complete pattern for all service tasks. Every method gets:

1. `ctx context.Context` as first parameter
2. `ctx, span := tracing.Start(ctx, "span.name")` + `defer tracing.End(span, &err)` at the top
3. GORM calls updated to use `s.db.WithContext(ctx)`

- [ ] **Step 1: Update all interfaces in `service/job/store.go`**

Update every interface method to accept `ctx context.Context` as first param. Update all GORM implementations to use `s.db.WithContext(ctx)` and add tracing:

```go
// Interfaces — add ctx to every method:

type JobReader interface {
	GetJobs(ctx context.Context, filter JobFilter, page, limit int) ([]model.Job, int64, error)
	GetJobByID(ctx context.Context, id string) (*model.Job, error)
	GetJobCounts(ctx context.Context) (*JobCounts, error)
	GetJobsAddedByDay(ctx context.Context, days int) ([]DailyJobCount, error)
}

type JobWriter interface {
	InsertJob(ctx context.Context, job *model.Job) error
	DeleteJob(ctx context.Context, id string) error
	UpdateJobStatus(ctx context.Context, id, status string) error
	UpdateJobSaved(ctx context.Context, id string, saved bool) error
	UpdateJobNotes(ctx context.Context, id, notes string) error
}

type SeenLedgerReader interface {
	HasSeen(ctx context.Context, jobID string) (bool, error)
}

type SeenLedgerWriter interface {
	MarkSeen(ctx context.Context, jobID, companyID, reason string) error
	UpdateSeenReason(ctx context.Context, jobID, reason string) error
	UpdateSeenReasonBatch(ctx context.Context, jobIDs []string, reason string) error
	MarkRemovedBatch(ctx context.Context, jobIDs []string) error
}

type SeenLedgerCompanyReader interface {
	GetActiveJobIDsByCompany(ctx context.Context, companyID string) ([]string, error)
}

type DiscardStore interface {
	InsertDiscard(ctx context.Context, jobResultID, reason, note string) error
}

type SnapshotWriter interface {
	InsertSnapshot(ctx context.Context, input SnapshotInput) error
	UpdateSnapshotAction(ctx context.Context, id, action string) error
}

type TechTagWriter interface {
	InsertTechTags(ctx context.Context, jobID, companyID string, techs []string, taggedAt time.Time) error
}
```

Update GORM implementations — pattern shown for `GetJobs` and `InsertJob`; apply to all methods:

```go
func (s *jobStore) GetJobs(ctx context.Context, filter JobFilter, page, limit int) (jobs []model.Job, total int64, err error) {
	ctx, span := tracing.Start(ctx, "job.store.GetJobs")
	defer tracing.End(span, &err)

	query := s.db.WithContext(ctx).Model(&model.Job{})
	// ... rest of filtering logic unchanged ...
	return jobs, total, err
}

func (s *jobStore) InsertJob(ctx context.Context, job *model.Job) (err error) {
	ctx, span := tracing.Start(ctx, "job.store.InsertJob")
	defer tracing.End(span, &err)
	return s.db.WithContext(ctx).Create(job).Error
}

func (s *jobStore) GetJobByID(ctx context.Context, id string) (job *model.Job, err error) {
	ctx, span := tracing.Start(ctx, "job.store.GetJobByID")
	defer tracing.End(span, &err)
	job = &model.Job{}
	err = s.db.WithContext(ctx).Preload("Company").Where("id = ?", id).First(job).Error
	if err != nil {
		return nil, err
	}
	return job, nil
}

func (s *jobStore) DeleteJob(ctx context.Context, id string) (err error) {
	ctx, span := tracing.Start(ctx, "job.store.DeleteJob")
	defer tracing.End(span, &err)
	return s.db.WithContext(ctx).Where("id = ?", id).Delete(&model.Job{}).Error
}

func (s *jobStore) UpdateJobStatus(ctx context.Context, id, status string) (err error) {
	ctx, span := tracing.Start(ctx, "job.store.UpdateJobStatus")
	defer tracing.End(span, &err)
	return s.db.WithContext(ctx).Model(&model.Job{}).Where("id = ?", id).Update("status", status).Error
}

func (s *jobStore) UpdateJobSaved(ctx context.Context, id string, saved bool) (err error) {
	ctx, span := tracing.Start(ctx, "job.store.UpdateJobSaved")
	defer tracing.End(span, &err)
	if saved {
		return s.db.WithContext(ctx).Model(&model.Job{}).
			Where("id = ? AND status != ?", id, model.JobStatusApplied).
			Updates(map[string]any{"saved": true, "status": model.JobStatusSaved}).Error
	}
	return s.db.WithContext(ctx).Exec(
		`UPDATE job_feed SET saved = false, status = CASE WHEN status = ? THEN ? ELSE status END WHERE id = ?`,
		model.JobStatusSaved, model.JobStatusNew, id,
	).Error
}

func (s *jobStore) UpdateJobNotes(ctx context.Context, id, notes string) (err error) {
	ctx, span := tracing.Start(ctx, "job.store.UpdateJobNotes")
	defer tracing.End(span, &err)
	return s.db.WithContext(ctx).Model(&model.Job{}).Where("id = ?", id).Update("notes", notes).Error
}

func (s *jobStore) GetJobCounts(ctx context.Context) (counts *JobCounts, err error) {
	ctx, span := tracing.Start(ctx, "job.store.GetJobCounts")
	defer tracing.End(span, &err)
	var rows []CountsResult
	err = s.db.WithContext(ctx).Raw(`
		SELECT status, COUNT(*) as count FROM job_feed
		WHERE status IN (?, ?, ?, ?, ?) GROUP BY status`,
		model.JobStatusNew, model.JobStatusSaved, model.JobStatusApplied,
		model.JobStatusInterviewing, model.JobStatusOffer,
	).Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	counts = &JobCounts{}
	for _, r := range rows {
		switch r.Status {
		case model.JobStatusNew:
			counts.New = r.Count
		case model.JobStatusSaved:
			counts.Saved = r.Count
		case model.JobStatusApplied:
			counts.Applied = r.Count
		case model.JobStatusInterviewing:
			counts.Interviewing = r.Count
		case model.JobStatusOffer:
			counts.Offer = r.Count
		}
	}
	return counts, nil
}

func (s *jobStore) GetJobsAddedByDay(ctx context.Context, days int) (results []DailyJobCount, err error) {
	ctx, span := tracing.Start(ctx, "job.store.GetJobsAddedByDay")
	defer tracing.End(span, &err)
	err = s.db.WithContext(ctx).Model(&model.Job{}).
		Select("DATE(added_at) as date, COUNT(*) as count").
		Where("added_at >= NOW() - ? * INTERVAL '1 day'", days).
		Group("DATE(added_at)").Order("date ASC").Scan(&results).Error
	return results, err
}

func (s *seenLedgerStore) HasSeen(ctx context.Context, jobID string) (seen bool, err error) {
	ctx, span := tracing.Start(ctx, "job.store.HasSeen")
	defer tracing.End(span, &err)
	var count int64
	err = s.db.WithContext(ctx).Model(&model.JobSeenLedger{}).Where("job_id = ?", jobID).Count(&count).Error
	return count > 0, err
}

func (s *seenLedgerStore) MarkSeen(ctx context.Context, jobID, companyID, reason string) (err error) {
	ctx, span := tracing.Start(ctx, "job.store.MarkSeen")
	defer tracing.End(span, &err)
	return s.db.WithContext(ctx).Create(&model.JobSeenLedger{JobID: jobID, CompanyID: companyID, Reason: reason}).Error
}

func (s *seenLedgerStore) UpdateSeenReason(ctx context.Context, jobID, reason string) (err error) {
	ctx, span := tracing.Start(ctx, "job.store.UpdateSeenReason")
	defer tracing.End(span, &err)
	return s.db.WithContext(ctx).Model(&model.JobSeenLedger{}).Where("job_id = ?", jobID).Update("reason", reason).Error
}

func (s *seenLedgerStore) UpdateSeenReasonBatch(ctx context.Context, jobIDs []string, reason string) (err error) {
	ctx, span := tracing.Start(ctx, "job.store.UpdateSeenReasonBatch")
	defer tracing.End(span, &err)
	if len(jobIDs) == 0 {
		return nil
	}
	return s.db.WithContext(ctx).Model(&model.JobSeenLedger{}).Where("job_id IN ?", jobIDs).Update("reason", reason).Error
}

func (s *seenLedgerStore) MarkRemovedBatch(ctx context.Context, jobIDs []string) (err error) {
	ctx, span := tracing.Start(ctx, "job.store.MarkRemovedBatch")
	defer tracing.End(span, &err)
	if len(jobIDs) == 0 {
		return nil
	}
	return s.db.WithContext(ctx).Model(&model.JobSeenLedger{}).
		Where("job_id IN ? AND removed_at IS NULL", jobIDs).
		Update("removed_at", time.Now()).Error
}

func (s *seenLedgerStore) GetActiveJobIDsByCompany(ctx context.Context, companyID string) (ids []string, err error) {
	ctx, span := tracing.Start(ctx, "job.store.GetActiveJobIDsByCompany")
	defer tracing.End(span, &err)
	err = s.db.WithContext(ctx).Model(&model.JobSeenLedger{}).
		Where("company_id = ? AND removed_at IS NULL", companyID).
		Pluck("job_id", &ids).Error
	return ids, err
}

func (s *discardStore) InsertDiscard(ctx context.Context, jobResultID, reason, note string) (err error) {
	ctx, span := tracing.Start(ctx, "job.store.InsertDiscard")
	defer tracing.End(span, &err)
	var notePtr *string
	if note != "" {
		notePtr = &note
	}
	return s.db.WithContext(ctx).Create(&model.Discarded{JobResultID: jobResultID, Reason: reason, Note: notePtr}).Error
}

func (s *techTagStore) InsertTechTags(ctx context.Context, jobID, companyID string, techs []string, taggedAt time.Time) (err error) {
	ctx, span := tracing.Start(ctx, "job.store.InsertTechTags")
	defer tracing.End(span, &err)
	if len(techs) == 0 {
		return nil
	}
	rows := make([]model.JobTechTag, 0, len(techs))
	for _, tech := range techs {
		rows = append(rows, model.JobTechTag{JobID: jobID, Tech: tech, CompanyID: companyID, TaggedAt: taggedAt})
	}
	return s.db.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(&rows).Error
}

func (s *snapshotStore) InsertSnapshot(ctx context.Context, input SnapshotInput) (err error) {
	ctx, span := tracing.Start(ctx, "job.store.InsertSnapshot")
	defer tracing.End(span, &err)
	row := model.JobSnapshot{
		ID: input.ID, CompanyID: input.CompanyID, SnapshottedAt: time.Now(),
		Remote: input.Remote, Sponsorship: input.Sponsorship,
		RequiresCitizen: input.RequiresCitizen, SecurityClearance: input.SecurityClearance,
		ExperienceYears: input.ExperienceYears, TechStack: input.TechStack,
	}
	if input.Country != "" { row.Country = &input.Country }
	if input.State != "" { row.State = &input.State }
	if input.EmployType != "" { row.EmployType = &input.EmployType }
	if input.RoleCategory != "" { row.RoleCategory = &input.RoleCategory }
	return s.db.WithContext(ctx).Create(&row).Error
}

func (s *snapshotStore) UpdateSnapshotAction(ctx context.Context, id, action string) (err error) {
	ctx, span := tracing.Start(ctx, "job.store.UpdateSnapshotAction")
	defer tracing.End(span, &err)
	return s.db.WithContext(ctx).Model(&model.JobSnapshot{}).
		Where("id = ?", id).Updates(map[string]any{"user_action": action, "actioned_at": time.Now()}).Error
}
```

- [ ] **Step 2: Update `service/job/service.go` interface and implementation**

Update the `Service` interface:

```go
type Service interface {
	List(ctx context.Context, filter JobFilter, page, limit int) ([]model.Job, int, error)
	Get(ctx context.Context, id string) (*model.Job, error)
	GetCounts(ctx context.Context) (*JobCounts, error)
	Dismiss(ctx context.Context, id string, input DismissInput) error
	SetSaved(ctx context.Context, id string, saved bool) error
	SetStatus(ctx context.Context, id, status string) error
	SetNotes(ctx context.Context, id, notes string) error
}
```

Update implementations:

```go
func (s *service) List(ctx context.Context, filter JobFilter, page, limit int) (jobs []model.Job, total int, err error) {
	ctx, span := tracing.Start(ctx, "job.service.List")
	defer tracing.End(span, &err)
	result, count, err := s.jobs.GetJobs(ctx, filter, page, limit)
	if err != nil {
		return nil, 0, domain.ErrInternal.WithMessage("failed to fetch jobs")
	}
	return result, int(count), nil
}

func (s *service) Get(ctx context.Context, id string) (job *model.Job, err error) {
	ctx, span := tracing.Start(ctx, "job.service.Get")
	defer tracing.End(span, &err)
	job, err = s.jobs.GetJobByID(ctx, id)
	if err != nil {
		if store.IsNotFound(err) {
			return nil, domain.ErrNotFound.WithMessage("job not found")
		}
		return nil, domain.ErrInternal.WithMessage("failed to fetch job")
	}
	return job, nil
}

func (s *service) GetCounts(ctx context.Context) (counts *JobCounts, err error) {
	ctx, span := tracing.Start(ctx, "job.service.GetCounts")
	defer tracing.End(span, &err)
	counts, err = s.jobs.GetJobCounts(ctx)
	if err != nil {
		return nil, domain.ErrInternal.WithMessage("failed to fetch job counts")
	}
	return counts, nil
}

func (s *service) Dismiss(ctx context.Context, id string, input DismissInput) (err error) {
	ctx, span := tracing.Start(ctx, "job.service.Dismiss")
	defer tracing.End(span, &err)
	job, err := s.Get(ctx, id)
	if err != nil {
		return err
	}
	if err := s.discard.InsertDiscard(ctx, id, input.Reason, input.Note); err != nil {
		return domain.ErrInternal.WithMessage("failed to discard job")
	}
	if err := s.ledger.UpdateSeenReason(ctx, job.JobPostID, model.SeenReasonUserDismissed); err != nil {
		return domain.ErrInternal.WithMessage("failed to update seen ledger")
	}
	if err := s.jobs.UpdateJobStatus(ctx, id, model.JobStatusDismissed); err != nil {
		return domain.ErrInternal.WithMessage("failed to update job status")
	}
	_ = s.snapshot.UpdateSnapshotAction(ctx, id, model.JobStatusDismissed)
	return nil
}

func (s *service) SetSaved(ctx context.Context, id string, saved bool) (err error) {
	ctx, span := tracing.Start(ctx, "job.service.SetSaved")
	defer tracing.End(span, &err)
	if err := s.jobs.UpdateJobSaved(ctx, id, saved); err != nil {
		return domain.ErrInternal.WithMessage("failed to update job")
	}
	if saved {
		_ = s.snapshot.UpdateSnapshotAction(ctx, id, model.JobStatusSaved)
	}
	return nil
}

func (s *service) SetStatus(ctx context.Context, id, status string) (err error) {
	ctx, span := tracing.Start(ctx, "job.service.SetStatus")
	defer tracing.End(span, &err)
	if err := s.jobs.UpdateJobStatus(ctx, id, status); err != nil {
		return domain.ErrInternal.WithMessage("failed to update status")
	}
	if status == model.JobStatusApplied {
		_ = s.snapshot.UpdateSnapshotAction(ctx, id, model.JobStatusApplied)
	}
	return nil
}

func (s *service) SetNotes(ctx context.Context, id, notes string) (err error) {
	ctx, span := tracing.Start(ctx, "job.service.SetNotes")
	defer tracing.End(span, &err)
	return s.jobs.UpdateJobNotes(ctx, id, notes)
}
```

- [ ] **Step 3: Update `service/job/pipeline_service.go`**

Update the `pipelineLedger` private interface and `PipelineService` public interface:

```go
type pipelineLedger interface {
	HasSeen(ctx context.Context, jobID string) (bool, error)
	UpdateSeenReason(ctx context.Context, jobID, reason string) error
}

type PipelineService interface {
	SaveEnrichedJob(ctx context.Context, input EnrichedJobInput) error
	MarkLedgerError(ctx context.Context, jobID string) error
	HasSeenJob(ctx context.Context, jobID string) (bool, error)
}
```

Update all method implementations by adding `ctx context.Context` as first param, adding `ctx, span := tracing.Start(ctx, "job.pipeline.MethodName")` + `defer tracing.End(span, &err)`, and passing ctx to all store calls.

- [ ] **Step 4: Update `service/job/handler.go` to pass `r.Context()`**

Every handler method that calls a service needs to pass `r.Context()`:

```go
// Example — List handler:
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	// ... filter building unchanged ...
	jobs, total, err := h.jobs.List(r.Context(), filter, p.Page, p.Limit)
	// ... rest unchanged ...
}

// Get handler:
func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	job, err := h.jobs.Get(r.Context(), id)
	// ...
}

// Dismiss handler:
func (h *Handler) Dismiss(w http.ResponseWriter, r *http.Request) {
	// ...
	if err := h.jobs.Dismiss(r.Context(), id, input); err != nil {
	// ...
}

// SetSaved, SetStatus, SetNotes — same pattern: pass r.Context() as first arg
```

- [ ] **Step 5: Update test files — add `context.Background()` as first arg**

In every test in `service/job/service_test.go`, `pipeline_service_test.go`, and `validator_test.go`, update all service and store method calls to pass `context.Background()` as the first argument. Example:

```go
// Before:
jobs, total, err := svc.List(filter, 1, 10)
// After:
jobs, total, err := svc.List(context.Background(), filter, 1, 10)
```

- [ ] **Step 6: Build and test**

```bash
cd backend
go build ./service/job/...
go test ./service/job/... -count=1
```

Expected: all tests pass.

- [ ] **Step 7: Commit**

```bash
git add backend/service/job/
git commit -m "feat(tracing): add ctx + OTel tracing to service/job"
```

---

## Task 6: `service/company` — Thread ctx + Tracing

**Files:**

- Modify: `backend/service/company/store.go`
- Modify: `backend/service/company/service.go`
- Modify: `backend/service/company/handler.go`
- Modify: `backend/service/company/*_test.go`

- [ ] **Step 1: Read the current files**

```bash
cat backend/service/company/store.go
cat backend/service/company/service.go
cat backend/service/company/handler.go
```

- [ ] **Step 2: Add ctx to all interface methods in `store.go`**

For every method in every interface in `service/company/store.go`:

- Add `ctx context.Context` as first parameter to interface method signatures
- Add `ctx context.Context` as first parameter to struct method signatures
- Add `ctx, span := tracing.Start(ctx, "company.store.MethodName")` + `defer tracing.End(span, &err)` at top of each method
- Replace `s.db.` with `s.db.WithContext(ctx).`

- [ ] **Step 3: Add ctx to all interface methods in `service.go`**

For every method in the `Service` interface and its implementation:

- Add `ctx context.Context` as first parameter
- Add `ctx, span := tracing.Start(ctx, "company.service.MethodName")` + `defer tracing.End(span, &err)`
- Pass `ctx` to all store calls

- [ ] **Step 4: Update `handler.go` to pass `r.Context()`**

Every call to `h.svc.SomeMethod(...)` becomes `h.svc.SomeMethod(r.Context(), ...)`.

- [ ] **Step 5: Update test files**

Add `context.Background()` as first arg to all service/store calls in tests.

- [ ] **Step 6: Build and test**

```bash
cd backend
go build ./service/company/...
go test ./service/company/... -count=1
```

- [ ] **Step 7: Commit**

```bash
git add backend/service/company/
git commit -m "feat(tracing): add ctx + OTel tracing to service/company"
```

---

## Task 7: `service/application` — Thread ctx + Tracing

**Files:**

- Modify: `backend/service/application/store.go`
- Modify: `backend/service/application/service.go`
- Modify: `backend/service/application/handler.go`
- Modify: `backend/service/application/*_test.go`

- [ ] **Step 1: Read the current files**

```bash
cat backend/service/application/store.go
cat backend/service/application/service.go
cat backend/service/application/handler.go
```

- [ ] **Step 2: Add ctx to all interface methods in `store.go`**

For every method in every interface:

- Add `ctx context.Context` as first parameter
- Add `ctx, span := tracing.Start(ctx, "application.store.MethodName")` + `defer tracing.End(span, &err)`
- Replace `s.db.` with `s.db.WithContext(ctx).`

- [ ] **Step 3: Add ctx to service interface and implementation**

- Add `ctx context.Context` as first parameter to all `Service` interface methods
- Add tracing to all implementations
- Pass `ctx` to all store calls

- [ ] **Step 4: Update handler to pass `r.Context()`**

- [ ] **Step 5: Update test files with `context.Background()`**

- [ ] **Step 6: Build and test**

```bash
cd backend
go build ./service/application/...
go test ./service/application/... -count=1
```

- [ ] **Step 7: Commit**

```bash
git add backend/service/application/
git commit -m "feat(tracing): add ctx + OTel tracing to service/application"
```

---

## Task 8: `service/insights` — Thread ctx + Tracing

**Files:**

- Modify: `backend/service/insights/store.go`
- Modify: `backend/service/insights/service.go`
- Modify: `backend/service/insights/handler.go`
- Modify: `backend/service/insights/*_test.go`

- [ ] **Step 1: Read the current files**

```bash
cat backend/service/insights/store.go
cat backend/service/insights/service.go
cat backend/service/insights/handler.go
```

- [ ] **Step 2: Add ctx to all interface methods in `store.go`**

- Add `ctx context.Context` as first parameter to all interface and implementation methods
- Add `ctx, span := tracing.Start(ctx, "insights.store.MethodName")` + `defer tracing.End(span, &err)`
- Replace `s.db.` with `s.db.WithContext(ctx).`

- [ ] **Step 3: Add ctx to service interface and implementation**

- Add `ctx context.Context` as first parameter to all `Service` interface methods
- Add `ctx, span := tracing.Start(ctx, "insights.service.MethodName")` + `defer tracing.End(span, &err)`
- Pass `ctx` to all store calls

- [ ] **Step 4: Update handler to pass `r.Context()`**

- [ ] **Step 5: Update test files with `context.Background()`**

- [ ] **Step 6: Build and test**

```bash
cd backend
go build ./service/insights/...
go test ./service/insights/... -count=1
```

- [ ] **Step 7: Commit**

```bash
git add backend/service/insights/
git commit -m "feat(tracing): add ctx + OTel tracing to service/insights"
```

---

## Task 9: `service/dashboard` — Thread ctx + Tracing

**Files:**

- Modify: `backend/service/dashboard/store.go`
- Modify: `backend/service/dashboard/service.go`
- Modify: `backend/service/dashboard/userdashboard.go`
- Modify: `backend/service/dashboard/handler.go`
- Modify: `backend/service/dashboard/*_test.go`

- [ ] **Step 1: Read the current files**

```bash
cat backend/service/dashboard/store.go
cat backend/service/dashboard/service.go
cat backend/service/dashboard/userdashboard.go
cat backend/service/dashboard/handler.go
```

- [ ] **Step 2: Add ctx to all interface methods in `store.go`**

- Add `ctx context.Context` as first parameter to all interface and implementation methods
- Add `ctx, span := tracing.Start(ctx, "dashboard.store.MethodName")` + `defer tracing.End(span, &err)`
- Replace `s.db.` with `s.db.WithContext(ctx).`

- [ ] **Step 3: Add ctx to service interface and implementation**

- Add `ctx context.Context` as first parameter to all methods
- Add `ctx, span := tracing.Start(ctx, "dashboard.service.MethodName")` + `defer tracing.End(span, &err)`
- Pass `ctx` through to store calls and to `userdashboard.go` functions if they are called from here

- [ ] **Step 4: Update `userdashboard.go` if it has standalone functions**

If `userdashboard.go` contains functions called by the service, add `ctx` as first parameter and thread it through.

- [ ] **Step 5: Update handler to pass `r.Context()`**

- [ ] **Step 6: Update test files with `context.Background()`**

- [ ] **Step 7: Build and test**

```bash
cd backend
go build ./service/dashboard/...
go test ./service/dashboard/... -count=1
```

- [ ] **Step 8: Commit**

```bash
git add backend/service/dashboard/
git commit -m "feat(tracing): add ctx + OTel tracing to service/dashboard"
```

---

## Task 10: `service/scan` — Thread ctx + Tracing

**Files:**

- Modify: `backend/service/scan/store.go`
- Modify: `backend/service/scan/service.go`
- Modify: `backend/service/scan/crawler_service.go`
- Modify: `backend/service/scan/handler.go`
- Modify: `backend/service/scan/*_test.go`

- [ ] **Step 1: Read the current files**

```bash
cat backend/service/scan/store.go
cat backend/service/scan/service.go
cat backend/service/scan/crawler_service.go
cat backend/service/scan/handler.go
```

- [ ] **Step 2: Add ctx to all interface methods in `store.go`**

- Add `ctx context.Context` as first parameter to all interface and implementation methods
- Add `ctx, span := tracing.Start(ctx, "scan.store.MethodName")` + `defer tracing.End(span, &err)`
- Replace `s.db.` with `s.db.WithContext(ctx).`

- [ ] **Step 3: Add ctx to service and crawler_service interfaces and implementations**

- Add `ctx context.Context` as first parameter to all methods on `Service` and `CrawlerService` interfaces
- Add `ctx, span := tracing.Start(ctx, "scan.service.MethodName")` + `defer tracing.End(span, &err)`
- Pass `ctx` through to store calls

- [ ] **Step 4: Update handler to pass `r.Context()`**

- [ ] **Step 5: Update test files with `context.Background()`**

- [ ] **Step 6: Build and test**

```bash
cd backend
go build ./service/scan/...
go test ./service/scan/... -count=1
```

- [ ] **Step 7: Commit**

```bash
git add backend/service/scan/
git commit -m "feat(tracing): add ctx + OTel tracing to service/scan"
```

---

## Task 11: `pipeline/crawler` — Thread ctx + Tracing

**Files:**

- Modify: `backend/pipeline/crawler/interfaces.go`
- Modify: `backend/pipeline/crawler/engine.go`

- [ ] **Step 1: Read `engine.go`**

```bash
cat backend/pipeline/crawler/engine.go
```

- [ ] **Step 2: Update `interfaces.go`**

```go
package crawler

import (
	"context"

	"github.com/trushilpatel/jobradar/model"
	scansvc "github.com/trushilpatel/jobradar/service/scan"
)

type CompanyReader interface {
	GetActiveCompanies(ctx context.Context) ([]model.Company, error)
	GetCompanyByID(ctx context.Context, id string) (model.Company, error)
}

type PipelineLedger interface {
	HasSeen(ctx context.Context, jobID string) (bool, error)
	MarkSeen(ctx context.Context, jobID, companyID, reason string) error
	GetActiveJobIDsByCompany(ctx context.Context, companyID string) ([]string, error)
	MarkRemovedBatch(ctx context.Context, jobIDs []string) error
}

type ScanRecorder interface {
	RecordScanResult(ctx context.Context, companyID string, result scansvc.ScanResult) error
}
```

- [ ] **Step 3: Update `engine.go`**

For every method in the crawler engine (especially `RunForCompany` and the DB poll loop):

- Pass `ctx` through to all interface calls
- Add `ctx, span := tracing.Start(ctx, "crawler.engine.MethodName")` + `defer tracing.End(span, &err)` at the top of key methods

- [ ] **Step 4: Build**

```bash
cd backend
go build ./pipeline/crawler/...
```

- [ ] **Step 5: Commit**

```bash
git add backend/pipeline/crawler/
git commit -m "feat(tracing): add ctx + OTel tracing to pipeline/crawler"
```

---

## Task 12: `pipeline/filter` — Thread ctx + Tracing + Fix Poll callback

**Files:**

- Modify: `backend/pipeline/filter/interfaces.go`
- Modify: `backend/pipeline/filter/worker.go`
- Modify: `backend/pipeline/filter/worker_test.go`

The filter worker's `Poll` callback now receives `headers []kgo.RecordHeader` (changed in Task 4). We extract the trace context from these headers so the filter span is a child of the crawler's span.

- [ ] **Step 1: Update `interfaces.go`**

```go
package filter

import "context"

type FilterLedger interface {
	UpdateSeenReasonBatch(ctx context.Context, jobIDs []string, reason string) error
}
```

- [ ] **Step 2: Update the Poll callback in `worker.go` to accept headers and extract trace context**

In the `Run` method, update the goroutine that calls `w.consumer.Poll`:

```go
go func() {
	err := w.consumer.Poll(ctx, func(_ string, value []byte, headers []kgo.RecordHeader) error {
		msgCtx := tracing.ExtractKafka(ctx, headers) // continue trace from crawler
		var job ats.RawJob
		if err := json.Unmarshal(value, &job); err != nil {
			w.log.Warn("malformed message, skipping", logger.Err(err))
			return nil
		}
		select {
		case jobsCh <- jobWithCtx{ctx: msgCtx, job: job}:
		case <-ctx.Done():
		}
		return nil
	})
	errCh <- err
}()
```

This requires a small internal struct to carry the context alongside the job:

```go
type jobWithCtx struct {
	ctx context.Context
	job ats.RawJob
}
```

Update `jobsCh` type from `chan ats.RawJob` to `chan jobWithCtx` and update the batch flush to pass the context:

```go
// In flushBatch, pass the context from the first job in the batch
// (all jobs in a batch come from the same consumer loop context)
func (w *Worker) flushBatch(ctx context.Context, batch []jobWithCtx) {
	ctx, span := tracing.Start(batch[0].ctx, "filter.worker.flushBatch")
	defer tracing.End(span, nil)
	// ... rest of flushBatch unchanged, but use ctx for all calls ...
}
```

Update `w.ledger.UpdateSeenReasonBatch` calls to pass `ctx`.

- [ ] **Step 3: Update `worker_test.go`**

Update any mock implementations of `FilterLedger` to include `ctx context.Context` as first param.

- [ ] **Step 4: Build and test**

```bash
cd backend
go build ./pipeline/filter/...
go test ./pipeline/filter/... -count=1
```

- [ ] **Step 5: Commit**

```bash
git add backend/pipeline/filter/
git commit -m "feat(tracing): add ctx + OTel tracing to pipeline/filter, extract Kafka trace context"
```

---

## Task 13: `pipeline/analyzer` — Thread ctx + Tracing

**Files:**

- Modify: `backend/pipeline/analyzer/worker.go`
- Modify: `backend/pipeline/analyzer/worker_test.go`

- [ ] **Step 1: Update the Poll callback in `worker.go` to accept headers and extract trace context**

In the `Run` method, update the `Poll` callback:

```go
err := w.consumer.Poll(ctx, func(_ string, value []byte, headers []kgo.RecordHeader) error {
	msgCtx := tracing.ExtractKafka(ctx, headers) // continue trace from filter
	var job ats.RawJob
	if err := json.Unmarshal(value, &job); err != nil {
		w.log.Warn("malformed message, skipping", logger.Err(err))
		return nil
	}
	// ...
	sem <- struct{}{}
	wg.Add(1)
	go func() {
		defer wg.Done()
		defer func() { <-sem }()
		w.enrich(msgCtx, job) // pass msgCtx so enrich spans are children of the filter trace
	}()
	return nil
})
```

- [ ] **Step 2: Update `enrich` method to accept and use ctx**

```go
func (w *Worker) enrich(ctx context.Context, job ats.RawJob) {
	ctx, span := tracing.Start(ctx, "analyzer.worker.enrich")
	defer tracing.End(span, nil)
	// pass ctx to all pipeline service calls, LLM calls, etc.
	// w.pipeline.SaveEnrichedJob(ctx, input)
	// w.pipeline.MarkLedgerError(ctx, job.ID)
	// w.scorer.Enrich(ctx, input)
}
```

- [ ] **Step 3: Update `worker_test.go`**

Update mock implementations to include `ctx context.Context` as first param.

- [ ] **Step 4: Build and test**

```bash
cd backend
go build ./pipeline/analyzer/...
go test ./pipeline/analyzer/... -count=1
```

- [ ] **Step 5: Commit**

```bash
git add backend/pipeline/analyzer/
git commit -m "feat(tracing): add ctx + OTel tracing to pipeline/analyzer, extract Kafka trace context"
```

---

## Task 14: Update All `cmd/*/main.go` Binaries

**Files:**

- Modify: `backend/cmd/api/main.go`
- Modify: `backend/cmd/company_scan_scheduler/main.go`
- Modify: `backend/cmd/job_listing_crawler/main.go`
- Modify: `backend/cmd/job_listing_relevance_filter/main.go`
- Modify: `backend/cmd/job_post_analyzer/main.go`

Each `main.go` gets two lines added after `logger.Init`:

```go
shutdown := tracing.Init("SERVICE_NAME", cfg.JaegerEndpoint)
defer shutdown()
```

The service names for each binary:

| Binary                             | Service name           |
| ---------------------------------- | ---------------------- |
| `cmd/api`                          | `"jobradar-api"`       |
| `cmd/company_scan_scheduler`       | `"jobradar-scheduler"` |
| `cmd/job_listing_crawler`          | `"jobradar-crawler"`   |
| `cmd/job_listing_relevance_filter` | `"jobradar-filter"`    |
| `cmd/job_post_analyzer`            | `"jobradar-enricher"`  |

- [ ] **Step 1: Update `cmd/api/main.go`**

```go
func main() {
	cfg := config.Load()
	log := logger.Init("api")

	shutdown := tracing.Init("jobradar-api", cfg.JaegerEndpoint)
	defer shutdown()

	// ... rest unchanged ...
}
```

Add import: `"github.com/trushilpatel/jobradar/infra/tracing"`

- [ ] **Step 2: Update `cmd/company_scan_scheduler/main.go`**

Add after `logger.Init`:

```go
shutdown := tracing.Init("jobradar-scheduler", cfg.JaegerEndpoint)
defer shutdown()
```

- [ ] **Step 3: Update `cmd/job_listing_crawler/main.go`**

Add after `logger.Init`:

```go
shutdown := tracing.Init("jobradar-crawler", cfg.JaegerEndpoint)
defer shutdown()
```

- [ ] **Step 4: Update `cmd/job_listing_relevance_filter/main.go`**

Add after `logger.Init`:

```go
shutdown := tracing.Init("jobradar-filter", cfg.JaegerEndpoint)
defer shutdown()
```

- [ ] **Step 5: Update `cmd/job_post_analyzer/main.go`**

Add after `logger.Init`:

```go
shutdown := tracing.Init("jobradar-enricher", cfg.JaegerEndpoint)
defer shutdown()
```

- [ ] **Step 6: Build all binaries**

```bash
cd backend
go build ./cmd/...
```

Expected: all 5 binaries compile cleanly.

- [ ] **Step 7: Run all tests**

```bash
cd backend
go test ./... -count=1
```

Expected: all tests pass.

- [ ] **Step 8: Commit**

```bash
git add backend/cmd/
git commit -m "feat(tracing): init OTel tracing in all cmd binaries"
```

---

## Task 15: End-to-End Verification

- [ ] **Step 1: Start the full stack**

```bash
docker compose up -d
```

Wait ~30s for all services to be healthy.

- [ ] **Step 2: Open Jaeger UI**

Navigate to `http://localhost:16686`.

Expected: you see 5 services listed: `jobradar-api`, `jobradar-scheduler`, `jobradar-crawler`, `jobradar-filter`, `jobradar-enricher`.

- [ ] **Step 3: Trigger an API request and find its trace**

```bash
curl http://localhost:8080/jobs
```

In Jaeger UI, select service `jobradar-api`, click **Find Traces**. You should see a trace for `GET /jobs` with child spans:

- `job.service.List`
- `job.store.GetJobs`
- `SELECT job_feed ...` (automatic from GORM plugin)

- [ ] **Step 4: Trigger a crawler scan and find the cross-service trace**

```bash
curl -X POST http://localhost:8080/companies/{any-company-id}/scan
```

Wait ~60s for the pipeline to process. In Jaeger UI, select `jobradar-crawler`, find a recent trace. You should see spans crossing into `jobradar-filter` and `jobradar-enricher` — one unified trace for a job's full journey.

- [ ] **Step 5: Verify Prometheus is scraping**

Navigate to `http://localhost:9090/targets`. All 5 services should show state `UP`.

- [ ] **Step 6: Final commit**

```bash
git add .
git commit -m "feat: complete OTel + Jaeger distributed tracing across all services"
```
