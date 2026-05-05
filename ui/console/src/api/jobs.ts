import type { Job, JobApplication } from '@/types'
import { apiClient } from '@/lib/api-client'

export interface JobsPage {
  items: Job[]
  next_cursor: string
}

export interface JobFilters {
  status: 'new' | 'saved' | 'applied' | 'interviewing' | 'offer'
  search?: string
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

// GET /jobs — cursor-based keyset pagination (Phase 2)
// cursor="" fetches the first page; subsequent pages use next_cursor from prior response.
export async function fetchJobs(
  filters: JobFilters,
  cursor = '',
  limit = 20
): Promise<JobsPage> {
  const p = new URLSearchParams()
  p.set('status', filters.status)
  p.set('limit', String(limit))
  if (cursor) p.set('cursor', cursor)

  if (filters.search) p.set('search', filters.search)
  filters.companies?.forEach((c) => p.append('companies', c))
  if (filters.remote) p.set('remote', 'true')
  if (filters.onSite) p.set('on_site', 'true')
  filters.experienceRanges?.forEach((r) => p.append('experience_ranges', r))
  if (filters.daysAdded) p.set('days_added', String(filters.daysAdded))
  if (filters.sortBy) p.set('sort_by', filters.sortBy)
  if (filters.sortDir) p.set('sort_dir', filters.sortDir)

  return apiClient.get<JobsPage>(`/jobs?${p}`)
}

// GET /jobs/:id
export async function fetchJob(id: string): Promise<Job> {
  return apiClient.get<Job>(`/jobs/${id}`)
}

// PATCH /jobs/:id/save
export async function setJobSaved(id: string, saved: boolean): Promise<void> {
  await apiClient.patch<void>(`/jobs/${id}/save`, { saved })
}

// PATCH /jobs/:id/status
export async function setJobStatus(
  id: string,
  status: 'new' | 'saved' | 'applied' | 'interviewing' | 'offer'
): Promise<void> {
  await apiClient.patch<void>(`/jobs/${id}/status`, { status })
}

// PATCH /jobs/:id/notes
export async function setJobNotes(id: string, notes: string): Promise<void> {
  await apiClient.patch<void>(`/jobs/${id}/notes`, { notes })
}

// GET /jobs/counts
export async function fetchJobCounts(): Promise<JobCounts> {
  return apiClient.get<JobCounts>('/jobs/counts')
}

// DELETE /jobs/:id  (dismiss)
export async function dismissJob(id: string, reason: string, note?: string): Promise<void> {
  await apiClient.delete<void>(`/jobs/${id}`, { reason, note: note ?? '' })
}

// POST /jobs/dismiss/bulk
export async function bulkDismissJobs(ids: string[]): Promise<{ dismissed: number }> {
  return apiClient.post<{ dismissed: number }>('/jobs/dismiss/bulk', { ids })
}

// GET /jobs/:id/applications
export async function fetchApplications(jobId: string): Promise<JobApplication[]> {
  return apiClient.get<JobApplication[]>(`/jobs/${jobId}/applications`)
}

// POST /jobs/:id/applications
export async function createApplication(
  jobId: string,
  stage: string,
  note?: string
): Promise<void> {
  await apiClient.post<void>(`/jobs/${jobId}/applications`, { stage, note: note ?? '' })
}
