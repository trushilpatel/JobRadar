# Dashboard Quality Fixes Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Fix all P0/P1/P2 dashboard quality issues found by the quality-team review: cross-user data leak, broken stats, missing error states, dead code, and observability gaps.

**Architecture:** Fixes split into four independent parallel batches (A=backend DB/security, B=backend wiring/quality, C=frontend panels, D=frontend structure), followed by a test batch. All batches use the same branch `fix/dashboard-quality-overhaul`. Agents write to non-overlapping files; wire_gen.go is owned exclusively by Batch B.

**Tech Stack:** Go 1.23, GORM, chi router, OTel tracing, React 19, TypeScript, TanStack Query v5, Radix UI, Tailwind, sonner (toast), Vitest.

---

## BATCH A — Backend DB Security (independent, no wire_gen.go)

Files modified:

- `backend/service/dashboard/services/DashboardRepositoryService.go` (GetProgressStats SQL, GetTechStackQuarterlyAdoption merge, RefreshPipelineStats/RefreshHeatmap spans)
- `backend/service/dashboard/services/DashboardUserImplService.go` (userID extraction, GetInterviews ctx fix, local stage constants)
- `backend/migrations/068_dashboard_security_indexes.sql` (NEW — composite index + BRIN verification)

---

### Task A1: Fix GetProgressStats — user_id scoping + correct CTE logic

**Files:**

- Modify: `backend/service/dashboard/services/DashboardRepositoryService.go:12-19` (interface)
- Modify: `backend/service/dashboard/services/DashboardRepositoryService.go:30-72` (implementation)
- Modify: `backend/service/dashboard/services/DashboardUserImplService.go:26-34` (call site)

**Context:** The current SQL has two bugs:

1. `WHERE status = 'applied'` in the outer CTE pre-filters ALL rows to `applied`, so `interviewing_count` always equals `applied_count`
2. Neither CTE has `user_id = ?` — every user sees aggregate counts across ALL users

**Steps:**

- [ ] **A1.1** Update `UserDashboardStore` interface to add `userID string` to `GetProgressStats`:

In `DashboardRepositoryService.go`, change line 14:

```go
// BEFORE
GetProgressStats(ctx context.Context) (dto.ProgressStats, error)
// AFTER
GetProgressStats(ctx context.Context, userID string) (dto.ProgressStats, error)
```

- [ ] **A1.2** Rewrite `GetProgressStats` implementation with correct SQL:

Replace the entire `GetProgressStats` method body (`DashboardRepositoryService.go:30-72`):

```go
func (s *userDashboardRepository) GetProgressStats(ctx context.Context, userID string) (_ dto.ProgressStats, err error) {
	ctx, span := tracing.Start(ctx, "dashboard.store.GetProgressStats")
	defer tracing.End(span, &err)
	var result dto.ProgressStats
	err = s.db.WithContext(ctx).Raw(`
		WITH status_counts AS (
			SELECT COUNT(*) AS applied_count
			FROM user_job_state
			WHERE status = 'applied'
			  AND user_id = ?
		),
		tracker_stats AS (
			SELECT
				COUNT(DISTINCT ja.job_result_id) FILTER (
					WHERE ja.stage IN ('phone_screen', 'technical', 'final_round')
				) AS interviewing_count,
				COUNT(DISTINCT ja.job_result_id) FILTER (
					WHERE ja.stage = 'offer'
				) AS offer_count,
				COUNT(DISTINCT ja.job_result_id) FILTER (
					WHERE ja.stage IN ('phone_screen', 'technical', 'final_round')
					  AND ja.stage_at >= DATE_TRUNC('week', NOW())
				) AS interviewing_this_week,
				COUNT(DISTINCT ja.job_result_id) FILTER (
					WHERE advanced.job_result_id IS NULL
				) AS awaiting_reply
			FROM job_application_tracker ja
			LEFT JOIN (
				SELECT DISTINCT job_result_id
				FROM job_application_tracker
				WHERE stage != 'applied'
				  AND user_id = ?
			) advanced ON advanced.job_result_id = ja.job_result_id
			WHERE ja.user_id = ?
		)
		SELECT
			sc.applied_count,
			ts.interviewing_count,
			ts.offer_count,
			ts.awaiting_reply,
			ts.interviewing_this_week
		FROM status_counts sc, tracker_stats ts
	`, userID, userID, userID).Scan(&result).Error
	return result, err
}
```

- [ ] **A1.3** Update `GetProgressStats` call in `DashboardUserImplService.go:26-34`:

```go
func (s *userDashboardImplService) GetProgressStats(ctx context.Context) (_ *dto.ProgressStats, err error) {
	ctx, span := tracing.Start(ctx, "dashboard.userdashboard.GetProgressStats")
	defer tracing.End(span, &err)
	userID := identity.UserIDFrom(ctx)
	stats, err := s.store.GetProgressStats(ctx, userID)
	if err != nil {
		return nil, errors.ErrInternal.WithMessage("failed to fetch progress stats")
	}
	return &stats, nil
}
```

Add `identity` import at top of `DashboardUserImplService.go`:

```go
import (
    "context"
    "math"

    "github.com/trushilpatel/jobradar/common/errors"
    "github.com/trushilpatel/jobradar/common/identity"
    "github.com/trushilpatel/jobradar/common/tracing"
    "github.com/trushilpatel/jobradar/service/dashboard/dto"
    insightscommon "github.com/trushilpatel/jobradar/service/insights/common"
)
```

Note: remove `appcommon` import as it will be replaced in Task A3.

- [ ] **A1.4** Run backend tests to confirm the change compiles and existing tests pass:

```bash
cd backend && go build ./... && go test ./service/dashboard/... -count=1 -timeout 60s
```

Expected: build succeeds; tests that use a fake store will need a stub update (see A1.5).

- [ ] **A1.5** Update test fakes in `backend/service/dashboard/tests/` — any fake `UserDashboardStore` that implements `GetProgressStats` must add the `userID string` parameter:

```bash
grep -r "GetProgressStats" backend/service/dashboard/tests/
```

For each fake, add `userID string` parameter to match the new interface signature.

---

### Task A2: Fix GetTechStackQuarterlyAdoption — merge double scan

**Files:**

- Modify: `backend/service/dashboard/services/DashboardRepositoryService.go:150-191`

**Context:** The query has two independent CTEs that each scan `job_tech_tags` — `quarterly_counts` and `total_q4`. Merge them into one scan.

- [ ] **A2.1** Replace `GetTechStackQuarterlyAdoption` method body:

```go
func (s *userDashboardRepository) GetTechStackQuarterlyAdoption(ctx context.Context) (results []dto.TechAdoptionRow, err error) {
	ctx, span := tracing.Start(ctx, "dashboard.store.GetTechStackQuarterlyAdoption")
	defer tracing.End(span, &err)
	err = s.db.WithContext(ctx).Raw(`
		WITH quarterly_counts AS (
			SELECT
				tech,
				COUNT(*) FILTER (
					WHERE tagged_at >= DATE_TRUNC('quarter', NOW() - INTERVAL '9 months')
					  AND tagged_at <  DATE_TRUNC('quarter', NOW() - INTERVAL '6 months')
				) AS q1,
				COUNT(*) FILTER (
					WHERE tagged_at >= DATE_TRUNC('quarter', NOW() - INTERVAL '6 months')
					  AND tagged_at <  DATE_TRUNC('quarter', NOW() - INTERVAL '3 months')
				) AS q2,
				COUNT(*) FILTER (
					WHERE tagged_at >= DATE_TRUNC('quarter', NOW() - INTERVAL '3 months')
					  AND tagged_at <  DATE_TRUNC('quarter', NOW())
				) AS q3,
				COUNT(*) FILTER (
					WHERE tagged_at >= DATE_TRUNC('quarter', NOW())
				) AS q4,
				COUNT(DISTINCT job_id) FILTER (
					WHERE tagged_at >= DATE_TRUNC('quarter', NOW())
				) AS total_jobs_this_q
			FROM job_tech_tags
			WHERE tagged_at >= DATE_TRUNC('quarter', NOW() - INTERVAL '9 months')
			GROUP BY tech
		)
		SELECT
			qc.tech,
			qc.q1, qc.q2, qc.q3, qc.q4,
			COALESCE(SUM(qc.total_jobs_this_q) OVER (), 0)::bigint AS total_jobs
		FROM quarterly_counts qc
		WHERE qc.q4 > 0
		ORDER BY qc.q4 DESC
	`).Scan(&results).Error
	return results, err
}
```

