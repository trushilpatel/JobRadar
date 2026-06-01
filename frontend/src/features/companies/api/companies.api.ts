import { z } from 'zod'
import type { ScanStats, CompanyAnalytics } from '@/types'
import { apiClient } from '@/lib/core/api-client'

// ── Zod schemas (response shapes only) ─────────────────────────────────────

const ScanRunSchema = z.object({
  company_id: z.string(),
  scanned_at: z.string(),
  jobs_fetched: z.number(),
  jobs_new: z.number(),
  error: z.string().nullable(),
  duration_ms: z.number().nullable(),
})

const ScanStatsSchema = z.object({
  total_scans: z.number(),
  total_fetched: z.number(),
  total_new: z.number(),
  error_count: z.number(),
  avg_duration_ms: z.number().nullable(),
  new_today: z.number(),
  new_this_month: z.number(),
  recent_runs: z.array(ScanRunSchema),
})

const MonthlyJobPointSchema = z.object({
  month: z.string(),
  new: z.number(),
  removed: z.number(),
})

const CompanyAnalyticsSchema = z.object({
  monthly: z.array(MonthlyJobPointSchema),
  velocity: z.object({
    this_week: z.number(),
    last_week: z.number(),
  }),
})

const TriggerScanAllResultSchema = z.object({ queued: z.number() })

// ── API functions ────────────────────────────────────────────────────────────
//
// Company CRUD + listing live in `adminCompanies.api.ts` (the `/admin/companies`
// chain). This file only owns the scan-oriented read/trigger endpoints exposed
// under `/api/companies/*` by the scan service.

export async function fetchScanStats(id: string): Promise<ScanStats> {
  const raw = await apiClient.get<unknown>(`/companies/${encodeURIComponent(id)}/scan-stats`)
  return ScanStatsSchema.parse(raw) as ScanStats
}

export async function triggerScan(id: string): Promise<void> {
  await apiClient.post<void>(`/companies/${encodeURIComponent(id)}/scan`)
}

export async function triggerScanAll(): Promise<{ queued: number }> {
  const raw = await apiClient.post<unknown>('/companies/scan-all')
  return TriggerScanAllResultSchema.parse(raw)
}

export async function fetchCompanyAnalytics(id: string): Promise<CompanyAnalytics> {
  const raw = await apiClient.get<unknown>(`/companies/${encodeURIComponent(id)}/analytics`)
  return CompanyAnalyticsSchema.parse(raw) as CompanyAnalytics
}
