import type { Job, JobApplication } from '@/types'

const BASE = '/api'

export interface JobsPage {
  data: Job[]
  total: number
  page: number
  limit: number
}

// GET /jobs?status=new|saved|applied&page=N&limit=N
export async function fetchJobs(
  status: 'new' | 'saved' | 'applied',
  page = 1,
  limit = 20,
): Promise<JobsPage> {
  const res = await fetch(`${BASE}/jobs?status=${status}&page=${page}&limit=${limit}`)
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