Note: `SUM(qc.total_jobs_this_q) OVER ()` is a window function that gives the grand total across all tech rows in a single pass — same semantics as the old `CROSS JOIN total_q4`, one fewer scan.

---

### Task A3: Replace cross-domain stage constants with local map in DashboardUserImplService

**Files:**

- Modify: `backend/service/dashboard/services/DashboardUserImplService.go`

**Context:** `toDisplayStage` imports application domain constants (`appcommon.StageApplied` etc.) for display labels. Replace with a local map to remove the inter-domain import.

- [ ] **A3.1** Remove `appcommon` import and replace `toDisplayStage`:

```go
// displayStageMap maps DB stage keys to their UI display labels.
// Defined locally to avoid coupling to the application domain's constants.
var displayStageMap = map[string]string{
	"applied":     "Waiting",
	"phone_screen": "Interviewing",
	"technical":   "Interviewing",
	"final_round": "Interviewing",
	"offer":       "Offer received",
}

func toDisplayStage(stage string) string {
	if label, ok := displayStageMap[stage]; ok {
		return label
	}
	return stage
}
```

Remove `appcommon "github.com/trushilpatel/jobradar/service/application/common"` from imports.

---

### Task A4: Fix GetInterviews — context propagation bug

**Files:**

- Modify: `backend/service/dashboard/services/DashboardUserImplService.go:57-78`

**Context:** Line 58 has `_, span := tracing.Start(ctx, ...)` — the child context is discarded. The downstream `ListActive(ctx)` call uses the parent context, so spans from `ListActive` won't link to the `GetInterviews` parent span in Jaeger.

- [ ] **A4.1** Fix the context capture on line 58:

```go
func (s *userDashboardImplService) GetInterviews(ctx context.Context) (_ []dto.InterviewRow, err error) {
	ctx, span := tracing.Start(ctx, "dashboard.userdashboard.GetInterviews")  // ctx, not _
	defer tracing.End(span, &err)
	apps, err := s.appStore.ListActive(ctx)
	// ... rest unchanged
```

---

### Task A5: Add OTel spans to RefreshPipelineStats and RefreshHeatmap

**Files:**

- Modify: `backend/service/dashboard/services/DashboardRepositoryService.go:231-237`

**Context:** These two methods execute expensive `REFRESH MATERIALIZED VIEW CONCURRENTLY` operations but are invisible in traces.

- [ ] **A5.1** Add spans:

```go
func (r *dashboardRefresher) RefreshPipelineStats(ctx context.Context) (_ error, err error) {
	ctx, span := tracing.Start(ctx, "dashboard.store.RefreshPipelineStats")
	defer tracing.End(span, &err)
	return nil, r.db.WithContext(ctx).Exec(`REFRESH MATERIALIZED VIEW CONCURRENTLY mv_pipeline_db_stats`).Error
}

func (r *dashboardRefresher) RefreshHeatmap(ctx context.Context) (_ error, err error) {
	ctx, span := tracing.Start(ctx, "dashboard.store.RefreshHeatmap")
	defer tracing.End(span, &err)
	return nil, r.db.WithContext(ctx).Exec(`REFRESH MATERIALIZED VIEW CONCURRENTLY mv_weekly_hiring_activity`).Error
}
```

Note: The named return `(_ error, err error)` is needed for `tracing.End(span, &err)` to capture the returned error.

---

### Task A6: Add DB migration for composite index

**Files:**

- Create: `backend/migrations/068_dashboard_security_indexes.sql`

- [ ] **A6.1** Create migration file:

```sql
-- Migration 068: dashboard security — composite index on job_application_tracker
-- Needed by GetProgressStats after adding user_id scoping (fix/dashboard-quality-overhaul).
-- The existing idx_job_application_tracker_user (single-column) cannot cover
-- time-range predicates on stage_at. The composite index eliminates the residual scan.

CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_jat_user_stage_at
    ON job_application_tracker (user_id, stage_at DESC);

-- Verify BRIN index on jobs.added_at exists (may have been lost in table rename migration 044).
-- BRIN is preferred for sequential append-only tables; the existing B-tree
-- idx_jobs_added_at_id covers range scans but is heavier.
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_jobs_added_at_brin
    ON jobs USING BRIN (added_at);
```

---

## BATCH B — Backend Wiring & Quality (owns wire_gen.go)

Files modified:

- `backend/apps/api/wire_gen.go`
- `backend/service/dashboard/services/DashboardImplService.go`
- `backend/service/dashboard/services/DashboardInterfaces.go`
- `backend/service/dashboard/services/DashboardRestService.go`
- `backend/service/dashboard/services/DashboardPipelineImplService.go`
- `backend/service/dashboard/dto/Pipeline.go` (remove gorm tags from PipelineDBStats)
- `backend/service/dashboard/dto/StatsView.go` (remove dead fields)

---

### Task B1: Remove zeroJobCountReader + simplify StatsView

**Context:** `JobCountReader` returns zeros permanently. `fetchDashboardStats` (frontend) is dead code. The safest fix is to remove the job count fields from `StatsView` and remove the `JobCountReader` dependency from `DashboardImplService`.

**Files:**

- Modify: `backend/service/dashboard/dto/StatsView.go`
- Modify: `backend/service/dashboard/services/DashboardInterfaces.go`
- Modify: `backend/service/dashboard/services/DashboardImplService.go`
- Modify: `backend/apps/api/wire_gen.go`

- [ ] **B1.1** Simplify `StatsView` DTO — remove fields that were always 0:

In `backend/service/dashboard/dto/StatsView.go`, replace the entire file content with:

```go
package dto

// StatsView is the response for GET /dashboard/stats.
// job_new/saved/applied were moved to user_job_state (per-user) in Phase 2
// and are now served exclusively by GET /dashboard/progress (ProgressStats).
type StatsView struct {
	JobsScannedToday int64 `json:"jobs_scanned_today"`
	PipelinePending  int64 `json:"pipeline_pending"`
	PipelineErrors   int64 `json:"pipeline_errors"`
}
```

- [ ] **B1.2** Remove `JobCountReader` from `DashboardInterfaces.go`:

The file currently only contains the `JobCountReader` interface. Replace with:

```go
package services
```

(empty package declaration — the file can stay as a placeholder or be deleted; keeping it avoids a git confusion).

- [ ] **B1.3** Simplify `DashboardImplService.GetStats` — remove errgroup, remove jobCounts:

Replace `DashboardImplService.go` entirely:

```go
package services

import (
	"context"

	"github.com/trushilpatel/jobradar/common/errors"
	"github.com/trushilpatel/jobradar/common/tracing"
	"github.com/trushilpatel/jobradar/service/dashboard/dto"
	scansvc "github.com/trushilpatel/jobradar/service/scan/services"
)

// pipelineCountReader is the narrow interface for pipeline counts.
type pipelineCountReader interface {
	GetPipelineCounts(ctx context.Context) (pending, errCount int64, err error)
}

// DashboardImplService implements the DashboardService interface.
type DashboardImplService struct {
	scanLog  scansvc.ScanLogReader
	pipeline pipelineCountReader
}

// NewDashboardImplService constructs a DashboardImplService implementation.
func NewDashboardImplService(scanLog scansvc.ScanLogReader, pipeline pipelineCountReader) DashboardService {
	return &DashboardImplService{
		scanLog:  scanLog,
		pipeline: pipeline,
	}
}

// GetStats returns aggregated dashboard statistics.
func (s *DashboardImplService) GetStats(ctx context.Context) (_ *dto.StatsView, err error) {
	ctx, span := tracing.Start(ctx, "dashboard.service.GetStats")
	defer tracing.End(span, &err)

	scannedToday, e := s.scanLog.GetJobsScannedToday(ctx)
	if e != nil {
		return nil, errors.ErrInternal.WithMessage("failed to fetch scanned today: " + e.Error())
	}

	pending, errs, e := s.pipeline.GetPipelineCounts(ctx)
	if e != nil {
		return nil, errors.ErrInternal.WithMessage("failed to fetch pipeline counts: " + e.Error())
	}

	return &dto.StatsView{
		JobsScannedToday: scannedToday,
		PipelinePending:  pending,
		PipelineErrors:   errs,
	}, nil
}

// GetActivity returns per-day scan activity for the given number of days.
func (s *DashboardImplService) GetActivity(ctx context.Context, days int) (_ []dto.ActivityView, err error) {
	ctx, span := tracing.Start(ctx, "dashboard.service.GetActivity")
	defer tracing.End(span, &err)
	scans, err := s.scanLog.GetScansByDay(ctx, days)
	if err != nil {
		return nil, errors.ErrInternal.WithMessage("failed to fetch activity")
	}

	views := make([]dto.ActivityView, len(scans))
	for i, sc := range scans {
		views[i] = dto.ActivityView{
			Date:    sc.Date,
			Scanned: sc.Scanned,
			NewJobs: sc.NewJobs,
		}
	}
	return views, nil
}
```

**Important:** Check what `insightssvc.InsightsService.GetPipelineCounts` returns. Read `backend/service/insights/services/` to find the exact signature and adapt the `pipelineCountReader` interface accordingly. If `GetPipelineCounts` returns a struct, change the interface to match. The key goal is a narrow interface that only exposes what `DashboardImplService` needs.

- [ ] **B1.4** Update `wire_gen.go` — remove zeroJobCountReader, update DashboardImplService constructor:

Find and replace the dashboard wiring block (lines ~94-109):

```go
// ── Dashboard ─────────────────────────────────────────────────────────────
dashboardService := services4.NewDashboardImplService(scanLogReader, insightsImplService)
userDashboardStore := services4.NewUserDashboardStore(db)
appListReader := provideApplicationListReader(applicationImplService)
userDashboardService := services4.NewUserDashboardService(userDashboardStore, appListReader)
pipelineStore := services4.NewPipelineStore(db)
pipelineService := services4.NewPipelineService(pipelineStore)
dashboardRestService := services4.NewDashboardRestService(dashboardService, userDashboardService, pipelineService)
dashboardRestRouter := services4.NewRestRouter(dashboardRestService)
```

Also delete the `zeroJobCountReader` struct and `provideZeroJobCountReader` function (lines ~158-167).

Also fix `context.Background()` in `applicationListReaderAdapter.ListAll` (line ~146). Change the `ListAll` method to properly use context or — since `ListAll` is not used by any live handler — remove it from the adapter entirely if possible. Check if `ApplicationListReader.ListAll` is called anywhere; if not, remove the method from the interface in `DashboardUserService.go` and from the adapter.

---

### Task B2: Remove dead Pipeline() handler from RestService

**Files:**

- Modify: `backend/service/dashboard/services/DashboardRestService.go`

- [ ] **B2.1** Remove `pipeline PipelineService` field and `Pipeline()` method:

Remove lines 15 (`pipeline PipelineService`), update `NewDashboardRestService` to not take `pipeline` param, and delete the `Pipeline` method (lines 131-139).

Updated struct and constructor:

```go
type RestService struct {
	dashboard     DashboardService
	userDashboard UserDashboardService
}

func NewDashboardRestService(dashboard DashboardService, userDashboard UserDashboardService) *RestService {
	return &RestService{
		dashboard:     dashboard,
		userDashboard: userDashboard,
	}
}
```

- [ ] **B2.2** Update wire_gen.go to match (no longer passes `pipelineService` to `NewDashboardRestService`):

```go
dashboardRestService := services4.NewDashboardRestService(dashboardService, userDashboardService)
```

---

### Task B3: Add upper-bound validation to Activity and Velocity handlers

**Files:**

- Modify: `backend/service/dashboard/services/DashboardRestService.go`

- [ ] **B3.1** In `Activity` handler — add max bound and fix silent swallow of parse error:

Replace the `days` parsing block (lines 38-53):

```go
func (h *RestService) Activity(w http.ResponseWriter, r *http.Request) {
	days := 7
	if d := r.URL.Query().Get("days"); d != "" {
		parsed, err := strconv.Atoi(d)
		if err != nil {
			handler.HandleError(w, &errors.ValidationError{
				Field: "days", Code: "INVALID_FORMAT", Message: "days must be an integer",
			})
			return
		}
		days = parsed
	}

	if days < 1 || days > 365 {
		handler.HandleError(w, &errors.ValidationError{
			Field: "days", Code: "INVALID_VALUE", Message: "days must be between 1 and 365",
		})
		return
	}
	// ... rest of handler unchanged
```

- [ ] **B3.2** In `Velocity` handler — add max bound:

Replace the `limit` parsing block (lines 107-110):

```go
func (h *RestService) Velocity(w http.ResponseWriter, r *http.Request) {
	limit := 10
	if l := r.URL.Query().Get("limit"); l != "" {
		parsed, err := strconv.Atoi(l)
		if err != nil || parsed < 1 || parsed > 100 {
			handler.HandleError(w, &errors.ValidationError{
				Field: "limit", Code: "INVALID_VALUE", Message: "limit must be between 1 and 100",
			})
			return
		}
		limit = parsed
	}
	// ... rest unchanged
```

---

### Task B4: Fix TriggerRetry — accept context, add OTel span, fix error message

**Files:**

- Modify: `backend/service/dashboard/services/DashboardPipelineImplService.go`

- [ ] **B4.1** Update `PipelineService` interface in `DashboardPipelineService.go` to add OTel span to `GetPipelineStats` and fix `TriggerRetry`:

In `DashboardPipelineImplService.go`, replace `TriggerRetry` and `GetPipelineStats`:

```go
func (s *PipelineImplService) GetPipelineStats(ctx context.Context) (_ *dto.PipelineStats, err error) {
	ctx, span := tracing.Start(ctx, "dashboard.pipeline.GetPipelineStats")
	defer tracing.End(span, &err)
	dbStats, err := s.store.GetPipelineDBStats(ctx)
	if err != nil {
		return nil, errors.ErrInternal.WithMessage("failed to fetch pipeline stats")
	}
	return &dto.PipelineStats{
		TotalCompanies:     dbStats.TotalCompanies,
		ActiveCompanies:    dbStats.ActiveCompanies,
		Queued:             dbStats.Queued,
		Crawling:           dbStats.Crawling,
		AwaitingFilter:     dbStats.PipelinePending,
		FilterRetries:      dto.RetryTierCounts{},
		AwaitingEnrichment: dbStats.PipelineFiltered,
		EnricherRetries:    dto.RetryTierCounts{},
		NewJobs24h:         dbStats.NewJobs24h,
		Errors24h:          dbStats.Errors24h,
	}, nil
}

func (s *PipelineImplService) TriggerRetry(ctx context.Context, worker, tier string) (_ int64, err error) {
	_, span := tracing.Start(ctx, "dashboard.pipeline.TriggerRetry")
	defer tracing.End(span, &err)
	tiers, ok := validRetryTargets[worker]
	if !ok {
		return 0, errors.ErrBadRequest.WithMessage("unknown worker")
	}
	if _, ok := tiers[tier]; !ok {
		return 0, errors.ErrBadRequest.WithMessage("unknown tier")
	}
	// NATS redelivers via BackOff/MaxDeliver — no manual drain needed.
	return 0, nil
}
```

