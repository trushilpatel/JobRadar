# Pipeline Retry Visibility Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add retry-backlog visibility to the admin pipeline strip — two new nodes (Filter Retrying, Enricher Retrying) showing per-tier counts with a hover tooltip and per-tier Retry button that force-drains jobs back to the main topic.

**Architecture:** Extend the existing `PipelineStats` DTO with `filter_retries` and `enricher_retries` fields populated via `LagReader.GetLag` against all 8 retry/DLQ topics. A new `RetryDrainer` Kafka primitive consumes uncommitted messages from a retry topic and republishes them (stripping `next_attempt_at`) to the main topic. A new `POST /admin/dashboard/pipeline/retry` endpoint delegates to `PipelineService.TriggerRetry`. Frontend adds a `RetryStage` component with a Radix Tooltip containing per-tier rows and Retry buttons.

**Tech Stack:** Go (franz-go kadm + kgo), Chi router, React + TanStack Query + Radix Tooltip + Sonner toasts

---

## File Map

| Action | File |
|--------|------|
| Modify | `backend/service/dashboard/dto/Pipeline.go` |
| Modify | `backend/service/dashboard/services/DashboardPipelineService.go` |
| Modify | `backend/service/dashboard/services/DashboardPipelineImplService.go` |
| Modify | `backend/service/dashboard/services/DashboardPipelineRestService.go` |
| Modify | `backend/service/dashboard/services/DashboardPipelineRestRouter.go` |
| Modify | `backend/service/dashboard/tests/DashboardPipelineImplService_test.go` |
| Create | `backend/common/kafka/retry_drainer.go` |
| Modify | `backend/apps/api/wire.go` |
| Modify | `backend/apps/api/wire_gen.go` |
| Modify | `ui/console/src/types/index.ts` |
| Modify | `ui/console/src/api/dashboard.ts` |
| Create | `ui/console/src/features/companies/hooks/useRetryTier.ts` |
| Create | `ui/console/src/features/companies/components/RetryStage.tsx` |
| Modify | `ui/console/src/features/companies/components/PipelineStrip.tsx` |

---

## Task 1: DTO — Add RetryTierCounts to PipelineStats

**Files:**
- Modify: `backend/service/dashboard/dto/Pipeline.go`

- [ ] **Step 1: Update Pipeline.go**

Replace the entire file:

```go
package dto

// RetryTierCounts holds the per-tier retry backlog counts for one worker.
type RetryTierCounts struct {
	Tier1m  int64 `json:"1m"`
	Tier10m int64 `json:"10m"`
	Tier1h  int64 `json:"1h"`
	DLQ     int64 `json:"dlq"`
}

// TriggerRetryRequest is the body for POST /admin/dashboard/pipeline/retry.
type TriggerRetryRequest struct {
	Worker string `json:"worker"` // "filter" or "enricher"
	Tier   string `json:"tier"`   // "1m", "10m", "1h", "dlq"
}

// TriggerRetryResponse is the response for POST /admin/dashboard/pipeline/retry.
type TriggerRetryResponse struct {
	Drained int64 `json:"drained"`
}

// PipelineStats is the full response returned by GET /dashboard/pipeline.
type PipelineStats struct {
	TotalCompanies     int64           `json:"total_companies"`
	ActiveCompanies    int64           `json:"active_companies"`
	Queued             int64           `json:"queued"`
	Crawling           int64           `json:"crawling"`
	AwaitingFilter     int64           `json:"awaiting_filter"`
	FilterRetries      RetryTierCounts `json:"filter_retries"`
	AwaitingEnrichment int64           `json:"awaiting_enrichment"`
	EnricherRetries    RetryTierCounts `json:"enricher_retries"`
	NewJobs24h         int64           `json:"new_jobs_24h"`
	Errors24h          int64           `json:"errors_24h"`
}

// PipelineDBStats holds the DB-sourced counts for the pipeline strip.
type PipelineDBStats struct {
	TotalCompanies  int64 `gorm:"column:total_companies"`
	ActiveCompanies int64 `gorm:"column:active_companies"`
	Queued          int64 `gorm:"column:queued"`
	Crawling        int64 `gorm:"column:crawling"`
	NewJobs24h      int64 `gorm:"column:new_jobs_24h"`
	Errors24h       int64 `gorm:"column:errors_24h"`
	// PipelinePending: jobs crawled (reason='fetch') but not yet written to `jobs`.
	// Used as the DB-based proxy for awaiting_filter when Kafka lag is unavailable.
	PipelinePending int64 `gorm:"column:pipeline_pending"`
}
```

- [ ] **Step 2: Verify compilation**

```bash
cd backend && go build ./service/dashboard/...
```
Expected: no errors.

- [ ] **Step 3: Commit**

```bash
git add backend/service/dashboard/dto/Pipeline.go
git commit -m "feat(dashboard): add RetryTierCounts to PipelineStats DTO"
```

---

## Task 2: Backend — RetryDrainer Kafka primitive

**Files:**
- Create: `backend/common/kafka/retry_drainer.go`

