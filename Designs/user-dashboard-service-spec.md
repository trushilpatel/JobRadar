# UserDashboard Service Spec

## Context

Single-user system. No caching, no materialized views, no Redis required at this scale. All queries run live on dashboard load. Total expected latency well under 200ms.

**Existing pipeline:**

```
company_scan_scheduler → company_scan_queue → job_listing_crawler
  → job_listings.raw (Kafka) → job_listing_relevance_filter
  → job_listings.filtered (Kafka) → job_post_analyzer
  → job_feed (DB) + job_analytics (DB) → API
```

**Relevant tables:**

| Table                     | Role                                                               |
| ------------------------- | ------------------------------------------------------------------ |
| `job_feed`                | Enriched jobs — status tracks user lifecycle                       |
| `job_analytics`           | Append-only snapshot per job — never deleted, has tech_stack JSONB |
| `job_application_tracker` | Append-only stage history per applied job                          |
| `companies`               | Tracked companies                                                  |
| `company_scan_log`        | Per-run scan summaries                                             |

**Job status values in `job_feed`** (migration 023 applied):
`new` → `saved` → `applied` → `interviewing` → `offer`
`new` → `dismissed`

**Application stage values in `job_application_tracker`:**
`applied`, `phone_screen`, `technical`, `final_round`, `offer`, `rejected`

---

## What Already Exists (do not rewrite)

| What                                | Location                       | Notes                                                                                       |
| ----------------------------------- | ------------------------------ | ------------------------------------------------------------------------------------------- |
| `GetLatestApplicationsWithJob()`    | `service/application/store.go` | Returns latest stage per job with company + title — use as-is for interviewing tracker list |
| `UpdateApplicationStage(id, stage)` | `service/application/store.go` | Already implemented — just needs a route                                                    |
| `GetJobCounts()`                    | `service/job/store.go`         | Returns new/saved/applied counts — partial overlap with progress stats                      |
| `GetCompanyVelocity(limit)`         | `service/insights/store.go`    | Returns this_week/last_week only — not sufficient, needs replacing with 4-week version      |
| `GetTechStackTrends(limit)`         | `service/insights/store.go`    | Flat all-time counts — not sufficient, needs replacing with quarterly version               |
| `Dashboard handler`                 | `service/dashboard/handler.go` | Has `/dashboard/stats` and `/dashboard/activity` — extend, don't replace                    |

---

## New: UserDashboardService

### Service Interface

```go
type UserDashboardService interface {
    GetProgressStats()  (*ProgressStats, error)
    GetMarketStats()    (*MarketStats, error)
    GetHeatmap()        ([]WeeklyCount, error)
    GetInterviews()     ([]InterviewRow, error)
    GetVelocity(limit int) ([]CompanyVelocityRow, error)
    GetTechTrends()     (*TechTrends, error)
}
```

Each method maps to one dashboard panel. The handler assembles them in parallel into a single `DashboardView` response, or exposes them as individual endpoints — see routing section.

---

## Panel Specs

---

### 1. My Progress

**UI shows:** Applied (count + "N awaiting reply"), Interviewing (count + "N this week"), Offer (count + "decision pending")

**Design notes:**

- Applied = `job_feed.status = 'applied'`
- Awaiting reply = applied jobs with no stage beyond `applied` in tracker yet
- Interviewing = `job_feed.status = 'interviewing'`
- Interviewing this week = stage transition into an interview stage (`phone_screen`, `technical`, `final_round`) this calendar week
- Offer = `job_feed.status = 'offer'`
- "decision pending" is just the offer count re-labelled — no extra field needed

**Response type:**

```go
type ProgressStats struct {
    AppliedCount          int64 `json:"applied_count"`
    AwaitingReply         int64 `json:"awaiting_reply"`
    InterviewingCount     int64 `json:"interviewing_count"`
    InterviewingThisWeek  int64 `json:"interviewing_this_week"`
    OfferCount            int64 `json:"offer_count"`
}
```

