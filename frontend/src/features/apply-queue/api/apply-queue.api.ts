import { z } from 'zod'
import { apiClient } from '@/lib/core/api-client'
import { parseItemsResilient } from '@/lib/core/resilient-parse'
import type { SalaryRange } from '@/types'
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

// Mirrors JobRawSchema's salary_range in features/jobs/api/jobs.api.ts — same
// backend shape, nullish min/max tolerated on parse. Normalized below (null ->
// undefined) so the exported ApplyQueueItem type lines up exactly with the
// shared `SalaryRange` interface used by formatSalary/JobDetailPanel.
const ApplyQueueSalaryRangeSchema = z
  .object({
    min: z.number().nullish(),
    max: z.number().nullish(),
    currency: z.string().optional(),
    period: z.string().optional(),
  })
  .nullable()

const ApplyQueueItemSchema = z.object({
  job_id: z.string(),
  job_title: z.string(),
  company: z.string(),
  job_url: z.string(),
  status: ApplyQueueStatusSchema,
  resume_id: z.string().nullable(),
  resume_label: z.string().nullable(),
  // Canonical job keywords (broadened past pure tech stack — see
  // docs/superpowers/specs/2026-07-05-resume-job-keyword-match-design.md).
  // The frontend computes any resume match against this client-side; the
  // backend performs no scoring.
  job_keywords: z.array(z.string()).catch([]),
  // Job info columns — same field names/shapes as the `Job` type's
  // equivalents (features/jobs/api/jobs.api.ts) so the Apply Queue table can
  // render the same location/salary/posted cells as the Jobs table.
  location: z.string().nullable(),
  remote: z.boolean(),
  salary_range: ApplyQueueSalaryRangeSchema,
  added_at: z.string(),
})

type ApplyQueueItemRaw = z.infer<typeof ApplyQueueItemSchema>

export interface ApplyQueueItem extends Omit<ApplyQueueItemRaw, 'salary_range'> {
  salary_range: SalaryRange | null
}

// Normalizes the raw parsed salary_range (which tolerates null min/max for
// backend leniency) into the exact SalaryRange shape (undefined, never null)
// consumed by formatSalary/JobDetailPanel — mirrors normalizeJob in
// features/jobs/api/jobs.api.ts.
function normalizeApplyQueueItem(raw: ApplyQueueItemRaw): ApplyQueueItem {
  return {
    ...raw,
    salary_range: raw.salary_range
      ? {
          min: raw.salary_range.min ?? undefined,
          max: raw.salary_range.max ?? undefined,
          currency: raw.salary_range.currency,
          period: raw.salary_range.period,
        }
      : null,
  }
}

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
  const items = parseItemsResilient(ApplyQueueItemSchema, env.items, 'apply-queue').map(
    normalizeApplyQueueItem
  )
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