Add `tracing` import.

---

### Task B5: Remove gorm tags from PipelineDBStats DTO

**Files:**

- Modify: `backend/service/dashboard/dto/Pipeline.go`
- Modify: `backend/service/dashboard/services/DashboardRepositoryService.go` (store scan target)

**Context:** `PipelineDBStats` in `dto/Pipeline.go` has `gorm:"column:..."` tags. DTOs should be ORM-agnostic. Move the scan target to an unexported struct inside the repo method.

- [ ] **B5.1** Remove gorm tags from `PipelineDBStats` in `dto/Pipeline.go`:

```go
type PipelineDBStats struct {
	TotalCompanies  int64
	ActiveCompanies int64
	Queued          int64
	Crawling        int64
	NewJobs24h      int64
	Errors24h       int64
	PipelinePending  int64
	PipelineFiltered int64
}
```

- [ ] **B5.2** In `DashboardRepositoryService.go`, add an internal scan struct and map to `PipelineDBStats`:

```go
type pipelineDBStatsRow struct {
	TotalCompanies  int64 `gorm:"column:total_companies"`
	ActiveCompanies int64 `gorm:"column:active_companies"`
	Queued          int64 `gorm:"column:queued"`
	Crawling        int64 `gorm:"column:crawling"`
	NewJobs24h      int64 `gorm:"column:new_jobs_24h"`
	Errors24h       int64 `gorm:"column:errors_24h"`
	PipelinePending  int64 `gorm:"column:pipeline_pending"`
	PipelineFiltered int64 `gorm:"column:pipeline_filtered"`
}

func (s *pipelineRepository) GetPipelineDBStats(ctx context.Context) (_ dto.PipelineDBStats, err error) {
	ctx, span := tracing.Start(ctx, "dashboard.store.GetPipelineDBStats")
	defer tracing.End(span, &err)
	var row pipelineDBStatsRow
	err = s.db.WithContext(ctx).Raw(`
		SELECT total_companies, active_companies, queued, crawling, new_jobs_24h, errors_24h, pipeline_pending, pipeline_filtered
		FROM mv_pipeline_db_stats
	`).Scan(&row).Error
	return dto.PipelineDBStats{
		TotalCompanies:   row.TotalCompanies,
		ActiveCompanies:  row.ActiveCompanies,
		Queued:           row.Queued,
		Crawling:         row.Crawling,
		NewJobs24h:       row.NewJobs24h,
		Errors24h:        row.Errors24h,
		PipelinePending:  row.PipelinePending,
		PipelineFiltered: row.PipelineFiltered,
	}, err
}
```

Similarly apply to `CompanyVelocityRow` in `dto/CompanyVelocityRow.go` if it has gorm tags: read the file first, then remove tags and add an internal scan struct in `GetCompanyVelocityDetailed`.

---

### Task B6: Build verification

- [ ] **B6.1** After all Batch B changes, run:

```bash
cd /Users/trushil/Projects/JobRadar/backend && go build ./... 2>&1
```

Expected: zero errors. Fix any compilation errors before proceeding.

- [ ] **B6.2** Run backend tests:

```bash
cd /Users/trushil/Projects/JobRadar/backend && go test -race ./service/dashboard/... -count=1 -timeout 120s 2>&1
```

Expected: all pass. Fix any failures.

---

## BATCH C — Frontend Panels (all frontend files, independent of A and B)

Files modified:

- `ui/console/src/features/dashboard/ProgressPanel.tsx`
- `ui/console/src/features/dashboard/MarketPanel.tsx`
- `ui/console/src/features/dashboard/VelocityPanel.tsx`
- `ui/console/src/features/dashboard/TechTrendsPanel.tsx`
- `ui/console/src/features/dashboard/InterviewingPanel.tsx`
- `ui/console/src/features/dashboard/HeatmapPanel.tsx`
- `ui/console/src/pages/DashboardPage.tsx`
- `ui/console/src/features/dashboard/components/StatCol.tsx` (NEW)
- `ui/console/src/features/dashboard/index.ts` (NEW)
- `ui/console/src/api/dashboard.ts` (remove dead functions, remove intermediate type)

---

### Task C1: Add isError to ProgressPanel, fix KPI zero display, extract StatCol

**Files:**

- Modify: `ui/console/src/features/dashboard/ProgressPanel.tsx`
- Create: `ui/console/src/features/dashboard/components/StatCol.tsx`

- [ ] **C1.1** Create `ui/console/src/features/dashboard/components/StatCol.tsx`:

```tsx
import { cn } from '@/lib/utils'

interface StatColProps {
  label: string
  value: number | string
  sub: string
  valueClass: string
  onClick?: () => void
  onKeyDown?: (e: React.KeyboardEvent) => void
}

export function StatCol({ label, value, sub, valueClass, onClick, onKeyDown }: StatColProps) {
  const isInteractive = Boolean(onClick)
  return (
    <div
      role={isInteractive ? 'button' : undefined}
      tabIndex={isInteractive ? 0 : undefined}
      className={cn(
        'flex-1 flex flex-col gap-1 px-5 first:pl-0 last:pr-0',
        isInteractive &&
          'cursor-pointer group focus:outline-none focus-visible:ring-1 focus-visible:ring-white/30 rounded'
      )}
      onClick={onClick}
      onKeyDown={onKeyDown}
    >
      <span className="text-[10px] font-medium uppercase tracking-[0.07em] text-slate-500">
        {label}
      </span>
      <span
        className={cn(
          'text-[36px] font-medium leading-none tabular-nums',
          valueClass,
          isInteractive && 'group-hover:opacity-80 transition-opacity'
        )}
      >
        {value}
      </span>
      <span
        className={cn(
          'text-[11px] text-slate-500',
          isInteractive && 'group-hover:text-slate-400 transition-colors'
        )}
      >
        {sub}
      </span>
    </div>
  )
}
```

- [ ] **C1.2** Rewrite `ProgressPanel.tsx`:

```tsx
import { useQuery } from '@tanstack/react-query'
import { fetchProgress } from '@/api/dashboard'
import { dashboardQueryKeys } from '../queryKeys'
import { StatCol } from './components/StatCol'

function hasAnyData(applied: number, interviewing: number, offer: number): boolean {
  return applied > 0 || interviewing > 0 || offer > 0
}

export function ProgressPanel() {
  const { data, isLoading, isError } = useQuery({
    queryKey: dashboardQueryKeys.progress(),
    queryFn: fetchProgress,
    refetchInterval: 60_000,
  })

  return (
    <div className="bg-white/[0.04] border border-white/[0.07] rounded-xl p-5">
      <p className="text-[10px] font-medium uppercase tracking-[0.07em] text-slate-500 mb-4">
        my progress
      </p>

      {isLoading ? (
        <div className="flex gap-4 animate-pulse">
          {[...Array(3)].map((_, i) => (
            <div key={i} className="flex-1 h-14 bg-white/[0.04] rounded-lg" />
          ))}
        </div>
      ) : isError || !data ? (
        <p className="text-sm text-slate-500 py-2">Failed to load progress data</p>
      ) : (
        <div className="flex items-stretch">
          <StatCol
            label="Applied"
            value={
              hasAnyData(data.applied_count, data.interviewing_count, data.offer_count)
                ? data.applied_count
                : '—'
            }
            sub={`${data.awaiting_reply} awaiting reply`}
            valueClass="text-emerald-400"
          />
          <div className="w-px bg-white/[0.07] self-stretch" />
          <StatCol
            label="Interviewing"
            value={
              hasAnyData(data.applied_count, data.interviewing_count, data.offer_count)
                ? data.interviewing_count
                : '—'
            }
            sub={`${data.interviewing_this_week} this week`}
            valueClass="text-violet-400"
          />
          <div className="w-px bg-white/[0.07] self-stretch" />
          <StatCol
            label="Offer"
            value={
              hasAnyData(data.applied_count, data.interviewing_count, data.offer_count)
                ? data.offer_count
                : '—'
            }
            sub="decision pending"
            valueClass="text-amber-400"
          />
        </div>
      )}
    </div>
  )
}
```

