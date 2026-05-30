import { describe, it, expect, vi, beforeEach } from 'vitest'
import { apiClient } from '@/lib/core/api-client'
import {
  fetchJobs,
  fetchJobCounts,
  fetchJobCompanies,
  setJobStatus,
  setJobSaved,
  setJobNotes,
  dismissJob,
  bulkDismissJobs,
} from '@/features/jobs/api/jobs.api'
import type { JobFilters, JobsPage, JobCounts } from '@/features/jobs/api/jobs.api'
import { JOB_STATUS } from '@/config/constants/jobStatus'

vi.mock('@/lib/core/api-client')

const baseFilters: JobFilters = { status: JOB_STATUS.NEW }

const rawJob = {
  id: 'j1',
  job_post_id: 'p1',
  company_id: 'c1',
  title: 'Software Engineer',
  location: 'New York',
  remote: false,
  url: 'https://example.com/job/1',
  department: null,
  employ_type: null,
  experience_years: null,
  tech_stack: null,
  salary_range: null,
  llm_summary: null,
  sponsorship: false,
  requires_citizen: false,
  security_clearance: false,
  role_brief: null,
  responsibilities: null,
  status: JOB_STATUS.NEW,
  saved: false,
  notes: null,
  user_state: {
    status: JOB_STATUS.NEW,
    notes: null,
    saved_at: null,
    applied_at: null,
    dismissed_at: null,
    dismissal_reason: null,
    dismissal_note: null,
  },
  added_at: '2024-01-01T00:00:00Z',
  company: {
    id: 'c1',
    name: 'Acme Corp',
    url: 'https://acme.com',
    adapter_type: 'greenhouse',
    active: true,
    created_at: '2024-01-01T00:00:00Z',
    last_scan_at: null,
    last_scan_status: null,
  },
}

