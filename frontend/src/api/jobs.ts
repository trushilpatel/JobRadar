import type { Job, JobApplication } from '@/types'

const BASE = '/api'

export interface JobsPage {
  data: Job[]
  total: number
  page: number
  limit: number
}

export interface JobFilters {
  status: 'new' | 'saved' | 'applied'
  companies?: string[]
  remote?: boolean
  employTypes?: string[]
  sponsorship?: boolean
  noRequirements?: boolean
  requiresCitizen?: boolean
  experienceRanges?: string[]
  daysAdded?: number
}

// GET /jobs — all filters pushed to server, no client-side filtering
export async function fetchJobs(
  filters: JobFilters,
  page = 1,
  limit = 20,
): Promise<JobsPage> {
  const p = new URLSearchParams()
  p.set('status', filters.status)
  p.set('page', String(page))
  p.set('limit', String(limit))

  filters.companies?.forEach(c => p.append('companies', c))
  if (filters.remote) p.set('remote', 'true')
  filters.employTypes?.forEach(t => p.append('employ_types', t))
  if (filters.sponsorship !== undefined) p.set('sponsorship', String(filters.sponsorship))
  if (filters.noRequirements) p.set('no_requirements', 'true')
  if (filters.requiresCitizen !== undefined) p.set('requires_citizen', String(filters.requiresCitizen))
  filters.experienceRanges?.forEach(r => p.append('experience_ranges', r))
  if (filters.daysAdded) p.set('days_added', String(filters.daysAdded))

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
export async function setJobStatus(id: string, status: 'new' | 'saved' | 'applied'): Promise<void> {
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
export async function createApplication(jobId: string, stage: string, note?: string): Promise<void> {
  const res = await fetch(`${BASE}/jobs/${jobId}/applications`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ stage, note: note ?? '' }),
  })
  if (!res.ok) throw new Error('Failed to create application stage')
}