---

### Task C2: Add isError to MarketPanel, make StatCol keyboard accessible

**Files:**

- Modify: `ui/console/src/features/dashboard/MarketPanel.tsx`

- [ ] **C2.1** Rewrite `MarketPanel.tsx` using shared `StatCol`:

```tsx
import { useQuery } from '@tanstack/react-query'
import { useNavigate } from 'react-router-dom'
import { fetchMarket } from '@/api/dashboard'
import { dashboardQueryKeys } from '../queryKeys'
import { StatCol } from './components/StatCol'

export function MarketPanel() {
  const navigate = useNavigate()
  const { data, isLoading, isError } = useQuery({
    queryKey: dashboardQueryKeys.market(),
    queryFn: fetchMarket,
    refetchInterval: 60_000,
  })

  const weeklyDeltaLabel = data
    ? data.weekly_delta >= 0
      ? `+${data.weekly_delta} this week`
      : `${data.weekly_delta} this week`
    : '—'

  const handleNewJobsClick = () => navigate('/jobs', { state: { dateFilter: '24h' } })
  const handleNewJobsKeyDown = (e: React.KeyboardEvent) => {
    if (e.key === 'Enter' || e.key === ' ') {
      e.preventDefault()
      navigate('/jobs', { state: { dateFilter: '24h' } })
    }
  }

  return (
    <div className="bg-white/[0.04] border border-white/[0.07] rounded-xl p-5">
      <p className="text-[10px] font-medium uppercase tracking-[0.07em] text-slate-500 mb-4">
        market
      </p>

      {isLoading ? (
        <div className="flex gap-4 animate-pulse">
          {[...Array(2)].map((_, i) => (
            <div key={i} className="flex-1 h-14 bg-white/[0.04] rounded-lg" />
          ))}
        </div>
      ) : isError || !data ? (
        <p className="text-sm text-slate-500 py-2">Failed to load market data</p>
      ) : (
        <div className="flex items-stretch">
          <StatCol
            label="New jobs"
            value={data.new_since_yesterday}
            sub="since yesterday"
            valueClass="text-blue-400"
            onClick={handleNewJobsClick}
            onKeyDown={handleNewJobsKeyDown}
          />
          <div className="w-px bg-white/[0.07] self-stretch" />
          <StatCol
            label="Total open"
            value={data.total_open}
            sub={weeklyDeltaLabel}
            valueClass="text-white"
          />
        </div>
      )}
    </div>
  )
}
```

---

### Task C3: Add isError to VelocityPanel, remove inline styles

**Files:**

- Modify: `ui/console/src/features/dashboard/VelocityPanel.tsx`

- [ ] **C3.1** Rewrite `VelocityPanel.tsx` — add `isError`, replace inline `style` with Tailwind:

```tsx
import { useQuery } from '@tanstack/react-query'
import { fetchVelocity } from '@/api/dashboard'
import type { CompanyVelocityRow } from '@/types'
import { cn } from '@/lib/utils'
import { dashboardQueryKeys } from '../queryKeys'

const BADGE: Record<string, string> = {
  spike: 'bg-amber-500/15 text-amber-400',
  rising: 'bg-emerald-500/15 text-emerald-400',
  normal: 'bg-white/[0.06] text-slate-400',
}

function Sparkline({ row }: { row: CompanyVelocityRow }) {
  const bars = [row.week_3, row.week_2, row.week_1, row.week_current]
  const max = Math.max(...bars, 1)
  return (
    <div
      className="flex items-end gap-[2px] h-[22px]"
      role="img"
      aria-label={`4-week trend: ${bars.join(', ')}`}
    >
      {bars.map((v, i) => {
        const heightPct = Math.max(3, Math.round((v / max) * 22))
        const isCurrent = i === bars.length - 1
        return (
          <div
            key={i}
            className={cn('rounded-[2px] w-2', isCurrent ? 'bg-emerald-600' : 'bg-emerald-950')}
            style={{ height: `${heightPct}px` }}
          />
        )
      })}
    </div>
  )
}

export function VelocityPanel() {
  const {
    data: rows,
    isLoading,
    isError,
  } = useQuery({
    queryKey: dashboardQueryKeys.velocity(),
    queryFn: () => fetchVelocity(10),
    refetchInterval: 120_000,
  })

  return (
    <div className="bg-white/[0.04] border border-white/[0.07] rounded-xl p-5">
      <p className="text-[10px] font-medium uppercase tracking-[0.07em] text-slate-500 mb-4">
        top companies by velocity
      </p>

      <div className="grid text-[10px] text-slate-500 pb-2 border-b border-white/[0.05] [grid-template-columns:1fr_60px_50px_36px] gap-3">
        <span>company</span>
        <span>velocity</span>
        <span className="text-center">4 weeks</span>
        <span className="text-right">new</span>
      </div>

      {isLoading ? (
        <div className="space-y-3 mt-2 animate-pulse">
          {[...Array(5)].map((_, i) => (
            <div key={i} className="h-8 bg-white/[0.04] rounded-lg" />
          ))}
        </div>
      ) : isError ? (
        <p className="text-sm text-slate-500 py-4">Failed to load velocity data</p>
      ) : !rows || rows.length === 0 ? (
        <p className="text-sm text-slate-600 py-4">No velocity data yet</p>
      ) : (
        <div>
          {rows.map((row, i) => (
            <div
              key={row.company_id}
              className={cn(
                'grid items-center py-2.5 [grid-template-columns:1fr_60px_50px_36px] gap-3',
                i < rows.length - 1 && 'border-b border-white/[0.05]'
              )}
            >
              <span className="text-[13px] font-medium text-white truncate">
                {row.company_name}
              </span>
              <span
                className={cn(
                  'inline-flex justify-center px-2 py-0.5 rounded-full text-[11px] font-medium',
                  BADGE[row.velocity_tag]
                )}
              >
                {row.velocity_tag !== 'normal' ? `${row.multiplier}×` : '—'}
              </span>
              <div className="flex justify-center">
                <Sparkline row={row} />
              </div>
              <span className="text-[13px] font-medium text-emerald-400 text-right tabular-nums">
                +{row.week_current}
              </span>
            </div>
          ))}
        </div>
      )}
    </div>
  )
}
```

---

### Task C4: Add isError to TechTrendsPanel (not null), remove inline styles

**Files:**

- Modify: `ui/console/src/features/dashboard/TechTrendsPanel.tsx`

- [ ] **C4.1** Fix `TechTrendsPanel.tsx` — change `!data ? null` to proper error state:

Change lines 98-101:

```tsx
{isLoading ? (
  <div className="h-40 animate-pulse bg-white/[0.03] rounded-lg" />
) : isError || !data ? (
  <p className="text-sm text-slate-500 py-4">
    {isError ? 'Failed to load tech trends' : 'No data yet'}
  </p>
) : (
```

Also remove inline styles from `TechSparkline` (the `style={{ height, flexShrink }}` properties):

- Replace `style={{ height: '20px', flexShrink: 0 }}` with Tailwind: `className="flex items-end gap-[3px] h-5 shrink-0"`
- Replace `style={{ width: '22px', height: '${h}px', background: ... }}` — the dynamic height must stay as inline style, but `width: '22px'` can become `w-[22px]` Tailwind class