- [ ] **Step 1: Create retry_drainer.go**

```go
package kafka

import (
	"context"
	"fmt"
	"strings"

	"github.com/trushilpatel/jobradar/common/config"
	"github.com/twmb/franz-go/pkg/kadm"
	"github.com/twmb/franz-go/pkg/kgo"
	"github.com/twmb/franz-go/pkg/sasl/plain"
)

// RetryDrainer consumes all uncommitted messages from a retry topic and
// republishes them to destTopic, stripping the next_attempt_at header so the
// worker processes them immediately.
type RetryDrainer interface {
	Drain(ctx context.Context, consumerGroupID, sourceTopic, destTopic string) (int64, error)
}

type retryDrainer struct {
	admin *kadm.Client
	cfg   config.KafkaConfig
}

// NewRetryDrainer creates a RetryDrainer backed by a Kafka admin client.
func NewRetryDrainer(cfg config.KafkaConfig) (RetryDrainer, error) {
	client, err := kgo.NewClient(brokerOpts(cfg)...)
	if err != nil {
		return nil, fmt.Errorf("kafka retry drainer: create client: %w", err)
	}
	return &retryDrainer{admin: kadm.NewClient(client), cfg: cfg}, nil
}

func brokerOpts(cfg config.KafkaConfig) []kgo.Opt {
	opts := []kgo.Opt{kgo.SeedBrokers(strings.Split(cfg.Brokers, ",")...)}
	if cfg.User != "" && cfg.Password != "" {
		opts = append(opts, kgo.SASL(plain.Auth{User: cfg.User, Pass: cfg.Password}.AsMechanism()))
	}
	return opts
}

// Drain reads all uncommitted messages in sourceTopic (from the committed
// offset of consumerGroupID to the current end offset) and publishes each to
// destTopic. The next_attempt_at header is stripped so workers process them
// immediately. Returns the number of records drained.
//
// For DLQ topics (no regular consumer group), pass a non-existent groupID
// (e.g. "jobradar-dlq-observer") — GetLag treats all messages as unconsumed.
func (d *retryDrainer) Drain(ctx context.Context, consumerGroupID, sourceTopic, destTopic string) (int64, error) {
	endOffsets, err := d.admin.ListEndOffsets(ctx, sourceTopic)
	if err != nil {
		return 0, fmt.Errorf("list end offsets for %q: %w", sourceTopic, err)
	}

	// Committed offsets tell us where to start. Errors treated as no commits
	// (start from earliest available offset).
	committed, _ := d.admin.FetchOffsets(ctx, consumerGroupID)

	type partRange struct{ end int64 }
	ranges := map[int32]partRange{}
	partOffsets := map[string]map[int32]kgo.Offset{sourceTopic: {}}
	var totalMessages int64

	for partition, endOff := range endOffsets[sourceTopic] {
		if endOff.Err != nil {
			continue
		}
		startAt := committed[sourceTopic][partition].At
		if startAt < 0 {
			startAt = 0
		}
		remaining := endOff.Offset - startAt
		if remaining <= 0 {
			continue
		}
		ranges[partition] = partRange{end: endOff.Offset}
		partOffsets[sourceTopic][partition] = kgo.NewOffset().At(startAt)
		totalMessages += remaining
	}

	if totalMessages == 0 {
		return 0, nil
	}

	// Direct partition assignment — does not interfere with existing retry consumers.
	consumer, err := kgo.NewClient(append(brokerOpts(d.cfg), kgo.ConsumePartitions(partOffsets))...)
	if err != nil {
		return 0, fmt.Errorf("create drain consumer: %w", err)
	}
	defer consumer.Close()

	publisher, err := kgo.NewClient(brokerOpts(d.cfg)...)
	if err != nil {
		return 0, fmt.Errorf("create drain publisher: %w", err)
	}
	defer publisher.Close()

	var drained int64
	for drained < totalMessages {
		fetches := consumer.PollFetches(ctx)
		if err := fetches.Err(); err != nil {
			return drained, fmt.Errorf("poll: %w", err)
		}

		var toPublish []*kgo.Record
		fetches.EachRecord(func(r *kgo.Record) {
			end, ok := ranges[r.Partition]
			if !ok || r.Offset >= end.end {
				return
			}
			headers := make([]kgo.RecordHeader, 0, len(r.Headers))
			for _, h := range r.Headers {
				if string(h.Key) != HeaderNextAttemptAt {
					headers = append(headers, h)
				}
			}
			toPublish = append(toPublish, &kgo.Record{
				Topic:   destTopic,
				Value:   r.Value,
				Headers: headers,
			})
			drained++
		})

		if len(toPublish) > 0 {
			if err := publisher.ProduceSync(ctx, toPublish...).FirstErr(); err != nil {
				return drained, fmt.Errorf("produce: %w", err)
			}
		}
	}

	return drained, nil
}
```

- [ ] **Step 2: Verify compilation**

```bash
cd backend && go build ./common/kafka/...
```
Expected: no errors.

