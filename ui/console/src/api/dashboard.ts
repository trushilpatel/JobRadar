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
} from '@/types'

const BASE = '/api'

const DAY_NAMES = ['Sun', 'Mon', 'Tue', 'Wed', 'Thu', 'Fri', 'Sat']

export async function fetchDashboardStats(): Promise<DashboardStats> {
  const res = await fetch(`${BASE}/dashboard/stats`)
  if (!res.ok) throw new Error('Failed to fetch dashboard stats')
  const { data: raw } = await res.json()
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
  const res = await fetch(`${BASE}/dashboard/activity?days=${days}`)
  if (!res.ok) throw new Error('Failed to fetch dashboard activity')
  const { data: raw }: { data: Array<{ date: string; scanned: number; new_jobs: number }> } =
    await res.json()
  return (raw ?? []).map((d) => ({
    // Use noon UTC to avoid date-boundary timezone shifts
    day: DAY_NAMES[new Date(d.date + 'T12:00:00Z').getDay()],
    scanned: d.scanned,
    relevant: d.new_jobs,
  }))
}

export async function fetchProgress(): Promise<ProgressStats> {
  const res = await fetch(`${BASE}/dashboard/progress`)
  if (!res.ok) throw new Error('Failed to fetch progress stats')
  const { data } = await res.json()
  return {
    applied_count: data?.applied_count ?? 0,
    awaiting_reply: data?.awaiting_reply ?? 0,
    interviewing_count: data?.interviewing_count ?? 0,
    interviewing_this_week: data?.interviewing_this_week ?? 0,
    offer_count: data?.offer_count ?? 0,
  }
}

export async function fetchMarket(): Promise<MarketStats> {
  const res = await fetch(`${BASE}/dashboard/market`)
  if (!res.ok) throw new Error('Failed to fetch market stats')
  const { data } = await res.json()
  return {
    new_since_yesterday: data?.new_since_yesterday ?? 0,
    total_open: data?.total_open ?? 0,
    added_this_week: data?.added_this_week ?? 0,
    added_last_week: data?.added_last_week ?? 0,
    weekly_delta: data?.weekly_delta ?? 0,
  }
}

export async function fetchHeatmap(): Promise<WeeklyCount[]> {
  const res = await fetch(`${BASE}/dashboard/heatmap`)
  if (!res.ok) throw new Error('Failed to fetch heatmap data')
  const { data } = await res.json()
  return data ?? []
}

export async function fetchInterviews(): Promise<InterviewRow[]> {
  const res = await fetch(`${BASE}/dashboard/interviews`)
  if (!res.ok) throw new Error('Failed to fetch interviews')
  const { data } = await res.json()
  return data ?? []
}

export async function fetchVelocity(limit = 10): Promise<CompanyVelocityRow[]> {
  const res = await fetch(`${BASE}/dashboard/velocity?limit=${limit}`)
  if (!res.ok) throw new Error('Failed to fetch velocity data')
  const { data } = await res.json()
  return data ?? []
}

export async function fetchTechTrends(): Promise<TechTrends> {
  const res = await fetch(`${BASE}/dashboard/tech-trends`)
  if (!res.ok) throw new Error('Failed to fetch tech trends')
  const { data } = await res.json()
  return {
    total_jobs: data?.total_jobs ?? 0,
    rising: data?.rising ?? [],
    new_entrants: data?.new_entrants ?? [],
  }
}

export async function fetchPipelineStats(): Promise<PipelineStats> {
  const res = await fetch(`${BASE}/dashboard/pipeline`)
  if (!res.ok) throw new Error('Failed to fetch pipeline stats')
  const { data } = await res.json()
  return {
    total_companies: data?.total_companies ?? 0,
    active_companies: data?.active_companies ?? 0,
    queued: data?.queued ?? 0,
    crawling: data?.crawling ?? 0,
    awaiting_filter: data?.awaiting_filter ?? 0,
    awaiting_enrichment: data?.awaiting_enrichment ?? 0,
    new_jobs_24h: data?.new_jobs_24h ?? 0,
    errors_24h: data?.errors_24h ?? 0,
  }
}

export async function updateApplicationStage(id: number, stage: string): Promise<void> {
  const res = await fetch(`${BASE}/applications/${id}/stage`, {
    method: 'PATCH',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ stage }),
  })
  if (!res.ok) throw new Error('Failed to update stage')
}