**Optimized query — single pass, two CTEs:**

```sql
WITH status_counts AS (
    SELECT
        COUNT(*) FILTER (WHERE status = 'applied')      AS applied_count,
        COUNT(*) FILTER (WHERE status = 'interviewing') AS interviewing_count,
        COUNT(*) FILTER (WHERE status = 'offer')        AS offer_count
    FROM job_feed
),
tracker_stats AS (
    SELECT
        COUNT(DISTINCT job_result_id) FILTER (
            WHERE stage IN ('phone_screen', 'technical', 'final_round')
              AND stage_at >= DATE_TRUNC('week', NOW())
        ) AS interviewing_this_week,
        -- awaiting_reply = applied jobs where tracker has no stage beyond 'applied'
        COUNT(DISTINCT j.id) FILTER (
            WHERE j.status = 'applied'
              AND NOT EXISTS (
                  SELECT 1 FROM job_application_tracker ja2
                  WHERE ja2.job_result_id = j.id
                    AND ja2.stage != 'applied'
              )
        ) AS awaiting_reply
    FROM job_application_tracker ja
    JOIN job_feed j ON ja.job_result_id = j.id
)
SELECT
    sc.applied_count,
    sc.interviewing_count,
    sc.offer_count,
    ts.awaiting_reply,
    ts.interviewing_this_week
FROM status_counts sc, tracker_stats ts
```

**Store method to add:** `GetProgressStats() (ProgressStats, error)` in a new `UserDashboardStore` interface.

---

### 2. Market Stats

**UI shows:** New jobs (since yesterday), Total open (new+saved+applied+interviewing+offer), weekly delta (+N this week)

**Response type:**

```go
type MarketStats struct {
    NewSinceYesterday int64 `json:"new_since_yesterday"`
    TotalOpen         int64 `json:"total_open"`
    AddedThisWeek     int64 `json:"added_this_week"`
    AddedLastWeek     int64 `json:"added_last_week"`
    // WeeklyDelta = AddedThisWeek - AddedLastWeek, computed in Go
    WeeklyDelta       int64 `json:"weekly_delta"`
}
```

**Optimized query — single scan of `job_feed`:**

```sql
SELECT
    COUNT(*) FILTER (
        WHERE added_at >= CURRENT_DATE - INTERVAL '1 day'
    ) AS new_since_yesterday,
    COUNT(*) FILTER (
        WHERE status IN ('new', 'saved', 'applied', 'interviewing', 'offer')
    ) AS total_open,
    COUNT(*) FILTER (
        WHERE added_at >= DATE_TRUNC('week', NOW())
    ) AS added_this_week,
    COUNT(*) FILTER (
        WHERE added_at >= DATE_TRUNC('week', NOW()) - INTERVAL '7 days'
          AND added_at <  DATE_TRUNC('week', NOW())
    ) AS added_last_week
FROM job_feed
```

`WeeklyDelta` is computed in Go: `AddedThisWeek - AddedLastWeek`.

---

### 3. Hiring Activity Heatmap (52 weeks)

**UI shows:** Grid of 52 weekly cells, colour-coded by job count. Hover tooltip shows week + count.

**Response type:**

```go
type WeeklyCount struct {
    WeekStart string `json:"week_start"` // "YYYY-MM-DD" ISO date
    JobCount  int    `json:"job_count"`
}
// Returns []WeeklyCount, up to 52 entries, ordered ASC
```

**Optimized query — single aggregate on `job_feed`:**

```sql
SELECT
    DATE_TRUNC('week', added_at)::date AS week_start,
    COUNT(*)                           AS job_count
FROM job_feed
WHERE added_at >= NOW() - INTERVAL '52 weeks'
GROUP BY DATE_TRUNC('week', added_at)
ORDER BY week_start ASC
```

Returns max 52 rows. Sparse weeks (zero jobs) are absent from results — frontend fills gaps with zero.

