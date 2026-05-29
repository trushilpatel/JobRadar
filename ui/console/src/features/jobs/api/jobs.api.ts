import { z } from 'zod'
import type { Job, JobApplication } from '@/types'
import { apiClient } from '@/lib/core/api-client'
import { JOB_STATUS, type JobStatus, type PatchableStatus } from '@/constants/jobStatus'

// ── Zod schemas (response shapes only) ─────────────────────────────────────

const UserJobStateSchema = z
  .object({
    status: z.string().nullable(),
    notes: z.string().nullable(),
    saved_at: z.string().nullable(),
    applied_at: z.string().nullable(),
    dismissed_at: z.string().nullable(),
    dismissal_reason: z.string().nullable(),
    dismissal_note: z.string().nullable(),
  })
  .partial()

const CompanyRefSchema = z
  .object({
    id: z.string(),
    name: z.string(),
    url: z.string(),
    adapter_type: z.string(),
    active: z.boolean(),
    created_at: z.string(),
    last_scan_at: z.string().nullable(),
    last_scan_status: z.enum(['ok', 'error', 'pending', 'running', 'queued']).nullable(),
  })
  .passthrough()

const JobRawSchema = z.object({
  id: z.string(),
  job_post_id: z.string(),
  company_id: z.string(),
  title: z.string(),
  location: z.string().nullable(),
  remote: z.boolean(),
  url: z.string(),
  department: z.string().nullable(),
  employ_type: z.string().nullable(),
  experience_years: z.number().nullable(),
  tech_stack: z.array(z.string()).nullable(),
  salary_range: z
    .object({
      min: z.number().optional(),
      max: z.number().optional(),
      currency: z.string().optional(),
      period: z.string().optional(),
    })
    .nullable(),
  llm_summary: z.string().nullable(),
  sponsorship: z.boolean(),
  requires_citizen: z.boolean(),
  security_clearance: z.boolean(),
  role_brief: z.string().nullable(),
  responsibilities: z.array(z.string()).nullable(),
  status: z.string().nullable().optional(),
  saved: z.boolean().optional(),
  notes: z.string().nullable().optional(),
  user_state: UserJobStateSchema.optional(),
  added_at: z.string(),
  company: CompanyRefSchema,
})

type JobRaw = z.infer<typeof JobRawSchema>

// Raw schema for validation only — items are normalized to Job[] before returning.
const JobsPageRawSchema = z.object({
  items: z.array(JobRawSchema),
  next_cursor: z.string(),
})

// The exported type reflects the normalized shape consumers receive.
export interface JobsPage {
  items: Job[]
  next_cursor: string
}

const JobCountsSchema = z.object({
  new: z.number(),
  saved: z.number(),
  applied: z.number(),
  interviewing: z.number(),
  offer: z.number(),
  rejected: z.number(),
})

export type JobCounts = z.infer<typeof JobCountsSchema>

const BulkDismissResultSchema = z.object({ dismissed: z.number() })

const JobApplicationSchema = z.object({
  id: z.number(),
  job_result_id: z.string(),
  stage: z.string(),
  stage_at: z.string(),
  note: z.string().nullable(),
})

// ── Input types (not validated — sent to backend) ───────────────────────────

export interface JobFilters {
  status: JobStatus
  search?: string
  companies?: string[]
  remote?: boolean
  onSite?: boolean
  experienceRanges?: string[]
  daysAdded?: number
  sortBy?: string
  sortDir?: 'asc' | 'desc'
}

// ── Normalization ────────────────────────────────────────────────────────────

// The backend returns per-user state nested under user_state. Flatten the
// frequently-accessed fields (status, notes, saved) to the top level so
// all existing components keep working without change.
function normalizeJob(raw: JobRaw & { user_state?: z.infer<typeof UserJobStateSchema> }): Job {
  const us = raw.user_state
  return {
    ...raw,
    status: (us?.status ?? JOB_STATUS.NEW) as JobStatus,
    notes: us?.notes ?? null,
    saved: us?.saved_at != null,
    user_state: {
      status: (us?.status ?? JOB_STATUS.NEW) as JobStatus,
      notes: us?.notes ?? null,
      saved_at: us?.saved_at ?? null,
      applied_at: us?.applied_at ?? null,
      dismissed_at: us?.dismissed_at ?? null,
      dismissal_reason: us?.dismissal_reason ?? null,
      dismissal_note: us?.dismissal_note ?? null,
    },
  } as Job
}

// ── API functions ────────────────────────────────────────────────────────────

// GET /jobs — cursor-based keyset pagination (Phase 2)
// cursor="" fetches the first page; subsequent pages use next_cursor from prior response.
export async function fetchJobs(
  filters: JobFilters,
  cursor = '',
  limit = 20,
  signal?: AbortSignal
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

  const raw = await apiClient.get<unknown>(`/jobs?${p}`, signal ? { signal } : undefined)
  const page = JobsPageRawSchema.parse(raw)
  return { next_cursor: page.next_cursor, items: page.items.map(normalizeJob) }
}

// GET /jobs/:id
export async function fetchJob(id: string): Promise<Job> {
  const raw = await apiClient.get<unknown>(`/jobs/${id}`)
  return normalizeJob(JobRawSchema.parse(raw))
}

// PATCH /jobs/:id/save
export async function setJobSaved(id: string, saved: boolean): Promise<void> {
  await apiClient.patch<void>(`/jobs/${id}/save`, { saved })
}

// PATCH /jobs/:id/status
export async function setJobStatus(id: string, status: PatchableStatus): Promise<void> {
  await apiClient.patch<void>(`/jobs/${id}/status`, { status })
}

// PATCH /jobs/:id/notes
export async function setJobNotes(id: string, notes: string): Promise<void> {
  await apiClient.patch<void>(`/jobs/${id}/notes`, { notes })
}

// GET /jobs/counts
export async function fetchJobCounts(): Promise<JobCounts> {
  const raw = await apiClient.get<unknown>('/jobs/counts')
  return JobCountsSchema.parse(raw)
}

// GET /jobs/companies — all distinct company names in the jobs table
export async function fetchJobCompanies(): Promise<string[]> {
  const raw = await apiClient.get<unknown>('/jobs/companies')
  return z.array(z.string()).parse(raw)
}

// DELETE /jobs/:id  (dismiss)
export async function dismissJob(id: string, reason: string, note?: string): Promise<void> {
  await apiClient.delete<void>(`/jobs/${id}`, { reason, note: note ?? '' })
}

// POST /jobs/dismiss/bulk
export async function bulkDismissJobs(ids: string[]): Promise<{ dismissed: number }> {
  const raw = await apiClient.post<unknown>('/jobs/dismiss/bulk', { ids })
  return BulkDismissResultSchema.parse(raw)
}

// GET /jobs/:id/applications
export async function fetchApplications(jobId: string): Promise<JobApplication[]> {
  const raw = await apiClient.get<unknown>(`/jobs/${jobId}/applications`)
  return z.array(JobApplicationSchema).parse(raw) as JobApplication[]
}

// POST /jobs/:id/applications
export async function createApplication(
  jobId: string,
  stage: string,
  note?: string
): Promise<void> {
  await apiClient.post<void>(`/jobs/${jobId}/applications`, { stage, note: note ?? '' })
}
