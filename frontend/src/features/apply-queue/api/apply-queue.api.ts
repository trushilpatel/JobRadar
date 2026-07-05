import { z } from 'zod'
import { apiClient } from '@/lib/core/api-client'
import { parseItemsResilient } from '@/lib/core/resilient-parse'
import type { ApplyQueueFilter } from '../constants'

// ── Schemas ──────────────────────────────────────────────────────────────────
//
// Field names stay snake_case end-to-end (matches every other API response in
// this codebase — see resumes.api.ts / jobs.api.ts) — no camelCasing at the
// boundary.

const ApplyQueueStatusSchema = z.union([
  z.literal('saved'),
  z.literal('applied'),
  z.literal('interviewing'),
])
export type ApplyQueueStatus = z.infer<typeof ApplyQueueStatusSchema>

const ApplyQueueItemSchema = z.object({
  job_id: z.string(),
  job_title: z.string(),
  company: z.string(),
  job_url: z.string(),
  status: ApplyQueueStatusSchema,
  resume_id: z.string().nullable(),
  resume_label: z.string().nullable(),
  // Best keyword-match resume for this job (issue #264) — null when no
  // recommendation is available (e.g. no resume has extracted keywords yet).
  recommended_resume_id: z.string().nullable(),
  recommended_resume_label: z.string().nullable(),
})
export type ApplyQueueItem = z.infer<typeof ApplyQueueItemSchema>

// Resilient envelope for runtime: never throws. `items` is parsed per-item by
// parseItemsResilient (a single drifted row is logged and dropped rather than
// blanking the whole page — see lib/core/resilient-parse.ts); a missing/odd
// next_cursor falls back to "" (no more pages).
const ApplyQueuePageEnvelopeSchema = z
  .object({
    items: z.array(z.unknown()).catch([]),
    next_cursor: z.string().catch(''),
  })
  .catch({ items: [], next_cursor: '' })

export interface ApplyQueuePage {
  items: ApplyQueueItem[]
  next_cursor: string
}

function parseApplyQueuePage(raw: unknown): ApplyQueuePage {
  const env = ApplyQueuePageEnvelopeSchema.parse(raw)
  const items = parseItemsResilient(ApplyQueueItemSchema, env.items, 'apply-queue')
  return { items, next_cursor: env.next_cursor }
}

// ── API functions ────────────────────────────────────────────────────────────

// GET /apply-queue — cursor-based keyset pagination.
// cursor="" fetches the first page; subsequent pages use next_cursor from the
// prior response.
export async function fetchApplyQueue(
  filter: ApplyQueueFilter,
  cursor = '',
  limit = 20,
  signal?: AbortSignal
): Promise<ApplyQueuePage> {
  const p = new URLSearchParams()
  p.set('filter', filter)
  p.set('limit', String(limit))
  if (cursor) p.set('cursor', cursor)

  const raw = await apiClient.get<unknown>(`/apply-queue?${p}`, signal ? { signal } : undefined)
  return parseApplyQueuePage(raw)
}

// PUT /apply-queue/:jobId/resume — attaches/clears the resume selection for a job.
export async function setApplyQueueResume(jobId: string, resumeId: string | null): Promise<void> {
  await apiClient.put<void>(`/apply-queue/${encodeURIComponent(jobId)}/resume`, {
    resume_id: resumeId,
  })
}