**Index note:** `job_feed.added_at` should already be indexed. If not, add one — this query and the market stats query both filter on it.

---

### 4. Interviewing Tracker

**UI shows:** List of active applications with company, role title, current display-stage pill. Dropdown to change stage in-place.

**Display stage mapping** (define as constants):

| Display label    | DB stages                                  |
| ---------------- | ------------------------------------------ |
| `Waiting`        | `applied`                                  |
| `Interviewing`   | `phone_screen`, `technical`, `final_round` |
| `Offer received` | `offer`                                    |

**Response type:**

```go
type InterviewRow struct {
    ApplicationID int64  `json:"application_id"`
    JobID         string `json:"job_id"`
    CompanyName   string `json:"company_name"`
    JobTitle      string `json:"job_title"`
    Stage         string `json:"stage"`         // raw DB stage
    DisplayStage  string `json:"display_stage"` // "Waiting" | "Interviewing" | "Offer received"
    StageAt       string `json:"stage_at"`      // ISO timestamp
}
```

**Query:** `GetLatestApplicationsWithJob()` already exists — use as-is. Map `Stage` → `DisplayStage` in Go at the service layer, not in SQL.

**Excludes:** Applications where latest stage is `rejected` — filter in Go after fetching.

**Stage update:** `UpdateApplicationStage(id int64, stage string)` already exists in application store. Only needs a new route wired to it.

---

### 5. Company Velocity (4-week sparklines)

**UI shows:** Top N companies ranked by current-week job count. Columns: company name, velocity badge (spike/rising/normal), 4-bar sparkline, current week new count.

**Velocity classification (computed in Go):**

```
avg_prior = (week_1 + week_2 + week_3) / 3
multiplier = week_current / avg_prior   (if avg_prior == 0, treat as 1.0)

spike  → multiplier >= 2.5
rising → multiplier >= 1.3 and < 2.5
normal → multiplier < 1.3
```

**Response type:**

```go
type CompanyVelocityRow struct {
    CompanyID    string  `json:"company_id"`
    CompanyName  string  `json:"company_name"`
    WeekCurrent  int     `json:"week_current"`
    Week1        int     `json:"week_1"`         // 1 week ago
    Week2        int     `json:"week_2"`         // 2 weeks ago
    Week3        int     `json:"week_3"`         // 3 weeks ago
    Multiplier   float64 `json:"multiplier"`     // computed in Go
    VelocityTag  string  `json:"velocity_tag"`   // "spike" | "rising" | "normal"
}
```

**Optimized query — single scan of `job_analytics`, last 28 days:**

```sql
SELECT
    c.id   AS company_id,
    c.name AS company_name,
    COUNT(*) FILTER (
        WHERE ja.snapshotted_at >= DATE_TRUNC('week', NOW())
    ) AS week_current,
    COUNT(*) FILTER (
        WHERE ja.snapshotted_at >= DATE_TRUNC('week', NOW()) - INTERVAL '7 days'
          AND ja.snapshotted_at <  DATE_TRUNC('week', NOW())
    ) AS week_1,
    COUNT(*) FILTER (
        WHERE ja.snapshotted_at >= DATE_TRUNC('week', NOW()) - INTERVAL '14 days'
          AND ja.snapshotted_at <  DATE_TRUNC('week', NOW()) - INTERVAL '7 days'
    ) AS week_2,
    COUNT(*) FILTER (
        WHERE ja.snapshotted_at >= DATE_TRUNC('week', NOW()) - INTERVAL '21 days'
          AND ja.snapshotted_at <  DATE_TRUNC('week', NOW()) - INTERVAL '14 days'
    ) AS week_3
FROM job_analytics ja
JOIN companies c ON ja.company_id = c.id
WHERE ja.snapshotted_at >= DATE_TRUNC('week', NOW()) - INTERVAL '28 days'
GROUP BY c.id, c.name
HAVING COUNT(*) FILTER (
    WHERE ja.snapshotted_at >= DATE_TRUNC('week', NOW())
) > 0
ORDER BY week_current DESC
LIMIT ?  -- configurable, default 10
```

