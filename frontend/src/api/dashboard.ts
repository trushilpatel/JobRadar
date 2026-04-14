import type { DashboardStats, WeeklyDataPoint } from '@/types'

const BASE = '/api'

const DAY_NAMES = ['Sun', 'Mon', 'Tue', 'Wed', 'Thu', 'Fri', 'Sat']

export async function fetchDashboardStats(): Promise<DashboardStats> {
  const res = await fetch(`${BASE}/dashboard/stats`)
  if (!res.ok) throw new Error('Failed to fetch dashboard stats')
  const { data: raw } = await res.json()
  return {
    scanned_today:    raw.jobs_scanned_today ?? 0,
    jobs_new:         raw.jobs_new ?? 0,
    saved:            raw.jobs_saved ?? 0,
    applied:          raw.jobs_applied ?? 0,
    pipeline_pending: raw.pipeline_pending ?? 0,
    pipeline_errors:  raw.pipeline_errors ?? 0,
  }
}

export async function fetchDashboardActivity(days = 7): Promise<WeeklyDataPoint[]> {
  const res = await fetch(`${BASE}/dashboard/activity?days=${days}`)
  if (!res.ok) throw new Error('Failed to fetch dashboard activity')
  const { data: raw }: { data: Array<{ date: string; scanned: number; new_jobs: number }> } = await res.json()
  return (raw ?? []).map(d => ({
    // Use noon UTC to avoid date-boundary timezone shifts
    day: DAY_NAMES[new Date(d.date + 'T12:00:00Z').getDay()],
    scanned: d.scanned,
    relevant: d.new_jobs,
  }))
}