- [ ] **Step 3: Commit**

```bash
git add backend/common/kafka/retry_drainer.go
git commit -m "feat(kafka): add RetryDrainer for force-draining retry topics"
```

---

## Task 3: Backend — PipelineService retry counts + TriggerRetry endpoint

**Files:**
- Modify: `backend/service/dashboard/services/DashboardPipelineService.go`
- Modify: `backend/service/dashboard/services/DashboardPipelineImplService.go`
- Modify: `backend/service/dashboard/services/DashboardPipelineRestService.go`
- Modify: `backend/service/dashboard/services/DashboardPipelineRestRouter.go`
- Modify: `backend/apps/api/wire.go`
- Modify: `backend/apps/api/wire_gen.go`

- [ ] **Step 1: Update DashboardPipelineService.go — add TriggerRetry to interface and base**

Replace the entire file:

```go
package services

import (
	"context"

	"github.com/trushilpatel/jobradar/service/dashboard/dto"
)

// PipelineService assembles DB stats and Kafka lag into PipelineStats and
// handles force-retry drain requests.
type PipelineService interface {
	GetPipelineStats(ctx context.Context) (*dto.PipelineStats, error)
	TriggerRetry(ctx context.Context, worker, tier string) (int64, error)
}

// PipelineBaseService delegates all PipelineService methods to next.
type PipelineBaseService struct{ next PipelineService }

// NewPipelineBaseService constructs a PipelineBaseService.
func NewPipelineBaseService(next PipelineService) PipelineBaseService {
	if next == nil {
		panic("PipelineBaseService: next must not be nil")
	}
	return PipelineBaseService{next: next}
}

func (b *PipelineBaseService) GetPipelineStats(ctx context.Context) (*dto.PipelineStats, error) {
	return b.next.GetPipelineStats(ctx)
}

func (b *PipelineBaseService) TriggerRetry(ctx context.Context, worker, tier string) (int64, error) {
	return b.next.TriggerRetry(ctx, worker, tier)
}
```

- [ ] **Step 2: Update DashboardPipelineImplService.go — retry counts + TriggerRetry**

Replace the entire file:

```go
package services

import (
	"context"

	"github.com/trushilpatel/jobradar/common/errors"
	"github.com/trushilpatel/jobradar/common/kafka"
	"github.com/trushilpatel/jobradar/service/dashboard/dto"
)

// retryTarget maps (worker, tier) to the Kafka group + topics for draining.
type retryTarget struct {
	groupID string
	src     string
	dst     string
}

// retryTargets is keyed by worker → tier → target.
var retryTargets = map[string]map[string]retryTarget{
	"filter": {
		"1m":  {"job-listing-relevance-filter-retry-1m", kafka.TopicJobListingsRawRetry1m, kafka.TopicJobListingsRaw},
		"10m": {"job-listing-relevance-filter-retry-10m", kafka.TopicJobListingsRawRetry10m, kafka.TopicJobListingsRaw},
		"1h":  {"job-listing-relevance-filter-retry-1h", kafka.TopicJobListingsRawRetry1h, kafka.TopicJobListingsRaw},
		"dlq": {"jobradar-dlq-observer", kafka.TopicJobListingsRawDLQ, kafka.TopicJobListingsRaw},
	},
	"enricher": {
		"1m":  {"job-post-analyzer-retry-1m", kafka.TopicJobListingsFilteredRetry1m, kafka.TopicJobListingsFiltered},
		"10m": {"job-post-analyzer-retry-10m", kafka.TopicJobListingsFilteredRetry10m, kafka.TopicJobListingsFiltered},
		"1h":  {"job-post-analyzer-retry-1h", kafka.TopicJobListingsFilteredRetry1h, kafka.TopicJobListingsFiltered},
		"dlq": {"jobradar-dlq-observer", kafka.TopicJobListingsFilteredDLQ, kafka.TopicJobListingsFiltered},
	},
}

// PipelineImplService implements PipelineService.
type PipelineImplService struct {
	store   PipelineStore
	lag     kafka.LagReader
	drainer kafka.RetryDrainer
}

// NewPipelineService constructs a PipelineService.
func NewPipelineService(store PipelineStore, lag kafka.LagReader, drainer kafka.RetryDrainer) PipelineService {
	return &PipelineImplService{store: store, lag: lag, drainer: drainer}
}

func (s *PipelineImplService) GetPipelineStats(ctx context.Context) (*dto.PipelineStats, error) {
	dbStats, err := s.store.GetPipelineDBStats(ctx)
	if err != nil {
		return nil, errors.ErrInternal.WithMessage("failed to fetch pipeline stats")
	}

	awaitingFilter := dbStats.PipelinePending

	awaitingEnrichment, err := s.lag.GetLag(ctx, "job-post-analyzer", kafka.TopicJobListingsFiltered)
	if err != nil {
		awaitingEnrichment = 0
	}

	getLag := func(groupID, topic string) int64 {
		n, _ := s.lag.GetLag(ctx, groupID, topic)
		return n
	}

	return &dto.PipelineStats{
		TotalCompanies:     dbStats.TotalCompanies,
		ActiveCompanies:    dbStats.ActiveCompanies,
		Queued:             dbStats.Queued,
		Crawling:           dbStats.Crawling,
		AwaitingFilter:     awaitingFilter,
		FilterRetries: dto.RetryTierCounts{
			Tier1m:  getLag("job-listing-relevance-filter-retry-1m", kafka.TopicJobListingsRawRetry1m),
			Tier10m: getLag("job-listing-relevance-filter-retry-10m", kafka.TopicJobListingsRawRetry10m),
			Tier1h:  getLag("job-listing-relevance-filter-retry-1h", kafka.TopicJobListingsRawRetry1h),
			DLQ:     getLag("jobradar-dlq-observer", kafka.TopicJobListingsRawDLQ),
		},
		AwaitingEnrichment: awaitingEnrichment,
		EnricherRetries: dto.RetryTierCounts{
			Tier1m:  getLag("job-post-analyzer-retry-1m", kafka.TopicJobListingsFilteredRetry1m),
			Tier10m: getLag("job-post-analyzer-retry-10m", kafka.TopicJobListingsFilteredRetry10m),
			Tier1h:  getLag("job-post-analyzer-retry-1h", kafka.TopicJobListingsFilteredRetry1h),
			DLQ:     getLag("jobradar-dlq-observer", kafka.TopicJobListingsFilteredDLQ),
		},
		NewJobs24h: dbStats.NewJobs24h,
		Errors24h:  dbStats.Errors24h,
	}, nil
}

func (s *PipelineImplService) TriggerRetry(ctx context.Context, worker, tier string) (int64, error) {
	workers, ok := retryTargets[worker]
	if !ok {
		return 0, errors.ErrBadRequest.WithMessage("unknown worker: " + worker)
	}
	target, ok := workers[tier]
	if !ok {
		return 0, errors.ErrBadRequest.WithMessage("unknown tier: " + tier)
	}
	n, err := s.drainer.Drain(ctx, target.groupID, target.src, target.dst)
	if err != nil {
		return 0, errors.ErrInternal.WithMessage("drain failed: " + err.Error())
	}
	return n, nil
}
```

