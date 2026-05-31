import { describe, it, expect, vi, beforeEach } from 'vitest'
import { apiClient } from '@/lib/core/api-client'
import { setSchemaDriftLogger } from '@/lib/core/resilient-parse'
import { JobRawSchema, JobsPageRawSchema, fetchJobs } from '../jobs.api'
import { JOB_STATUS } from '@/config/constants/jobStatus'
import realPayload from './fixtures/jobsPage.real.json'

vi.mock('@/lib/core/api-client')

// ── Contract guard ───────────────────────────────────────────────────────────
// `jobsPage.real.json` is regenerated from the live backend via
// `go run ./cmd/apifixtures` (marshalling real rows through the actual response
// DTOs). Strict-parsing it here means: if the backend changes job serialization
// and the frontend schema isn't updated to match, CI fails — drift cannot be
// merged silently. This is the detection half of the contract guarantee.
describe('jobs API contract — real backend payload', () => {
  it('has representative coverage (>0 recorded items)', () => {
    expect(Array.isArray(realPayload.items)).toBe(true)
    expect(realPayload.items.length).toBeGreaterThan(0)
  })

  it('strict-parses the whole recorded page', () => {
    expect(() => JobsPageRawSchema.parse(realPayload)).not.toThrow()
  })

  it('strict-parses every recorded item individually', () => {
    realPayload.items.forEach((item, i) => {
      const r = JobRawSchema.safeParse(item)
      expect(
        r.success,
        `item ${i} failed contract: ${r.success ? '' : JSON.stringify(r.error.issues)}`
      ).toBe(true)
    })
  })
})

// ── Resilience ───────────────────────────────────────────────────────────────
// The runtime half: even if an unforeseen/drifted item slips through (version
// skew between deploys), it is dropped — never thrown — so the feed never blanks.
describe('fetchJobs — resilient runtime parsing', () => {
  const drift = vi.fn()
  beforeEach(() => {
    vi.clearAllMocks()
    drift.mockClear()
    setSchemaDriftLogger(drift)
  })

  it('keeps valid jobs and drops a drifted one without throwing', async () => {
    const good = realPayload.items[0]
    const broken = { ...realPayload.items[0], id: 123 } // id must be a string
    vi.mocked(apiClient.get).mockResolvedValue({ items: [good, broken], next_cursor: '' })

    const page = await fetchJobs({ status: JOB_STATUS.NEW }, '', 20)
    expect(page.items).toHaveLength(1)
    expect(drift).toHaveBeenCalledTimes(1)
  })

  it('returns an empty page (no throw) when the envelope is malformed', async () => {
    vi.mocked(apiClient.get).mockResolvedValue({ unexpected: 'shape' })
    const page = await fetchJobs({ status: JOB_STATUS.NEW }, '', 20)
    expect(page).toEqual({ items: [], next_cursor: '' })
  })
})