describe('jobs API', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  // ── fetchJobCounts ──────────────────────────────────────────────────────────

  describe('fetchJobCounts', () => {
    it('calls /jobs/counts', async () => {
      const counts: JobCounts = {
        new: 5,
        saved: 3,
        applied: 1,
        interviewing: 0,
        offer: 0,
        rejected: 0,
      }
      vi.mocked(apiClient.get).mockResolvedValue(counts)
      const result = await fetchJobCounts()
      expect(apiClient.get).toHaveBeenCalledWith('/jobs/counts')
      expect(result.new).toBe(5)
      expect(result.saved).toBe(3)
    })

    it('returns all count fields', async () => {
      const counts: JobCounts = {
        new: 10,
        saved: 2,
        applied: 3,
        interviewing: 1,
        offer: 0,
        rejected: 4,
      }
      vi.mocked(apiClient.get).mockResolvedValue(counts)
      const result = await fetchJobCounts()
      expect(result).toEqual(counts)
    })
  })

  // ── fetchJobCompanies ───────────────────────────────────────────────────────

  describe('fetchJobCompanies', () => {
    it('calls /jobs/companies', async () => {
      vi.mocked(apiClient.get).mockResolvedValue(['Acme', 'Stripe'])
      const result = await fetchJobCompanies()
      expect(apiClient.get).toHaveBeenCalledWith('/jobs/companies')
      expect(result).toEqual(['Acme', 'Stripe'])
    })

    it('returns empty array when no companies', async () => {
      vi.mocked(apiClient.get).mockResolvedValue([])
      const result = await fetchJobCompanies()
      expect(result).toEqual([])
    })
  })

  // ── fetchJobs ───────────────────────────────────────────────────────────────

  describe('fetchJobs', () => {
    it('includes status param in URL', async () => {
      const page: JobsPage = { items: [], next_cursor: '' }
      vi.mocked(apiClient.get).mockResolvedValue(page)
      await fetchJobs({ status: JOB_STATUS.SAVED }, '', 20)
      const [url] = vi.mocked(apiClient.get).mock.calls[0]!
      expect(url).toContain('status=saved')
    })

    it('includes limit param in URL', async () => {
      const page: JobsPage = { items: [], next_cursor: '' }
      vi.mocked(apiClient.get).mockResolvedValue(page)
      await fetchJobs(baseFilters, '', 50)
      const [url] = vi.mocked(apiClient.get).mock.calls[0]!
      expect(url).toContain('limit=50')
    })

    it('includes cursor param when provided', async () => {
      const page: JobsPage = { items: [], next_cursor: '' }
      vi.mocked(apiClient.get).mockResolvedValue(page)
      await fetchJobs(baseFilters, 'abc123', 20)
      const [url] = vi.mocked(apiClient.get).mock.calls[0]!
      expect(url).toContain('cursor=abc123')
    })

    it('omits cursor param when empty', async () => {
      const page: JobsPage = { items: [], next_cursor: '' }
      vi.mocked(apiClient.get).mockResolvedValue(page)
      await fetchJobs(baseFilters, '', 20)
      const [url] = vi.mocked(apiClient.get).mock.calls[0]!
      expect(url).not.toContain('cursor=')
    })

    it('includes search param when provided', async () => {
      const page: JobsPage = { items: [], next_cursor: '' }
      vi.mocked(apiClient.get).mockResolvedValue(page)
      await fetchJobs({ ...baseFilters, search: 'react' }, '', 20)
      const [url] = vi.mocked(apiClient.get).mock.calls[0]!
      expect(url).toContain('search=react')
    })

    it('includes remote param when true', async () => {
      const page: JobsPage = { items: [], next_cursor: '' }
      vi.mocked(apiClient.get).mockResolvedValue(page)
      await fetchJobs({ ...baseFilters, remote: true }, '', 20)
      const [url] = vi.mocked(apiClient.get).mock.calls[0]!
      expect(url).toContain('remote=true')
    })

    it('includes on_site param when onSite is true', async () => {
      const page: JobsPage = { items: [], next_cursor: '' }
      vi.mocked(apiClient.get).mockResolvedValue(page)
      await fetchJobs({ ...baseFilters, onSite: true }, '', 20)
      const [url] = vi.mocked(apiClient.get).mock.calls[0]!
      expect(url).toContain('on_site=true')
    })

    it('returns { items, next_cursor } shape', async () => {
      const page: JobsPage = { items: [rawJob as never], next_cursor: 'cursor-abc' }
      vi.mocked(apiClient.get).mockResolvedValue(page)
      const result = await fetchJobs(baseFilters, '', 20)
      expect(result).toHaveProperty('items')
      expect(result).toHaveProperty('next_cursor')
      expect(result.next_cursor).toBe('cursor-abc')
      expect(result.items).toHaveLength(1)
    })

    it('parses the real embedded-company shape (no url/adapter_type/active/scan status)', async () => {
      // Regression: the backend `JobView` embeds the `companies` row, which only
      // carries id/name/slug/website/created_at — operational fields live in
      // company_scan_settings and are NOT in this object. A strict schema that
      // required url/adapter_type/active/last_scan_status threw on every job and
      // blanked the jobs page once the feed had data.
      const rawBackendCompany = {
        ...rawJob,
        company: {
          id: 'c1',
          name: 'Acme Corp',
          slug: 'acme',
          created_at: '2024-01-01T00:00:00Z',
          updated_at: '2024-01-02T00:00:00Z',
        },
      }
      const page: JobsPage = { items: [rawBackendCompany as never], next_cursor: '' }
      vi.mocked(apiClient.get).mockResolvedValue(page)
      const result = await fetchJobs(baseFilters, '', 20)
      expect(result.items).toHaveLength(1)
      expect(result.items[0]?.company?.name).toBe('Acme Corp')
    })

    it('normalizes job: fills status from user_state when present', async () => {
      const rawWithState = {
        ...rawJob,
        status: undefined,
        user_state: { ...rawJob.user_state, status: JOB_STATUS.SAVED },
      }
      const page: JobsPage = { items: [rawWithState as never], next_cursor: '' }
      vi.mocked(apiClient.get).mockResolvedValue(page)
      const result = await fetchJobs(baseFilters, '', 20)
      expect(result.items[0]?.status).toBe(JOB_STATUS.SAVED)
    })

    it('normalizes job: defaults status to "new" when user_state is absent', async () => {
      const rawNoState = { ...rawJob, user_state: undefined }
      const page: JobsPage = { items: [rawNoState as never], next_cursor: '' }
      vi.mocked(apiClient.get).mockResolvedValue(page)
      const result = await fetchJobs(baseFilters, '', 20)
      expect(result.items[0]?.status).toBe(JOB_STATUS.NEW)
    })

    it('includes days_added param when daysAdded is set', async () => {
      const page: JobsPage = { items: [], next_cursor: '' }
      vi.mocked(apiClient.get).mockResolvedValue(page)
      await fetchJobs({ ...baseFilters, daysAdded: 7 }, '', 20)
      const [url] = vi.mocked(apiClient.get).mock.calls[0]!
      expect(url).toContain('days_added=7')
    })

    it('includes sort_by and sort_dir params when set', async () => {
      const page: JobsPage = { items: [], next_cursor: '' }
      vi.mocked(apiClient.get).mockResolvedValue(page)
      await fetchJobs({ ...baseFilters, sortBy: 'added_at', sortDir: 'desc' }, '', 20)
      const [url] = vi.mocked(apiClient.get).mock.calls[0]!
      expect(url).toContain('sort_by=added_at')
      expect(url).toContain('sort_dir=desc')
    })

    it('appends multiple companies params', async () => {
      const page: JobsPage = { items: [], next_cursor: '' }
      vi.mocked(apiClient.get).mockResolvedValue(page)
      await fetchJobs({ ...baseFilters, companies: ['Acme', 'Stripe'] }, '', 20)
      const [url] = vi.mocked(apiClient.get).mock.calls[0]!
      expect(url).toContain('companies=Acme')
      expect(url).toContain('companies=Stripe')
    })
  })

  // ── setJobStatus ────────────────────────────────────────────────────────────

  describe('setJobStatus', () => {
    it('calls PATCH with applied status', async () => {
      vi.mocked(apiClient.patch).mockResolvedValue(undefined)
      await setJobStatus('job-2', JOB_STATUS.APPLIED)
      expect(apiClient.patch).toHaveBeenCalledWith('/jobs/job-2/status', { status: 'applied' })
    })

    it('calls PATCH with "new" to clear workflow status (saved_at preserved)', async () => {
      vi.mocked(apiClient.patch).mockResolvedValue(undefined)
      await setJobStatus('job-3', JOB_STATUS.NEW)
      expect(apiClient.patch).toHaveBeenCalledWith('/jobs/job-3/status', { status: 'new' })
    })
  })

  // ── setJobSaved ─────────────────────────────────────────────────────────────

  describe('setJobSaved', () => {
    it('calls PATCH /jobs/:id/save with saved=true', async () => {
      vi.mocked(apiClient.patch).mockResolvedValue(undefined)
      await setJobSaved('job-1', true)
      expect(apiClient.patch).toHaveBeenCalledWith('/jobs/job-1/save', { saved: true })
    })

    it('calls PATCH /jobs/:id/save with saved=false', async () => {
      vi.mocked(apiClient.patch).mockResolvedValue(undefined)
      await setJobSaved('job-1', false)
      expect(apiClient.patch).toHaveBeenCalledWith('/jobs/job-1/save', { saved: false })
    })
  })

  // ── setJobNotes ─────────────────────────────────────────────────────────────

  describe('setJobNotes', () => {
    it('calls PATCH /jobs/:id/notes with notes body', async () => {
      vi.mocked(apiClient.patch).mockResolvedValue(undefined)
      await setJobNotes('job-1', 'Great opportunity!')
      expect(apiClient.patch).toHaveBeenCalledWith('/jobs/job-1/notes', {
        notes: 'Great opportunity!',
      })
    })

    it('calls PATCH with empty notes string', async () => {
      vi.mocked(apiClient.patch).mockResolvedValue(undefined)
      await setJobNotes('job-1', '')
      expect(apiClient.patch).toHaveBeenCalledWith('/jobs/job-1/notes', { notes: '' })
    })
  })

  // ── dismissJob ──────────────────────────────────────────────────────────────

  describe('dismissJob', () => {
    it('calls DELETE /jobs/:id with reason', async () => {
      vi.mocked(apiClient.delete).mockResolvedValue(undefined)
      await dismissJob('job-1', 'user_dismissed')
      expect(apiClient.delete).toHaveBeenCalledWith('/jobs/job-1', {
        reason: 'user_dismissed',
        note: '',
      })
    })

    it('calls DELETE /jobs/:id with reason and note', async () => {
      vi.mocked(apiClient.delete).mockResolvedValue(undefined)
      await dismissJob('job-1', 'user_dismissed', 'Too far away')
      expect(apiClient.delete).toHaveBeenCalledWith('/jobs/job-1', {
        reason: 'user_dismissed',
        note: 'Too far away',
      })
    })

    it('defaults note to empty string when not provided', async () => {
      vi.mocked(apiClient.delete).mockResolvedValue(undefined)
      await dismissJob('job-2', 'user_not_relevant')
      const call = vi.mocked(apiClient.delete).mock.calls[0]!
      expect((call[1] as { note: string }).note).toBe('')
    })
  })

  // ── bulkDismissJobs ─────────────────────────────────────────────────────────

  describe('bulkDismissJobs', () => {
    it('calls POST /jobs/dismiss/bulk with ids array', async () => {
      vi.mocked(apiClient.post).mockResolvedValue({ dismissed: 3 })
      const result = await bulkDismissJobs(['j1', 'j2', 'j3'])
      expect(apiClient.post).toHaveBeenCalledWith('/jobs/dismiss/bulk', {
        ids: ['j1', 'j2', 'j3'],
      })
      expect(result.dismissed).toBe(3)
    })

    it('handles empty ids array', async () => {
      vi.mocked(apiClient.post).mockResolvedValue({ dismissed: 0 })
      const result = await bulkDismissJobs([])
      expect(apiClient.post).toHaveBeenCalledWith('/jobs/dismiss/bulk', { ids: [] })
      expect(result.dismissed).toBe(0)
    })
  })
})