- [ ] **Step 3: Update DashboardPipelineRestService.go — add triggerRetry handler**

Replace the entire file:

```go
package services

import (
	"encoding/json"
	"net/http"

	"github.com/trushilpatel/jobradar/apps/api/handler"
	"github.com/trushilpatel/jobradar/common/errors"
	"github.com/trushilpatel/jobradar/service/dashboard/dto"
)

// PipelineRestService is the HTTP adapter for the admin pipeline dashboard.
type PipelineRestService struct {
	next PipelineService
}

// NewPipelineRestService constructs the pipeline HTTP adapter.
func NewPipelineRestService(next PipelineService) *PipelineRestService {
	if next == nil {
		panic("dashboard.NewPipelineRestService: next must not be nil")
	}
	return &PipelineRestService{next: next}
}

// getPipelineStats handles GET /admin/dashboard/pipeline.
func (h *PipelineRestService) getPipelineStats(w http.ResponseWriter, r *http.Request) {
	stats, err := h.next.GetPipelineStats(r.Context())
	if err != nil {
		handler.HandleError(w, err)
		return
	}
	handler.Respond(w, http.StatusOK, stats)
}

// triggerRetry handles POST /admin/dashboard/pipeline/retry.
func (h *PipelineRestService) triggerRetry(w http.ResponseWriter, r *http.Request) {
	var req dto.TriggerRetryRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		handler.HandleError(w, errors.ErrBadRequest.WithMessage("invalid request body"))
		return
	}
	n, err := h.next.TriggerRetry(r.Context(), req.Worker, req.Tier)
	if err != nil {
		handler.HandleError(w, err)
		return
	}
	handler.Respond(w, http.StatusOK, dto.TriggerRetryResponse{Drained: n})
}
```

- [ ] **Step 4: Update DashboardPipelineRestRouter.go — add POST /retry route**

Replace the entire file:

```go
package services

import "github.com/go-chi/chi/v5"

// PipelineRestRouter mounts the admin pipeline dashboard routes under /admin.
type PipelineRestRouter struct {
	handler *PipelineRestService
}

// NewPipelineRestRouter constructs the pipeline admin RestRouter.
func NewPipelineRestRouter(h *PipelineRestService) *PipelineRestRouter {
	return &PipelineRestRouter{handler: h}
}

// Register mounts pipeline routes onto the supplied router.
func (rr *PipelineRestRouter) Register(router chi.Router) {
	router.Get("/dashboard/pipeline", rr.handler.getPipelineStats)
	router.Post("/dashboard/pipeline/retry", rr.handler.triggerRetry)
}
```

- [ ] **Step 5: Add provideRetryDrainer to wire.go**

Add this function to `backend/apps/api/wire.go` after `provideLagReader`:

```go
// provideRetryDrainer creates a Kafka RetryDrainer from the config.
func provideRetryDrainer(cfg *config.Config) (kafka.RetryDrainer, error) {
	return kafka.NewRetryDrainer(cfg.Kafka)
}
```

Also add `provideRetryDrainer` to the `wire.Build(...)` call in `InitializeApp`.

- [ ] **Step 6: Update wire_gen.go — wire RetryDrainer into PipelineService**

In `wire_gen.go`, find:
```go
lagReader, err := provideLagReader(cfg)
if err != nil {
    return nil, err
}
pipelineService := services4.NewPipelineService(pipelineStore, lagReader)
```

Replace with:
```go
lagReader, err := provideLagReader(cfg)
if err != nil {
    return nil, err
}
retryDrainer, err := provideRetryDrainer(cfg)
if err != nil {
    return nil, err
}
pipelineService := services4.NewPipelineService(pipelineStore, lagReader, retryDrainer)
```

- [ ] **Step 7: Verify compilation**

```bash
cd backend && go build ./...
```
Expected: no errors.

- [ ] **Step 8: Commit**

```bash
git add backend/service/dashboard/services/DashboardPipelineService.go \
        backend/service/dashboard/services/DashboardPipelineImplService.go \
        backend/service/dashboard/services/DashboardPipelineRestService.go \
        backend/service/dashboard/services/DashboardPipelineRestRouter.go \
        backend/apps/api/wire.go \
        backend/apps/api/wire_gen.go
git commit -m "feat(dashboard): add retry counts to pipeline stats and TriggerRetry endpoint"
```

---

## Task 4: Tests — Update DashboardPipelineImplService_test.go

**Files:**
- Modify: `backend/service/dashboard/tests/DashboardPipelineImplService_test.go`

- [ ] **Step 1: Add fakeDrainer test double and update existing tests**

Replace the entire file:

```go
package dashboard_test

import (
	"context"
	"errors"
	"testing"

	"github.com/trushilpatel/jobradar/service/dashboard/dto"
	"github.com/trushilpatel/jobradar/service/dashboard/services"
)

// Ensure test doubles implement interfaces.
var _ services.PipelineStore = (*fakePipelineStore)(nil)

// fakeLagReader is a test double for kafka.LagReader.
// lags maps groupID → lag value.
type fakeLagReader struct {
	lags map[string]int64
	err  error
}

func (f *fakeLagReader) GetLag(_ context.Context, groupID, _ string) (int64, error) {
	return f.lags[groupID], f.err
}

// fakeDrainer is a test double for kafka.RetryDrainer.
type fakeDrainer struct {
	drained int64
	err     error
}

func (f *fakeDrainer) Drain(_ context.Context, _, _, _ string) (int64, error) {
	return f.drained, f.err
}

// fakePipelineStore is a test double for PipelineStore.
type fakePipelineStore struct {
	stats dto.PipelineDBStats
	err   error
}

func (f *fakePipelineStore) GetPipelineDBStats(_ context.Context) (dto.PipelineDBStats, error) {
	return f.stats, f.err
}

func TestPipelineService_GetPipelineStats_OK(t *testing.T) {
	svc := services.NewPipelineService(
		&fakePipelineStore{stats: dto.PipelineDBStats{
			TotalCompanies:  10,
			ActiveCompanies: 8,
			Queued:          2,
			Crawling:        1,
			NewJobs24h:      5,
			Errors24h:       0,
			PipelinePending: 12,
		}},
		&fakeLagReader{lags: map[string]int64{
			"job-post-analyzer":                        3,
			"job-listing-relevance-filter-retry-1m":    4,
			"job-listing-relevance-filter-retry-10m":   5,
			"job-listing-relevance-filter-retry-1h":    6,
			"jobradar-dlq-observer":                    1,
			"job-post-analyzer-retry-1m":               2,
			"job-post-analyzer-retry-10m":              3,
			"job-post-analyzer-retry-1h":               1,
		}},
		&fakeDrainer{},
	)

	stats, err := svc.GetPipelineStats(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if stats.TotalCompanies != 10 {
		t.Errorf("TotalCompanies = %d, want 10", stats.TotalCompanies)
	}
	if stats.AwaitingFilter != 12 {
		t.Errorf("AwaitingFilter = %d, want 12 (pipeline_pending from DB)", stats.AwaitingFilter)
	}
	if stats.AwaitingEnrichment != 3 {
		t.Errorf("AwaitingEnrichment = %d, want 3", stats.AwaitingEnrichment)
	}
	if stats.FilterRetries.Tier1m != 4 {
		t.Errorf("FilterRetries.Tier1m = %d, want 4", stats.FilterRetries.Tier1m)
	}
	if stats.FilterRetries.Tier10m != 5 {
		t.Errorf("FilterRetries.Tier10m = %d, want 5", stats.FilterRetries.Tier10m)
	}
	if stats.FilterRetries.Tier1h != 6 {
		t.Errorf("FilterRetries.Tier1h = %d, want 6", stats.FilterRetries.Tier1h)
	}
	if stats.FilterRetries.DLQ != 1 {
		t.Errorf("FilterRetries.DLQ = %d, want 1", stats.FilterRetries.DLQ)
	}
	if stats.EnricherRetries.Tier1m != 2 {
		t.Errorf("EnricherRetries.Tier1m = %d, want 2", stats.EnricherRetries.Tier1m)
	}
	if stats.NewJobs24h != 5 {
		t.Errorf("NewJobs24h = %d, want 5", stats.NewJobs24h)
	}
}

func TestPipelineService_GetPipelineStats_KafkaError(t *testing.T) {
	svc := services.NewPipelineService(
		&fakePipelineStore{stats: dto.PipelineDBStats{TotalCompanies: 5}},
		&fakeLagReader{lags: nil, err: errors.New("kafka unavailable")},
		&fakeDrainer{},
	)

	stats, err := svc.GetPipelineStats(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if stats.AwaitingEnrichment != 0 {
		t.Errorf("AwaitingEnrichment = %d, want 0 on kafka error", stats.AwaitingEnrichment)
	}
	if stats.FilterRetries.Tier1m != 0 {
		t.Errorf("FilterRetries.Tier1m = %d, want 0 on kafka error", stats.FilterRetries.Tier1m)
	}
}

func TestPipelineService_GetPipelineStats_StoreError(t *testing.T) {
	svc := services.NewPipelineService(
		&fakePipelineStore{err: errors.New("db down")},
		&fakeLagReader{},
		&fakeDrainer{},
	)

	_, err := svc.GetPipelineStats(context.Background())
	if err == nil {
		t.Fatal("expected error from store failure, got nil")
	}
}

func TestPipelineService_TriggerRetry_OK(t *testing.T) {
	svc := services.NewPipelineService(
		&fakePipelineStore{},
		&fakeLagReader{},
		&fakeDrainer{drained: 42},
	)

	n, err := svc.TriggerRetry(context.Background(), "filter", "1m")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n != 42 {
		t.Errorf("drained = %d, want 42", n)
	}
}

func TestPipelineService_TriggerRetry_UnknownWorker(t *testing.T) {
	svc := services.NewPipelineService(&fakePipelineStore{}, &fakeLagReader{}, &fakeDrainer{})

	_, err := svc.TriggerRetry(context.Background(), "unknown", "1m")
	if err == nil {
		t.Fatal("expected error for unknown worker, got nil")
	}
}

func TestPipelineService_TriggerRetry_UnknownTier(t *testing.T) {
	svc := services.NewPipelineService(&fakePipelineStore{}, &fakeLagReader{}, &fakeDrainer{})

	_, err := svc.TriggerRetry(context.Background(), "filter", "99m")
	if err == nil {
		t.Fatal("expected error for unknown tier, got nil")
	}
}

func TestPipelineService_TriggerRetry_DrainerError(t *testing.T) {
	svc := services.NewPipelineService(
		&fakePipelineStore{},
		&fakeLagReader{},
		&fakeDrainer{err: errors.New("kafka unavailable")},
	)

	_, err := svc.TriggerRetry(context.Background(), "enricher", "10m")
	if err == nil {
		t.Fatal("expected error from drainer failure, got nil")
	}
}
```

