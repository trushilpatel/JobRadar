import type { Job, JobApplication } from '@/types'

const BASE = '/api'

export interface JobsPage {
  data: Job[]
  total: number
  page: number
  limit: number
}

export interface JobFilters {
  status: 'new' | 'saved' | 'applied' | 'interviewing' | 'offer'
  companies?: string[]
  remote?: boolean
  onSite?: boolean
  experienceRanges?: string[]
  daysAdded?: number
  sortBy?: string
  sortDir?: 'asc' | 'desc'
}

export interface JobCounts {
  new: number
  saved: number
  applied: number
  interviewing: number
  offer: number
}

// GET /jobs — all filters pushed to server, no client-side filtering
export async function fetchJobs(filters: JobFilters, page = 1, limit = 20): Promise<JobsPage> {
  const p = new URLSearchParams()
  p.set('status', filters.status)
  p.set('page', String(page))
  p.set('limit', String(limit))

  filters.companies?.forEach((c) => p.append('companies', c))
  if (filters.remote) p.set('remote', 'true')
  if (filters.onSite) p.set('on_site', 'true')
  filters.experienceRanges?.forEach((r) => p.append('experience_ranges', r))
  if (filters.daysAdded) p.set('days_added', String(filters.daysAdded))
  if (filters.sortBy) p.set('sort_by', filters.sortBy)
  if (filters.sortDir) p.set('sort_dir', filters.sortDir)

  const res = await fetch(`${BASE}/jobs?${p}`)
  if (!res.ok) throw new Error('Failed to fetch jobs')
  return res.json()
}

// GET /jobs/:id
export async function fetchJob(id: string): Promise<Job> {
  const res = await fetch(`${BASE}/jobs/${id}`)
  if (!res.ok) throw new Error('Failed to fetch job')
  return res.json().then((r: { data: Job }) => r.data)
}

// PATCH /jobs/:id/save
export async function setJobSaved(id: string, saved: boolean): Promise<void> {
  const res = await fetch(`${BASE}/jobs/${id}/save`, {
    method: 'PATCH',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ saved }),
  })
  if (!res.ok) throw new Error('Failed to update job')
}

// PATCH /jobs/:id/status
export async function setJobStatus(
  id: string,
  status: 'new' | 'saved' | 'applied' | 'interviewing' | 'offer'
): Promise<void> {
  const res = await fetch(`${BASE}/jobs/${id}/status`, {
    method: 'PATCH',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ status }),
  })
  if (!res.ok) throw new Error('Failed to update job status')
}

// PATCH /jobs/:id/notes
export async function setJobNotes(id: string, notes: string): Promise<void> {
  const res = await fetch(`${BASE}/jobs/${id}/notes`, {
    method: 'PATCH',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ notes }),
  })
  if (!res.ok) throw new Error('Failed to update notes')
}

// GET /jobs/counts
export async function fetchJobCounts(): Promise<JobCounts> {
  const res = await fetch(`${BASE}/jobs/counts`)
  if (!res.ok) throw new Error('Failed to fetch job counts')
  return res.json().then((r: { data: JobCounts }) => r.data)
}

// DELETE /jobs/:id  (dismiss)
export async function dismissJob(id: string, reason: string, note?: string): Promise<void> {
  const res = await fetch(`${BASE}/jobs/${id}`, {
    method: 'DELETE',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ reason, note: note ?? '' }),
  })
  if (!res.ok) throw new Error('Failed to dismiss job')
}

// GET /jobs/:id/applications
export async function fetchApplications(jobId: string): Promise<JobApplication[]> {
  const res = await fetch(`${BASE}/jobs/${jobId}/applications`)
  if (!res.ok) throw new Error('Failed to fetch applications')
  return res.json().then((r: { data: JobApplication[] }) => r.data)
}

// POST /jobs/:id/applications
export async function createApplication(
  jobId: string,
  stage: string,
  note?: string
): Promise<void> {
  const res = await fetch(`${BASE}/jobs/${jobId}/applications`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ stage, note: note ?? '' }),
  })
  if (!res.ok) throw new Error('Failed to create application stage')
}