Also remove `style={{ minWidth: 0 }}` from `TechRow` — replace with `min-w-0` Tailwind class.

---

### Task C5: Add isError + onError toast + fix cache invalidation in InterviewingPanel, replace StageDropdown with Radix Select

**Files:**

- Modify: `ui/console/src/features/dashboard/InterviewingPanel.tsx`

- [ ] **C5.1** Rewrite `InterviewingPanel.tsx`:

Key changes:

1. Add `isError` branch
2. Add `onError: () => toast.error('Failed to update stage')` to mutation
3. Also invalidate `dashboardQueryKeys.progress()` on mutation success
4. Replace `StageDropdown` with Radix `Select` (already installed: `@radix-ui/react-select`)

```tsx
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { toast } from 'sonner'
import * as Select from '@radix-ui/react-select'
import { ChevronDown, Check } from 'lucide-react'
import { fetchInterviews, updateApplicationStage } from '@/api/dashboard'
import type { InterviewRow } from '@/types'
import { cn } from '@/lib/utils'
import { dashboardQueryKeys } from '../queryKeys'

const STAGES = [
  {
    key: 'applied',
    label: 'Waiting',
    dotColor: '#888780',
    pillClass: 'bg-white/[0.06] text-slate-400',
  },
  {
    key: 'phone_screen',
    label: 'Interviewing',
    dotColor: '#7f77dd',
    pillClass: 'bg-violet-500/15 text-violet-300',
  },
  {
    key: 'offer',
    label: 'Offer received',
    dotColor: '#1d9e75',
    pillClass: 'bg-emerald-500/15 text-emerald-400',
  },
]

function pillConfig(stage: string) {
  return (
    STAGES.find((s) => s.key === stage) ?? {
      key: stage,
      label: stage,
      dotColor: '#888780',
      pillClass: 'bg-white/[0.06] text-slate-400',
    }
  )
}

function StageSelect({
  row,
  onStageChange,
}: {
  row: InterviewRow
  onStageChange: (id: number, stage: string) => void
}) {
  const cfg = pillConfig(row.stage)
  return (
    <Select.Root value={row.stage} onValueChange={(val) => onStageChange(row.application_id, val)}>
      <Select.Trigger
        className={cn(
          'inline-flex items-center gap-1 px-2.5 py-1 rounded-full text-[11px] font-medium cursor-pointer hover:opacity-85 transition-opacity focus:outline-none focus-visible:ring-1 focus-visible:ring-white/30',
          cfg.pillClass
        )}
        aria-label={`Stage: ${cfg.label}`}
      >
        <Select.Value />
        <Select.Icon>
          <ChevronDown size={11} className="opacity-60" />
        </Select.Icon>
      </Select.Trigger>

      <Select.Portal>
        <Select.Content
          className="z-50 min-w-[160px] overflow-hidden bg-[#141418] border border-white/[0.12] rounded-lg shadow-xl"
          position="popper"
          sideOffset={4}
        >
          <Select.Viewport className="p-1">
            {STAGES.map((opt) => (
              <Select.Item
                key={opt.key}
                value={opt.key}
                className="flex items-center gap-2 px-3 py-2 text-[11px] text-slate-300 rounded cursor-pointer hover:bg-white/[0.05] focus:bg-white/[0.05] focus:outline-none data-[state=checked]:text-white data-[state=checked]:bg-white/[0.06]"
              >
                <span
                  className="w-[6px] h-[6px] rounded-full shrink-0"
                  style={{ background: opt.dotColor }}
                />
                <Select.ItemText>{opt.label}</Select.ItemText>
                <Select.ItemIndicator className="ml-auto">
                  <Check size={11} />
                </Select.ItemIndicator>
              </Select.Item>
            ))}
          </Select.Viewport>
        </Select.Content>
      </Select.Portal>
    </Select.Root>
  )
}

export function InterviewingPanel() {
  const queryClient = useQueryClient()

  const {
    data: rows,
    isLoading,
    isError,
  } = useQuery({
    queryKey: dashboardQueryKeys.interviews(),
    queryFn: fetchInterviews,
    refetchInterval: 30_000,
  })

  const mutation = useMutation({
    mutationFn: ({ id, stage }: { id: number; stage: string }) => updateApplicationStage(id, stage),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: dashboardQueryKeys.interviews() })
      queryClient.invalidateQueries({ queryKey: dashboardQueryKeys.progress() })
    },
    onError: () => toast.error('Failed to update stage'),
  })

  return (
    <div className="bg-white/[0.04] border border-white/[0.07] rounded-xl p-5">
      <p className="text-[10px] font-medium uppercase tracking-[0.07em] text-slate-500 mb-4">
        interviewing
      </p>

      {isLoading ? (
        <div className="space-y-3 animate-pulse">
          {[...Array(3)].map((_, i) => (
            <div key={i} className="h-10 bg-white/[0.04] rounded-lg" />
          ))}
        </div>
      ) : isError ? (
        <p className="text-sm text-slate-500 py-2">Failed to load interviews</p>
      ) : !rows || rows.length === 0 ? (
        <p className="text-sm text-slate-600 py-2">No active applications</p>
      ) : (
        <div>
          {rows.map((row, i) => (
            <div
              key={row.application_id}
              className={cn(
                'flex items-center gap-3 py-2.5',
                i < rows.length - 1 && 'border-b border-white/[0.05]'
              )}
            >
              <div className="flex-1 min-w-0">
                <p className="text-[13px] font-medium text-white truncate">{row.company_name}</p>
                <p className="text-[11px] text-slate-500 truncate mt-0.5">{row.job_title}</p>
              </div>
              <StageSelect
                row={row}
                onStageChange={(id, stage) => mutation.mutate({ id, stage })}
              />
            </div>
          ))}
        </div>
      )}
    </div>
  )
}
```

---

### Task C6: Fix HeatmapPanel — remove data=[] anti-pattern

**Files:**

- Modify: `ui/console/src/features/dashboard/HeatmapPanel.tsx`

- [ ] **C6.1** Remove `data: sparse = []` default (line 115):

Change:

```tsx
const { data: sparse = [], isLoading, isError } = useQuery({...})
```

To:

```tsx
const { data: sparse, isLoading, isError } = useQuery({...})
```

Then update line 124 where `sparse` is used:

```tsx
const weeks = buildWeekGrid(sparse ?? [])
```

The `isError` branch already exists in HeatmapPanel — no change needed there.

---

### Task C7: Add page-level ErrorBoundary to DashboardPage

**Files:**

- Read: `ui/console/src/pages/DashboardPage.tsx` (check current structure)
- Modify: `ui/console/src/pages/DashboardPage.tsx`

- [ ] **C7.1** Read the current DashboardPage to understand its structure, then wrap all panels in an `ErrorBoundary`.

Check if an `ErrorBoundary` component exists in `ui/console/src/components/`. If not, create `ui/console/src/components/shared/ErrorBoundary.tsx`:

```tsx
import { Component, type ErrorInfo, type ReactNode } from 'react'

interface Props {
  fallback?: ReactNode
  children: ReactNode
}
interface State {
  hasError: boolean
}

export class ErrorBoundary extends Component<Props, State> {
  state: State = { hasError: false }

  static getDerivedStateFromError(): State {
    return { hasError: true }
  }

  componentDidCatch(error: Error, info: ErrorInfo) {
    console.error('Dashboard ErrorBoundary caught:', error, info)
  }

  render() {
    if (this.state.hasError) {
      return (
        this.props.fallback ?? (
          <div className="rounded-xl border border-white/[0.07] bg-white/[0.04] p-5 text-sm text-slate-500">
            Something went wrong loading this panel.
          </div>
        )
      )
    }
    return this.props.children
  }
}
```