- [ ] **Step 2: Run tests**

```bash
cd backend && go test ./service/dashboard/... -v
```
Expected: all tests PASS.

- [ ] **Step 3: Run backend lint**

```bash
cd backend && golangci-lint run --config ../.github/.golangci.yml --timeout 5m ./service/dashboard/... ./common/kafka/... ./apps/api/...
```
Expected: no lint errors.

- [ ] **Step 4: Commit**

```bash
git add backend/service/dashboard/tests/DashboardPipelineImplService_test.go
git commit -m "test(dashboard): update pipeline service tests for retry counts and TriggerRetry"
```

---

## Task 5: Frontend — Types and API

**Files:**
- Modify: `ui/console/src/types/index.ts`
- Modify: `ui/console/src/api/dashboard.ts`

- [ ] **Step 1: Add RetryTierCounts and update PipelineStats in types/index.ts**

Find the `PipelineStats` interface (line 237) and add `RetryTierCounts` before it, then update `PipelineStats`:

```typescript
export interface RetryTierCounts {
  '1m': number
  '10m': number
  '1h': number
  dlq: number
}

export interface PipelineStats {
  total_companies: number
  active_companies: number
  queued: number
  crawling: number
  awaiting_filter: number
  filter_retries: RetryTierCounts
  awaiting_enrichment: number
  enricher_retries: RetryTierCounts
  new_jobs_24h: number
  errors_24h: number
}
```

- [ ] **Step 2: Update fetchPipelineStats and add triggerRetry in api/dashboard.ts**

Update the `fetchPipelineStats` function to include retry defaults:

```typescript
export async function fetchPipelineStats(): Promise<PipelineStats> {
  // Phase 2: pipeline moved to /admin/dashboard/pipeline (admin-only)
  const data = await apiClient.get<Partial<PipelineStats> | null>('/admin/dashboard/pipeline')
  const emptyRetries: RetryTierCounts = { '1m': 0, '10m': 0, '1h': 0, dlq: 0 }
  return {
    total_companies: data?.total_companies ?? 0,
    active_companies: data?.active_companies ?? 0,
    queued: data?.queued ?? 0,
    crawling: data?.crawling ?? 0,
    awaiting_filter: data?.awaiting_filter ?? 0,
    filter_retries: data?.filter_retries ?? emptyRetries,
    awaiting_enrichment: data?.awaiting_enrichment ?? 0,
    enricher_retries: data?.enricher_retries ?? emptyRetries,
    new_jobs_24h: data?.new_jobs_24h ?? 0,
    errors_24h: data?.errors_24h ?? 0,
  }
}
```

Add `RetryTierCounts` to the imports at the top of `dashboard.ts`:

```typescript
import { ..., RetryTierCounts } from '@/types'
```

Add `triggerRetry` at the end of `dashboard.ts`:

```typescript
export async function triggerRetry(
  worker: 'filter' | 'enricher',
  tier: '1m' | '10m' | '1h' | 'dlq'
): Promise<{ drained: number }> {
  return apiClient.post<{ drained: number }>('/admin/dashboard/pipeline/retry', { worker, tier })
}
```

- [ ] **Step 3: Verify TypeScript**

```bash
cd ui/console && npm run lint
```
Expected: no errors.

- [ ] **Step 4: Commit**

```bash
git add ui/console/src/types/index.ts ui/console/src/api/dashboard.ts
git commit -m "feat(ui): add RetryTierCounts type and triggerRetry API function"
```

---

## Task 6: Frontend — RetryStage component and PipelineStrip update

**Files:**
- Create: `ui/console/src/features/companies/hooks/useRetryTier.ts`
- Create: `ui/console/src/features/companies/components/RetryStage.tsx`
- Modify: `ui/console/src/features/companies/components/PipelineStrip.tsx`

- [ ] **Step 1: Create useRetryTier.ts**

```typescript
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { toast } from 'sonner'
import { triggerRetry } from '@/api/dashboard'

export function useRetryTier() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({
      worker,
      tier,
    }: {
      worker: 'filter' | 'enricher'
      tier: '1m' | '10m' | '1h' | 'dlq'
    }) => triggerRetry(worker, tier),
    onSuccess: (data) => {
      qc.invalidateQueries({ queryKey: ['pipeline-stats'] })
      toast.success(`Retrying ${data.drained} job${data.drained === 1 ? '' : 's'}`)
    },
    onError: () => {
      toast.error('Failed to trigger retry')
    },
  })
}
```

- [ ] **Step 2: Create RetryStage.tsx**

```tsx
import * as Tooltip from '@radix-ui/react-tooltip'
import { cn } from '@/lib/utils'
import { RetryTierCounts } from '@/types'
import { useRetryTier } from '../hooks/useRetryTier'

const TIERS: Array<{ key: keyof RetryTierCounts; label: string }> = [
  { key: '1m', label: '1m' },
  { key: '10m', label: '10m' },
  { key: '1h', label: '1h' },
  { key: 'dlq', label: 'DLQ' },
]

interface RetryStageProps {
  label: string
  worker: 'filter' | 'enricher'
  counts: RetryTierCounts
}

export function RetryStage({ label, worker, counts }: RetryStageProps) {
  const { mutate: retry, isPending } = useRetryTier()
  const total = counts['1m'] + counts['10m'] + counts['1h'] + counts.dlq

  return (
    <Tooltip.Provider delayDuration={200}>
      <Tooltip.Root>
        <Tooltip.Trigger asChild>
          <div className="flex flex-col items-center min-w-[70px] cursor-default select-none">
            <span
              className={cn(
                'text-[18px] font-semibold leading-tight tabular-nums',
                total === 0 ? 'text-white/25' : 'text-amber-400'
              )}
            >
              {total}
            </span>
            <span
              className={cn(
                'text-[10px] uppercase tracking-[0.08em] mt-0.5 text-center whitespace-nowrap',
                total === 0 ? 'text-slate-700' : 'text-slate-500'
              )}
            >
              {label}
            </span>
          </div>
        </Tooltip.Trigger>
        <Tooltip.Portal>
          <Tooltip.Content
            className="bg-slate-800 border border-slate-700 rounded-lg p-3 shadow-xl z-50 min-w-[170px]"
            sideOffset={8}
          >
            <div className="space-y-1.5">
              {TIERS.map(({ key, label: tierLabel }) => (
                <div key={key} className="flex items-center gap-2">
                  <span className="text-slate-400 text-xs font-mono w-8 shrink-0">{tierLabel}</span>
                  <span className="text-white text-xs tabular-nums flex-1 text-right">
                    {counts[key]}
                  </span>
                  <button
                    disabled={counts[key] === 0 || isPending}
                    onClick={() => retry({ worker, tier: key })}
                    className={cn(
                      'text-[10px] px-1.5 py-0.5 rounded border transition-colors shrink-0',
                      counts[key] === 0 || isPending
                        ? 'border-slate-700 text-slate-600 cursor-not-allowed'
                        : 'border-violet-500/50 text-violet-400 hover:bg-violet-500/10 cursor-pointer'
                    )}
                  >
                    Retry
                  </button>
                </div>
              ))}
            </div>
            <Tooltip.Arrow className="fill-slate-800" />
          </Tooltip.Content>
        </Tooltip.Portal>
      </Tooltip.Root>
    </Tooltip.Provider>
  )
}
```