Multiplier and velocity tag are computed in Go after scanning results.

**Replaces** `GetCompanyVelocity(limit)` in insights store — the existing method returns only two buckets.

---

### 6. Tech Stack Trends (quarterly, by company adoption)

**UI shows:** Two columns — Rising (grew >15% QoQ, >15% company adoption) and New Entrants (was <5% adoption last quarter, now >15%). Each row has tech name, 4-quarter sparkline, growth% or adoption%.

**Quarter labels in UI:** Q3 → Q4 → Q1 → Q2, where Q2 = current calendar quarter.

**Classification (computed in Go after query):**

```
total_companies = COUNT of active companies

Rising criteria:
  - q4_pct > 15%    (q4 / total_companies * 100)
  - q3 > 0          (was present last quarter — not a new entrant)
  - (q4 - q3) / q3 > 0.15   (grew more than 15% QoQ)

New entrant criteria:
  - q4_pct > 15%
  - q2_pct < 5%     (q2 = 2 quarters ago — was barely present)
```

**Response type:**

```go
type TechTrendRow struct {
    Tech        string  `json:"tech"`
    Q1          int     `json:"q1"`   // companies using it, 3 quarters ago
    Q2          int     `json:"q2"`   // 2 quarters ago
    Q3          int     `json:"q3"`   // 1 quarter ago
    Q4          int     `json:"q4"`   // current quarter
    GrowthPct   float64 `json:"growth_pct"`    // for Rising: (q4-q3)/q3 * 100
    AdoptionPct float64 `json:"adoption_pct"`  // q4 / total * 100
}

type TechTrends struct {
    TotalCompanies int            `json:"total_companies"`
    Rising         []TechTrendRow `json:"rising"`
    NewEntrants    []TechTrendRow `json:"new_entrants"`
}
```

**Optimized query — single CTE chain, one pass over `job_analytics`:**

```sql
WITH total_companies AS (
    SELECT COUNT(*) AS total FROM companies WHERE active = true
),
quarterly_unnested AS (
    SELECT
        tech,
        ja.company_id,
        CASE
            WHEN ja.snapshotted_at >= DATE_TRUNC('quarter', NOW() - INTERVAL '9 months')
             AND ja.snapshotted_at <  DATE_TRUNC('quarter', NOW() - INTERVAL '6 months')
            THEN 'q1'
            WHEN ja.snapshotted_at >= DATE_TRUNC('quarter', NOW() - INTERVAL '6 months')
             AND ja.snapshotted_at <  DATE_TRUNC('quarter', NOW() - INTERVAL '3 months')
            THEN 'q2'
            WHEN ja.snapshotted_at >= DATE_TRUNC('quarter', NOW() - INTERVAL '3 months')
             AND ja.snapshotted_at <  DATE_TRUNC('quarter', NOW())
            THEN 'q3'
            WHEN ja.snapshotted_at >= DATE_TRUNC('quarter', NOW())
            THEN 'q4'
        END AS quarter
    FROM job_analytics ja,
         jsonb_array_elements_text(ja.tech_stack) AS tech
    WHERE ja.snapshotted_at >= DATE_TRUNC('quarter', NOW() - INTERVAL '9 months')
      AND ja.tech_stack IS NOT NULL
      AND ja.tech_stack::text != 'null'
),
per_quarter_adoption AS (
    SELECT
        tech,
        quarter,
        COUNT(DISTINCT company_id) AS company_count
    FROM quarterly_unnested
    WHERE quarter IS NOT NULL
    GROUP BY tech, quarter
),
pivoted AS (
    SELECT
        tech,
        COALESCE(MAX(company_count) FILTER (WHERE quarter = 'q1'), 0) AS q1,
        COALESCE(MAX(company_count) FILTER (WHERE quarter = 'q2'), 0) AS q2,
        COALESCE(MAX(company_count) FILTER (WHERE quarter = 'q3'), 0) AS q3,
        COALESCE(MAX(company_count) FILTER (WHERE quarter = 'q4'), 0) AS q4
    FROM per_quarter_adoption
    GROUP BY tech
)
SELECT
    p.tech,
    p.q1, p.q2, p.q3, p.q4,
    t.total AS total_companies
FROM pivoted p
CROSS JOIN total_companies t
WHERE p.q4 > 0
ORDER BY p.q4 DESC
```

