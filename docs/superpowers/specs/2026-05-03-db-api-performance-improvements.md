# DB & API Performance Improvements

**Date:** 2026-05-03

## Changes

### `backend/service/dashboard/services/DashboardImplService.go`
`GetStats()` now runs `GetJobsScannedToday`, `GetJobCounts`, and `GetPipelineCounts` concurrently via `errgroup.WithContext`. Previously sequential; now completes in the time of the slowest query instead of the sum.

### `backend/service/company/services/CompanyRepositoryService.go`
- `repositoryService.List()` now runs `GetCompanies` and `GetCompanyCounts` concurrently via `errgroup` — eliminates a sequential second full-table scan on every company list request.
- `companyStore.GetCompaniesByIDs` added — fetches companies by a slice of IDs (`WHERE id IN (?)`); returns empty slice when input is empty.
- `repositoryService.GetByIDs` added — maps store results to `CompanyView` slice.

### `backend/service/company/services/CompanyService.go`
`GetByIDs(ctx, ids []string) ([]CompanyView, error)` added to the `Service` interface and `BaseService` delegation.

### `backend/service/scan/services/ScanImplService.go`
`ListQueue()` now collects unique company IDs from queue entries and calls `companySvc.GetByIDs` instead of `companySvc.GetAll`. Eliminates a full companies-table fetch when enriching a small queue.

### `backend/service/scan/services/ScanRepositoryService.go`
- `ScanQueueStore.EnqueueAll` added — single bulk `INSERT ... SELECT` that enqueues all active companies not already queued/running.
- `repositoryService.TriggerScanAll` implemented using `EnqueueAll`; removed the N-INSERT loop that previously lived in `ScanImplService`.

### `backend/service/insights/services/InsightsRepositoryService.go`
Added `LIMIT` caps to three unbounded queries: `CompanyActivity` (200), `Location` countries (50), `RoleCategories` (25).

### `backend/service/application/services/ApplicationRepositoryService.go`
Added `Limit(100)` safety ceiling to `getApplicationsByJobID`.

### `backend/service/job/services/JobFeedRepository.go`
Replaced `.Preload("Company")` with `.Joins("Company")` on both `GetJobs` and `GetJobByID`. Fetches job + company data in one SQL JOIN instead of two round-trips. JSON output is unchanged.

### `backend/migrations/041_job_analytics_perf_indexes.sql`
New migration adding:
- `idx_job_analytics_role_category` on `job_analytics(role_category)` — supports `GROUP BY role_category` in `RoleCategories`.
- `idx_job_analytics_state` partial index on `job_analytics(state) WHERE state IS NOT NULL` — supports `WHERE country = 'USA' AND state IS NOT NULL` in `Location`.
