import type {
  DashboardStats,
  WeeklyDataPoint,
  ProgressStats,
  MarketStats,
  WeeklyCount,
  InterviewRow,
  CompanyVelocityRow,
  TechTrends,
  PipelineStats,
  RetryTierCounts,
} from '@/types'
import { apiClient } from '@/lib/api-client'

const DAY_NAMES = ['Sun', 'Mon', 'Tue', 'Wed', 'Thu', 'Fri', 'Sat']

interface DashboardStatsRaw {
  jobs_scanned_today?: number
  jobs_new?: number
  jobs_saved?: number
  jobs_applied?: number
  pipeline_pending?: number
  pipeline_errors?: number
}

export async function fetchDashboardStats(): Promise<DashboardStats> {
  const raw = await apiClient.get<DashboardStatsRaw>('/dashboard/stats')
  return {
    scanned_today: raw.jobs_scanned_today ?? 0,
    jobs_new: raw.jobs_new ?? 0,
    saved: raw.jobs_saved ?? 0,
    applied: raw.jobs_applied ?? 0,
    pipeline_pending: raw.pipeline_pending ?? 0,
    pipeline_errors: raw.pipeline_errors ?? 0,
  }
}

export async function fetchDashboardActivity(days = 7): Promise<WeeklyDataPoint[]> {
  const raw = await apiClient.get<Array<{ date: string; scanned: number; new_jobs: number }>>(
    `/dashboard/activity?days=${days}`
  )
  return (raw ?? []).map((d) => ({
    // Use noon UTC to avoid date-boundary timezone shifts
    day: DAY_NAMES[new Date(d.date + 'T12:00:00Z').getDay()],
    scanned: d.scanned,
    relevant: d.new_jobs,
  }))
}

export async function fetchProgress(): Promise<ProgressStats> {
  const data = await apiClient.get<Partial<ProgressStats> | null>('/dashboard/progress')
  return {
    applied_count: data?.applied_count ?? 0,
    awaiting_reply: data?.awaiting_reply ?? 0,
    interviewing_count: data?.interviewing_count ?? 0,
    interviewing_this_week: data?.interviewing_this_week ?? 0,
    offer_count: data?.offer_count ?? 0,
  }
}

export async function fetchMarket(): Promise<MarketStats> {
  const data = await apiClient.get<Partial<MarketStats> | null>('/dashboard/market')
  return {
    new_since_yesterday: data?.new_since_yesterday ?? 0,
    total_open: data?.total_open ?? 0,
    added_this_week: data?.added_this_week ?? 0,
    added_last_week: data?.added_last_week ?? 0,
    weekly_delta: data?.weekly_delta ?? 0,
  }
}

export async function fetchHeatmap(): Promise<WeeklyCount[]> {
  const data = await apiClient.get<WeeklyCount[] | null>('/dashboard/heatmap')
  return data ?? []
}

export async function fetchInterviews(): Promise<InterviewRow[]> {
  const data = await apiClient.get<InterviewRow[] | null>('/dashboard/interviews')
  return data ?? []
}

export async function fetchVelocity(limit = 10): Promise<CompanyVelocityRow[]> {
  const data = await apiClient.get<CompanyVelocityRow[] | null>(
    `/dashboard/velocity?limit=${limit}`
  )
  return data ?? []
}

export async function fetchTechTrends(): Promise<TechTrends> {
  const data = await apiClient.get<Partial<TechTrends> | null>('/dashboard/tech-trends')
  return {
    total_jobs: data?.total_jobs ?? 0,
    rising: data?.rising ?? [],
    new_entrants: data?.new_entrants ?? [],
  }
}

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

export async function triggerRetry(
  worker: 'filter' | 'enricher',
  tier: '1m' | '10m' | '1h' | 'dlq'
): Promise<{ drained: number }> {
  return apiClient.post<{ drained: number }>('/admin/dashboard/pipeline/retry', { worker, tier })
}

export async function updateApplicationStage(id: number, stage: string): Promise<void> {
  await apiClient.patch<void>(`/applications/${id}/stage`, { stage })
}
