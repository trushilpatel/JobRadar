# Tech Stack Trends — Monthly Redesign

Date: 2026-05-29

## Problem

The `TechTrendsPanel` on the dashboard shows only one entry in practice. Three compounding causes:

1. **Wrong denominator.** `total_jobs` is computed as `SUM(per_tech distinct_job_count) OVER ()` — a window sum that counts a job once per tech tag it carries. A job with 5 tech tags inflates the denominator 5×, making every percentage threshold 5× harder to clear.
2. **Quarterly cadence is too slow.** A live job feed produces meaningful month-over-month signal. Quarter-over-quarter requires months of lag before trends surface.
3. **Prevalence threshold is too high.** The 5% gate eliminates all but the most dominant technologies.

Secondary: the subtitle "BY COMPANY ADOPTION" is wrong — the query counts job postings, not distinct companies.

## Goals

- Surface 5–15 rising and new-entrant technologies per panel render.
- Switch to monthly cadence.
- Fix the denominator to count unique jobs, not tag-weighted jobs.
- Use job count (demand volume) as the metric — consistent with the feature's actual data.
- Improve classification quality: weight growth by volume, enforce an absolute floor.
- Add index + cache so the query doesn't degrade as `job_tech_tags` grows.
- Normalize tech names at write time to prevent "React" / "React.js" / "ReactJS" fragmenting counts.

## Out of scope

- Backfilling existing `job_tech_tags` rows with normalized names (separate cleanup task).
- Fading/declining tech detection.
- User-level filtering.

---

## Data layer

### Migration

Add a composite index on `job_tech_tags`:

```sql
CREATE INDEX idx_job_tech_tags_tagged_at_tech ON job_tech_tags (tagged_at, tech);
```

This makes the monthly `GROUP BY tech` with date-range filters index-only for the aggregate, avoiding a full table scan as the table grows.

### New repository method

Replace `GetTechStackQuarterlyAdoption` with `GetTechStackMonthlyTrends` in `DashboardRepositoryService.go`.

Query shape:

```sql
WITH period_base AS (
  SELECT COUNT(DISTINCT job_id) AS total_jobs
  FROM job_tech_tags
  WHERE tagged_at >= DATE_TRUNC('month', NOW())
),
monthly_counts AS (
  SELECT
    tech,
    COUNT(*) FILTER (WHERE tagged_at >= DATE_TRUNC('month', NOW() - INTERVAL '3 months')
                       AND tagged_at <  DATE_TRUNC('month', NOW() - INTERVAL '2 months')) AS m1,
    COUNT(*) FILTER (WHERE tagged_at >= DATE_TRUNC('month', NOW() - INTERVAL '2 months')
                       AND tagged_at <  DATE_TRUNC('month', NOW() - INTERVAL '1 month'))  AS m2,
    COUNT(*) FILTER (WHERE tagged_at >= DATE_TRUNC('month', NOW() - INTERVAL '1 month')
                       AND tagged_at <  DATE_TRUNC('month', NOW()))                        AS m3,
    COUNT(*) FILTER (WHERE tagged_at >= DATE_TRUNC('month', NOW()))                        AS m_current
  FROM job_tech_tags
  WHERE tagged_at >= DATE_TRUNC('month', NOW() - INTERVAL '3 months')
  GROUP BY tech
)
SELECT
  mc.tech,
  mc.m1, mc.m2, mc.m3, mc.m_current,
  pb.total_jobs
FROM monthly_counts mc
CROSS JOIN period_base pb
WHERE mc.m_current > 0
ORDER BY mc.m_current DESC
```

`total_jobs` is a single scalar from `period_base` — one `COUNT(DISTINCT job_id)` for the current calendar month only. Not a window sum.

### Internal DTO

Rename `TechAdoptionRow` → `TechMonthlyRow` in `dashboard/dto/TechAdoption.go`:

```go
type TechMonthlyRow struct {
    Tech      string
    M1        int  // 3 months ago
    M2        int  // 2 months ago
    M3        int  // last month
    MCurrent  int  // this month
    TotalJobs int  // unique jobs this month (denominator)
}
```

### External DTO (API response)

Rename `Q1/Q2/Q3/Q4` → `M1/M2/M3/M4` in `dashboard/dto/TechTrends.go` and JSON tags:

```go
type TechTrendRow struct {
    Tech        string  `json:"tech"`
    M1          int     `json:"m1"`           // 3 months ago
    M2          int     `json:"m2"`           // 2 months ago
    M3          int     `json:"m3"`           // last month
    M4          int     `json:"m4"`           // this month
    GrowthPct   float64 `json:"growth_pct"`   // (m4-m3)/m3 * 100; 0 when IsNew=true
    AdoptionPct float64 `json:"adoption_pct"` // m4 / total_jobs * 100
    IsNew       bool    `json:"is_new"`       // true when m3=0 (no prior-month baseline)
}
```

`TechTrends` envelope is unchanged: `{ total_jobs, rising, new_entrants }`.

---

## Classification logic (`GetTechTrends` in `DashboardUserImplService.go`)

Updated thresholds and sort:

| Signal               | Condition                                                                        |
| -------------------- | -------------------------------------------------------------------------------- |
| Prevalence gate      | `m4_pct > 2%` AND `m_current >= 10 jobs`                                         |
| Rising               | Passes prevalence gate AND `m3 > 0` AND `(m4 - m3) / m3 > 0.15`                  |
| Rising (new variant) | Passes prevalence gate AND `m3 == 0` (no prior month baseline) → `is_new = true` |
| New Entrant          | Passes prevalence gate AND `m2_pct < 1%` (near-zero two months ago)              |