Go post-processing classifies each row into Rising, New Entrant, or neither, then splits into the two lists. Top 10 per list.

**Performance note for single user:** `job_analytics` will have at most a few thousand rows. The JSONB unnest is the heaviest operation here but it's negligible at this scale. No index on `tech_stack` needed now — revisit if row count exceeds 100k.

---

## New API Routes

| Method  | Path                      | Handler                           | Notes                                 |
| ------- | ------------------------- | --------------------------------- | ------------------------------------- |
| `GET`   | `/dashboard/progress`     | `UserDashboardHandler.Progress`   | My Progress card                      |
| `GET`   | `/dashboard/market`       | `UserDashboardHandler.Market`     | Market stats card                     |
| `GET`   | `/dashboard/heatmap`      | `UserDashboardHandler.Heatmap`    | 52-week activity                      |
| `GET`   | `/dashboard/interviews`   | `UserDashboardHandler.Interviews` | Interviewing tracker list             |
| `GET`   | `/dashboard/velocity`     | `UserDashboardHandler.Velocity`   | Company velocity, accepts `?limit=10` |
| `GET`   | `/dashboard/tech-trends`  | `UserDashboardHandler.TechTrends` | Quarterly tech adoption               |
| `PATCH` | `/applications/:id/stage` | `ApplicationHandler.UpdateStage`  | Body: `{"stage": "phone_screen"}`     |

**Existing routes to keep unchanged:**

- `GET /dashboard/stats` — pipeline stats (scanned today, pipeline pending/errors)
- `GET /dashboard/activity` — per-day scan activity

---

## New Store Methods Needed

All go into a new `UserDashboardStore` interface in `service/dashboard/store.go`:

| Method                                  | Returns                       | Query target                           |
| --------------------------------------- | ----------------------------- | -------------------------------------- |
| `GetProgressStats()`                    | `ProgressStats, error`        | `job_feed` + `job_application_tracker` |
| `GetMarketStats()`                      | `MarketStats, error`          | `job_feed`                             |
| `GetWeeklyHiringActivity(weeks int)`    | `[]WeeklyCount, error`        | `job_feed`                             |
| `GetCompanyVelocityDetailed(limit int)` | `[]CompanyVelocityRow, error` | `job_analytics` + `companies`          |
| `GetTechStackQuarterlyAdoption()`       | `[]techAdoptionRow, error`    | `job_analytics` + `companies`          |

`GetLatestApplicationsWithJob()` already exists in `service/application/store.go` — inject that store into the dashboard service, don't duplicate.

---

## Model Constants to Add

Add to `model/constants.go`:

```go
// JobStatus — add missing values matching migration 023
const (
    JobStatusInterviewing = "interviewing"
    JobStatusOffer        = "offer"
)

// ApplicationDisplayStage — UI-facing stage groupings
const (
    DisplayStageWaiting      = "Waiting"
    DisplayStageInterviewing = "Interviewing"
    DisplayStageOffer        = "Offer received"
)

// VelocityTag
const (
    VelocitySpike  = "spike"
    VelocityRising = "rising"
    VelocityNormal = "normal"
)
```

---

## Wire Dependencies

The new `UserDashboardService` needs:

- `UserDashboardStore` (new — owns progress, market, heatmap, velocity, tech trends queries)
- `application.Store` (existing — for interviews list)

Add both to the Wire provider set in `service/dashboard/wire.go`.