- [ ] **Step 3: Update PipelineStrip.tsx — insert two RetryStage nodes**

Replace the entire file:

```tsx
import { AlertCircle } from 'lucide-react'
import { cn } from '@/lib/utils'
import { usePipelineStats } from '../hooks/usePipelineStats'
import { RetryStage } from './RetryStage'

function Stage({
  label,
  value,
  valueClass,
}: {
  label: string
  value: number
  valueClass?: string
}) {
  return (
    <div className="flex flex-col items-center min-w-[70px]">
      <span
        className={cn(
          'text-[18px] font-semibold leading-tight tabular-nums',
          value === 0 ? 'text-white/25' : (valueClass ?? 'text-white')
        )}
      >
        {value}
      </span>
      <span
        className={cn(
          'text-[10px] uppercase tracking-[0.08em] mt-0.5 text-center whitespace-nowrap',
          value === 0 ? 'text-slate-700' : 'text-slate-500'
        )}
      >
        {label}
      </span>
    </div>
  )
}

function Arrow() {
  return <span className="text-slate-600 text-sm px-1.5 select-none">→</span>
}

function Skeleton() {
  return (
    <div className="flex items-center gap-2 animate-pulse">
      {[...Array(9)].map((_, i) => (
        <div key={i} className="flex items-center gap-1">
          <div className="w-14 h-8 bg-white/[0.04] rounded" />
          {i < 8 && <div className="w-4 h-3 bg-white/[0.02] rounded" />}
        </div>
      ))}
    </div>
  )
}

export function PipelineStrip() {
  const { data, isLoading, isError } = usePipelineStats()

  if (isLoading) return <Skeleton />
  if (isError || !data) {
    return (
      <div className="flex items-center gap-2 text-xs text-slate-500">
        <AlertCircle size={13} className="text-red-400/70" />
        Pipeline stats unavailable
      </div>
    )
  }

  return (
    <div className="flex items-center">
      {data.total_companies !== data.active_companies && (
        <>
          <Stage label="Total" value={data.total_companies} />
          <Arrow />
        </>
      )}
      <Stage label="Active" value={data.active_companies} valueClass="text-emerald-400" />
      <Arrow />
      <Stage label="Queued" value={data.queued} valueClass="text-amber-400" />
      <Arrow />
      <Stage label="Crawling" value={data.crawling} valueClass="text-blue-400" />
      <Arrow />
      <Stage label="Awaiting Filter" value={data.awaiting_filter} valueClass="text-violet-400" />
      <Arrow />
      <RetryStage label="Filter Retrying" worker="filter" counts={data.filter_retries} />
      <Arrow />
      <Stage
        label="Awaiting Enrichment"
        value={data.awaiting_enrichment}
        valueClass="text-violet-400"
      />
      <Arrow />
      <RetryStage label="Enricher Retrying" worker="enricher" counts={data.enricher_retries} />
      <Arrow />
      <Stage label="New Jobs (24h)" value={data.new_jobs_24h} valueClass="text-emerald-400" />
    </div>
  )
}
```

- [ ] **Step 4: Run lint**

```bash
cd ui/console && npm run lint
```
Expected: no errors.

- [ ] **Step 5: Commit**

```bash
git add ui/console/src/features/companies/hooks/useRetryTier.ts \
        ui/console/src/features/companies/components/RetryStage.tsx \
        ui/console/src/features/companies/components/PipelineStrip.tsx
git commit -m "feat(ui): add RetryStage to pipeline strip with per-tier hover and retry buttons"
```

---

## Task 7: Deploy and verify

- [ ] **Step 1: Build and import images**

```bash
make k8s-build
```

- [ ] **Step 2: Deploy**

```bash
kubectl config use-context k3d-jobradar
make k8s-up
```

- [ ] **Step 3: Verify backend endpoint returns retry counts**

```bash
# Get a valid admin JWT first, then:
curl -s -H "Authorization: Bearer <token>" http://localhost:30080/admin/dashboard/pipeline | jq '{filter_retries, enricher_retries}'
```
Expected: both fields present with numeric tier counts.

- [ ] **Step 4: Check pipeline strip in browser**

Open the Companies page. Confirm:
- "Filter Retrying" and "Enricher Retrying" nodes appear in the strip
- Hovering shows the 4-tier breakdown (1m / 10m / 1h / DLQ) with Retry buttons
- Retry buttons are disabled when count is 0
- Clicking an active Retry button shows a success toast and refreshes counts