Then in `DashboardPage.tsx`, wrap each panel:

```tsx
import { ErrorBoundary } from '@/components/shared/ErrorBoundary'
// ...
<ErrorBoundary>
  <ProgressPanel />
</ErrorBoundary>
<ErrorBoundary>
  <MarketPanel />
</ErrorBoundary>
// etc.
```

---

### Task C8: Remove dead fetchDashboardStats and fetchDashboardActivity from api/dashboard.ts

**Files:**

- Modify: `ui/console/src/api/dashboard.ts`
- Modify: `ui/console/src/features/dashboard/queryKeys.ts`

**Important — verify before deleting:**

```bash
grep -r "fetchDashboardStats\|fetchDashboardActivity\|dashboardQueryKeys\.stats\|dashboardQueryKeys\.activity" \
  /Users/trushil/Projects/JobRadar/ui/console/src --include="*.tsx" --include="*.ts" | grep -v "__tests__"
```

If the result is only the definition files themselves (no consumers), proceed to delete.

- [ ] **C8.1** Remove `fetchDashboardStats`, `fetchDashboardActivity`, and `DashboardStatsRaw` interface from `api/dashboard.ts`. Remove `DAY_NAMES` constant if only used by those two functions.

- [ ] **C8.2** Remove `stats: () => [...]` and `activity: (days) => [...]` from `queryKeys.ts` if confirmed dead.

---

### Task C9: Add barrel index.ts for dashboard feature

**Files:**

- Create: `ui/console/src/features/dashboard/index.ts`

- [ ] **C9.1** Create `ui/console/src/features/dashboard/index.ts` exporting all public components:

```ts
export { ProgressPanel } from './ProgressPanel'
export { MarketPanel } from './MarketPanel'
export { VelocityPanel } from './VelocityPanel'
export { TechTrendsPanel } from './TechTrendsPanel'
export { InterviewingPanel } from './InterviewingPanel'
export { HeatmapPanel } from './HeatmapPanel'
```

- [ ] **C9.2** Frontend build verification:

```bash
cd /Users/trushil/Projects/JobRadar/ui/console && npm run build 2>&1 | tail -30
```

Expected: zero TypeScript errors and a successful build.

---

## BATCH D — Backend Tests (depends on A+B completing first)

Files modified:

- `backend/service/dashboard/tests/DashboardRepositoryService_test.go`
- `backend/service/dashboard/tests/DashboardRestService_test.go`
- `backend/service/dashboard/tests/DashboardImplService_test.go`
- `backend/service/dashboard/tests/DashboardPipelineImplService_test.go`

### Task D1: Test GetProgressStats user-scoping

- [ ] **D1.1** Add a success-path test for `GetProgressStats` that verifies it passes userID:

In `DashboardRepositoryService_test.go`, add a test using a fake `UserDashboardStore` that records the `userID` argument and verify it's populated. The existing repository tests use a closed DB (error path only) which is fine for unit tests — add a fake-store test at the impl layer:

```go
func TestUserDashboardImplService_GetProgressStats_PassesUserID(t *testing.T) {
    var capturedUserID string
    fake := &fakeUserDashboardStore{
        getProgressStatsFn: func(ctx context.Context, userID string) (dto.ProgressStats, error) {
            capturedUserID = userID
            return dto.ProgressStats{AppliedCount: 5, InterviewingCount: 2, OfferCount: 1}, nil
        },
    }
    svc := NewUserDashboardService(fake, &fakeAppListReader{})
    ctx := identity.WithTestIdentity(context.Background(), identity.Identity{UserID: "user-42"})
    stats, err := svc.GetProgressStats(ctx)
    require.NoError(t, err)
    assert.Equal(t, "user-42", capturedUserID)
    assert.Equal(t, int64(5), stats.AppliedCount)
    assert.Equal(t, int64(2), stats.InterviewingCount)
    assert.Equal(t, int64(1), stats.OfferCount)
}
```

Ensure `fakeUserDashboardStore.getProgressStatsFn` takes `(ctx context.Context, userID string)` to match the updated interface.

### Task D2: Test GetMarketStats success path

- [ ] **D2.1** Add success-path test to `DashboardRepositoryService_test.go`:

Currently only error path is tested (75% coverage). Add a fake-store test at the impl level:

```go
func TestUserDashboardImplService_GetMarketStats_WeeklyDeltaComputed(t *testing.T) {
    fake := &fakeUserDashboardStore{
        getMarketStatsFn: func(ctx context.Context) (dto.MarketStats, error) {
            return dto.MarketStats{
                NewSinceYesterday: 3,
                TotalOpen:        100,
                AddedThisWeek:    10,
                AddedLastWeek:    6,
                WeeklyDelta:      4, // 10 - 6
            }, nil
        },
    }
    svc := NewUserDashboardService(fake, &fakeAppListReader{})
    result, err := svc.GetMarketStats(context.Background())
    require.NoError(t, err)
    assert.Equal(t, int64(4), result.WeeklyDelta)
    assert.Equal(t, int64(3), result.NewSinceYesterday)
}
```

### Task D3: Test Activity handler upper-bound validation

- [ ] **D3.1** Add tests to `DashboardRestService_test.go`:

```go
func TestDashboardHandlerActivity_NegativeDays_Returns400(t *testing.T) {
    // days=-5 parses successfully but fails < 1 check
    // ... assert 400
}

func TestDashboardHandlerActivity_TooBigDays_Returns400(t *testing.T) {
    // days=366 exceeds max
    // ... assert 400
}

func TestDashboardHandlerActivity_NonIntegerDays_Returns400(t *testing.T) {
    // days=abc fails Atoi, should now return 400 (not fall back to default)
    // ... assert 400
}

func TestDashboardHandlerVelocity_TooLargeLimit_Returns400(t *testing.T) {
    // limit=999 exceeds 100
    // ... assert 400
}
```

### Task D4: Test TriggerRetry edge cases

- [ ] **D4.1** Add to `DashboardPipelineImplService_test.go`:

```go
func TestTriggerRetry_EmptyWorker_Returns400(t *testing.T) {
    svc := NewPipelineService(&fakePipelineStore{})
    _, err := svc.TriggerRetry(context.Background(), "", "1m")
    require.Error(t, err)
    // assert errors.ErrBadRequest
}

func TestTriggerRetry_EmptyTier_Returns400(t *testing.T) {
    svc := NewPipelineService(&fakePipelineStore{})
    _, err := svc.TriggerRetry(context.Background(), "filter", "")
    require.Error(t, err)
}
```

### Task D5: Run full backend test suite

- [ ] **D5.1**:

```bash
cd /Users/trushil/Projects/JobRadar/backend && go test -race ./service/dashboard/... -count=1 -timeout 120s -v 2>&1 | tail -40
```

Expected: all pass, race-clean.

- [ ] **D5.2** Check coverage:

```bash
cd /Users/trushil/Projects/JobRadar/backend && go test -coverprofile=/tmp/cov.out ./service/dashboard/... && go tool cover -func=/tmp/cov.out | grep -v "100.0%"
```

Expected: no package below 80%.

---

## BATCH E — Frontend Tests (depends on C completing first)

Files modified:

- `ui/console/src/features/dashboard/__tests__/dashboard.test.tsx`

### Task E1: Add error state tests for all 5 panels

- [ ] **E1.1** In `dashboard.test.tsx`, for each panel that was missing an error test, add:

```tsx
describe('ProgressPanel', () => {
  // ... existing tests ...
  it('shows error state when fetchProgress rejects', async () => {
    vi.mocked(fetchProgress).mockRejectedValue(new Error('network error'))
    render(<ProgressPanel />, { wrapper })
    await waitFor(() => {
      expect(screen.getByText(/failed to load progress data/i)).toBeInTheDocument()
    })
    expect(screen.queryByText(/applied/i)).not.toBeInTheDocument() // no data rendered
  })
})
```