Sort order for Rising: `growth_pct × log(m_current + 1)` descending — volume-weighted momentum. A tech that grew 30% from 200→260 jobs ranks above one that grew 100% from 5→10. Raw growth percentage without volume weighting surfaces noise.

Sort order for New Entrants: `m_current` descending (largest absolute presence first).

Cap: 10 rows per list (unchanged).

---

## Cache

Wire `GetTechTrends` through the dashboard cache decorator with a **1-hour TTL**. Tech trends are a global (non-user-scoped) query — one cache key per service instance. Cache key: `dashboard:tech-trends`. Invalidation: time-based only (no write-path trigger needed).

This follows the existing decorator chain pattern used in `service/user/`. Add a `DashboardCacheService` wrapping `UserDashboardService` if it does not already exist, or extend the existing one.

---

## Tech name normalization

Add a `normalizeTech(tech string) string` pure function in `service/job/common/` (or inline in `JobPipelineImplService`). Applied to every element of the `techs []string` slice before `InsertTechTags` writes to `job_tech_tags`.

Canonical map covers the most common LLM variants:

| Raw                  | Canonical  |
| -------------------- | ---------- |
| golang, go lang      | Go         |
| react.js, reactjs    | React      |
| node.js, nodejs      | Node.js    |
| typescript           | TypeScript |
| javascript           | JavaScript |
| postgres, postgresql | PostgreSQL |
| k8s                  | Kubernetes |
| aws lambda           | AWS Lambda |

Map is lowercase-keyed; input is lowercased before lookup; original casing is preserved on miss.

Applies to new writes only. Existing rows in `job_tech_tags` are not backfilled in this change.

---

## Frontend

### `types/index.ts`

Rename `q1/q2/q3/q4 → m1/m2/m3/m4` in `TechTrendRow`.

### `TechTrendsPanel.tsx`

| Location                 | Old                                       | New                                     |
| ------------------------ | ----------------------------------------- | --------------------------------------- |
| Sparkline values         | `[row.q1, row.q2, row.q3, row.q4]`        | `[row.m1, row.m2, row.m3, row.m4]`      |
| Column header labels     | `['Q3','Q4','Q1','Q2']`                   | `['M-3','M-2','M-1','Now']`             |
| Panel subtitle           | `tech stack trends — by company adoption` | `tech stack trends — by job demand`     |
| Rising description       | `>5% of jobs · grew >15% vs last quarter` | `>2% of jobs · grew >15% vs last month` |
| New Entrants description | `>5% of jobs · was <1% two quarters ago`  | `>2% of jobs · was <1% two months ago`  |

No structural changes to `TechRow`, `TechSparkline`, or `ColHeader` components.

---

## Testing

### Backend

- `DashboardRepositoryService_test.go`: replace quarterly test fixtures with monthly fixtures; verify `total_jobs` equals `COUNT(DISTINCT job_id)` for current month, not sum of per-tech counts.
- `DashboardUserImplService_test.go`: cover prevalence gate (2%), absolute floor (10 jobs), momentum-weighted sort order, new-entrant two-month lookback, rising with and without prior-month baseline.
- `normalizeTech` unit tests: known variants map to canonical, unknown pass through unchanged, empty string is a no-op.

### Frontend

- `TechTrendsPanel.test.tsx`: update field references from `q1–q4` to `m1–m4`; verify updated subtitle and column header labels render correctly; existing four-state coverage (loading, populated, empty, error) remains.

---

## Files touched

| File                                                                   | Change                                                                   |
| ---------------------------------------------------------------------- | ------------------------------------------------------------------------ |
| `backend/service/dashboard/dto/TechAdoption.go`                        | Rename to `TechMonthlyRow`, rename fields                                |
| `backend/service/dashboard/dto/TechTrends.go`                          | Rename `Q1–Q4 → M1–M4` in `TechTrendRow`                                 |
| `backend/service/dashboard/services/DashboardRepositoryService.go`     | Replace `GetTechStackQuarterlyAdoption` with `GetTechStackMonthlyTrends` |
| `backend/service/dashboard/services/DashboardUserImplService.go`       | Update `GetTechTrends` thresholds, floor, momentum sort                  |
| `backend/service/dashboard/services/DashboardService.go`               | Update interface method signature                                        |
| `backend/service/dashboard/tests/DashboardRepositoryService_test.go`   | Update test fixtures                                                     |
| `backend/service/dashboard/tests/DashboardUserImplService_test.go`     | Update threshold + sort tests                                            |
| `backend/service/job/common/` (new file)                               | `tech_normalize.go` — `normalizeTech` + map                              |
| `backend/service/job/services/JobPipelineImplService.go`               | Call `normalizeTech` before `InsertTechTags`                             |
| `backend/service/job/tests/JobPipelineImplService_test.go`             | Cover normalization in tag-write path                                    |
| New migration                                                          | `CREATE INDEX idx_job_tech_tags_tagged_at_tech`                          |
| `ui/console/src/types/index.ts`                                        | Rename `q1–q4 → m1–m4` in `TechTrendRow`                                 |
| `ui/console/src/features/dashboard/components/TechTrendsPanel.tsx`     | Labels, subtitle, field refs                                             |
| `ui/console/src/features/dashboard/__tests__/TechTrendsPanel.test.tsx` | Update field refs and label assertions                                   |
