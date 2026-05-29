import { z } from 'zod'
import type { Company, ScanStats, QueueEntry, CompanyAnalytics } from '@/types'
import { apiClient } from '@/lib/api-client'

// ── Zod schemas (response shapes only) ─────────────────────────────────────

const CompanySchema = z
  .object({
    id: z.string(),
    name: z.string(),
    url: z.string(),
    adapter_type: z.string(),
    active: z.boolean(),
    created_at: z.string(),
    last_scan_at: z.string().nullable(),
    last_scan_status: z.enum(['ok', 'error', 'pending', 'running', 'queued']).nullable(),
    queue_status: z.string().optional(),
    scan_history: z.array(z.boolean()).optional(),
  })
  .passthrough()

const ScanRunSchema = z.object({
  id: z.number(),
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

const QueueEntrySchema = z.object({
  id: z.string(),
  company_id: z.string(),
  company_name: z.string(),
  adapter_type: z.string(),
  status: z.enum(['queued', 'running']),
  source: z.enum(['scheduler', 'manual']),
  priority: z.number(),
  queued_at: z.string(),
  started_at: z.string().nullable(),
  next_scan_at: z.string().nullable(),
})

const CompaniesPageSchema = z.object({
  data: z.array(CompanySchema),
  total: z.number(),
  total_active: z.number(),
  page: z.number(),
  limit: z.number(),
})

const TriggerScanAllResultSchema = z.object({ queued: z.number() })

// ── Input types (not validated — sent to backend) ───────────────────────────

export interface CompanyFilters {
  active?: boolean // true = active only, false = inactive only, undefined = all
  adapterTypes?: string[] // e.g. ['greenhouse', 'lever']
  search?: string // ILIKE name match
  lastScanStatus?: string // e.g. 'error'
  sortBy?: string
  sortDir?: 'asc' | 'desc'
}

export type CompaniesPage = z.infer<typeof CompaniesPageSchema>

// ── API functions ────────────────────────────────────────────────────────────

export async function fetchCompanies(
  filters: CompanyFilters = {},
  page = 1,
  limit = 20
): Promise<CompaniesPage> {
  const p = new URLSearchParams()
  p.set('page', String(page))
  p.set('limit', String(limit))
  if (filters.active !== undefined) p.set('active', String(filters.active))
  filters.adapterTypes?.forEach((at) => p.append('adapter_types', at))
  if (filters.search) p.set('search', filters.search)
  if (filters.lastScanStatus) p.set('last_scan_status', filters.lastScanStatus)
  if (filters.sortBy) p.set('sort_by', filters.sortBy)
  if (filters.sortDir) p.set('sort_dir', filters.sortDir)

  const raw = await apiClient.get<unknown>(`/companies?${p}`)
  return CompaniesPageSchema.parse(raw) as CompaniesPage
}

export async function createCompany(data: {
  id: string
  name: string
  url: string
  adapter_type: string
  active: boolean
}): Promise<Company> {
  return apiClient.post<Company>('/companies', data)
}

export async function updateCompany(
  id: string,
  fields: Partial<Pick<Company, 'name' | 'url' | 'adapter_type' | 'active'>>
): Promise<void> {
  await apiClient.patch<void>(`/companies/${encodeURIComponent(id)}`, fields)
}

export async function deleteCompany(id: string): Promise<void> {
  await apiClient.delete<void>(`/companies/${encodeURIComponent(id)}`)
}

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

export async function setAllCompaniesActive(active: boolean): Promise<void> {
  await apiClient.patch<void>('/companies/active', { active })
}

export async function fetchQueue(): Promise<QueueEntry[]> {
  const raw = await apiClient.get<unknown>('/companies/queue')
  return z.array(QueueEntrySchema).parse(raw) as QueueEntry[]
}

export async function removeFromQueue(entryId: string): Promise<void> {
  await apiClient.delete<void>(`/companies/queue/${entryId}`)
}

export async function fetchCompanyAnalytics(id: string): Promise<CompanyAnalytics> {
  const raw = await apiClient.get<unknown>(`/companies/${encodeURIComponent(id)}/analytics`)
  return CompanyAnalyticsSchema.parse(raw) as CompanyAnalytics
}