Repeat for `MarketPanel`, `InterviewingPanel`, `VelocityPanel`, `TechTrendsPanel`.

For `TechTrendsPanel` specifically, verify the error state renders text (not null):

```tsx
it('shows error message (not blank) when fetchTechTrends rejects', async () => {
  vi.mocked(fetchTechTrends).mockRejectedValue(new Error('err'))
  render(<TechTrendsPanel />, { wrapper })
  await waitFor(() => {
    expect(screen.getByText(/failed to load tech trends/i)).toBeInTheDocument()
  })
})
```

### Task E2: Test InterviewingPanel mutation behavior

- [ ] **E2.1** Add mutation tests:

```tsx
it('shows error toast when stage update fails', async () => {
  vi.mocked(fetchInterviews).mockResolvedValue([sampleRow])
  vi.mocked(updateApplicationStage).mockRejectedValue(new Error('server error'))
  render(<InterviewingPanel />, { wrapper })
  await screen.findByText(sampleRow.company_name)
  // open Radix Select and select an option
  // ... verify toast.error is called (mock sonner)
})

it('invalidates both interviews and progress queries on success', async () => {
  vi.mocked(fetchInterviews).mockResolvedValue([sampleRow])
  vi.mocked(updateApplicationStage).mockResolvedValue(undefined)
  const client = createTestQueryClient()
  const invalidateSpy = vi.spyOn(client, 'invalidateQueries')
  // ... trigger mutation ...
  await waitFor(() => {
    expect(invalidateSpy).toHaveBeenCalledWith(
      expect.objectContaining({
        queryKey: dashboardQueryKeys.interviews(),
      })
    )
    expect(invalidateSpy).toHaveBeenCalledWith(
      expect.objectContaining({
        queryKey: dashboardQueryKeys.progress(),
      })
    )
  })
})
```

### Task E3: Test DashboardPage smoke test

- [ ] **E3.1** Add a basic render test for `DashboardPage`:

```tsx
describe('DashboardPage', () => {
  it('renders without crashing when all queries succeed', async () => {
    vi.mocked(fetchProgress).mockResolvedValue({ applied_count: 1, ... })
    vi.mocked(fetchMarket).mockResolvedValue({ ... })
    // mock all other fetch functions
    render(<DashboardPage />, { wrapper: withRouter })
    // just verify it doesn't throw
    await waitFor(() => {
      expect(screen.queryByText(/error/i)).not.toBeInTheDocument()
    })
  })
})
```

### Task E4: Add API normalisation tests

- [ ] **E4.1** Create `ui/console/src/api/__tests__/dashboard.test.ts`:

```ts
import { vi, describe, it, expect, beforeEach } from 'vitest'
import { apiClient } from '@/lib/api-client'
import { fetchProgress, fetchMarket, fetchVelocity } from '@/api/dashboard'

vi.mock('@/lib/api-client')

describe('fetchProgress', () => {
  it('normalizes null response to zero counts', async () => {
    vi.mocked(apiClient.get).mockResolvedValue(null)
    const result = await fetchProgress()
    expect(result.applied_count).toBe(0)
    expect(result.interviewing_count).toBe(0)
    expect(result.offer_count).toBe(0)
  })

  it('normalizes partial response', async () => {
    vi.mocked(apiClient.get).mockResolvedValue({ applied_count: 5 })
    const result = await fetchProgress()
    expect(result.applied_count).toBe(5)
    expect(result.interviewing_count).toBe(0)
  })
})

describe('fetchVelocity', () => {
  it('normalizes null to empty array', async () => {
    vi.mocked(apiClient.get).mockResolvedValue(null)
    const result = await fetchVelocity()
    expect(result).toEqual([])
  })
})
```

### Task E5: Run frontend tests

- [ ] **E5.1**:

```bash
cd /Users/trushil/Projects/JobRadar/ui/console && npx vitest run --reporter=verbose 2>&1 | tail -40
```

Expected: all tests pass.

- [ ] **E5.2** Check coverage:

```bash
cd /Users/trushil/Projects/JobRadar/ui/console && npx vitest run --coverage 2>&1 | grep -E "dashboard|All files"
```

Expected: dashboard files ≥ 90% statement coverage.

---

## Final Steps: Full Build, Migration, Push

### Task F1: Run full pre-commit gates

- [ ] **F1.1** Backend:

```bash
cd /Users/trushil/Projects/JobRadar/backend && \
  go build ./... && \
  go test -race -short ./... -count=1 -timeout 120s && \
  golangci-lint run --config ../.github/.golangci.yml --timeout 5m
```

- [ ] **F1.2** Frontend:

```bash
cd /Users/trushil/Projects/JobRadar/ui/console && \
  npm run lint && \
  npx tsc --noEmit && \
  npx vitest run
```

Fix any failures before proceeding.

### Task F2: Commit and push

- [ ] **F2.1** Stage all changes:

```bash
git add backend/service/dashboard/ backend/apps/api/wire_gen.go backend/migrations/068_dashboard_security_indexes.sql \
        ui/console/src/features/dashboard/ ui/console/src/pages/DashboardPage.tsx \
        ui/console/src/api/dashboard.ts ui/console/src/components/shared/ErrorBoundary.tsx
git status
```

- [ ] **F2.2** Commit:

```bash
git commit -m "fix(dashboard): security, correctness, UX, and observability overhaul

- P0: GetProgressStats now filters by user_id (cross-user data leak)
- P0: Fix interviewing_count/offer_count CTE logic (always returned 0)
- P0: Remove zeroJobCountReader; simplify StatsView to accurate fields only
- P0: Fix context.Background() in applicationListReaderAdapter.ListAll
- P1: Add composite index (user_id, stage_at DESC) on job_application_tracker
- P1: Merge double job_tech_tags scan into single CTE
- P1: Add isError state to 5 panels (were stuck in skeleton on failure)
- P1: Replace hand-rolled StageDropdown with Radix UI Select (keyboard/a11y)
- P1: Make MarketPanel StatCol keyboard accessible
- P1: Add upper-bound validation on days and limit query params
- P2: Remove dead Pipeline() handler from RestService
- P2: Fix GetInterviews context propagation (discarded span child ctx)
- P2: Add OTel spans to GetPipelineStats, TriggerRetry, Refresh* methods
- P2: Fix TriggerRetry to accept context; sanitize error message
- P2: Remove gorm tags from PipelineDBStats DTO
- P2: Add onError toast to InterviewingPanel mutation
- P2: InterviewingPanel mutation now also invalidates progress query
- P2: KPI cards show — instead of 0 for new users
- P2: Add page-level ErrorBoundary to DashboardPage
- P2: Remove fetchDashboardStats/fetchDashboardActivity dead code
- P2: Replace cross-domain appcommon imports with local stage display map
- P2: Remove inline styles in VelocityPanel/TechTrendsPanel (Tailwind)
- P2: Extract shared StatCol component; add dashboard feature barrel
- Tests: error states, mutation behavior, API normalisation, smoke tests"
```

### Task F3: Push and create PR

- [ ] **F3.1** Push:

```bash
git push -u origin fix/dashboard-quality-overhaul
```

The output will include a URL to create the PR on GitHub. Use that URL.

### Task F4: Run migrations in deployment

- [ ] **F4.1** After the PR is merged, apply migration 068:

```bash
# Via k3d/kubectl (see infra/CLAUDE.md for details)
# The migration runner applies all pending migrations in numeric order
kubectl exec -it <api-pod> -- /app/api migrate
```

Or if running locally:

```bash
cd backend && go run ./apps/api/... migrate
```

Migration 068 uses `CREATE INDEX CONCURRENTLY IF NOT EXISTS` which is safe on a live database.
