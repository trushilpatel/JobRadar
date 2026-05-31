import { describe, it, expect, vi, beforeEach } from 'vitest'
import { apiClient } from '@/lib/core/api-client'
import {
  fetchScanStats,
  fetchCompanyAnalytics,
  triggerScan,
  fetchCompanies,
  createCompany,
  updateCompany,
  deleteCompany,
  triggerScanAll,
  setAllCompaniesActive,
  fetchQueue,
  removeFromQueue,
} from '@/features/companies/api/companies.api'
import type { ScanStats, CompanyAnalytics, Company, QueueEntry } from '@/types'

vi.mock('@/lib/core/api-client')

const mockScanStats: ScanStats = {
  total_scans: 10,
  total_fetched: 50,
  total_new: 20,
  error_count: 2,
  avg_duration_ms: 1200,
  new_today: 3,
  new_this_month: 15,
  recent_runs: [],
}

const mockAnalytics: CompanyAnalytics = {
  monthly: [
    { month: '2024-01', new: 10, removed: 2 },
    { month: '2024-02', new: 15, removed: 1 },
  ],
  velocity: {
    this_week: 5,
    last_week: 3,
  },
}

describe('companies API', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  // ── fetchScanStats ─────────────────────────────────────────────────────────

  describe('fetchScanStats', () => {
    it('calls GET /companies/:id/scan-stats and returns data', async () => {
      vi.mocked(apiClient.get).mockResolvedValue(mockScanStats)

      const result = await fetchScanStats('stripe')

      expect(apiClient.get).toHaveBeenCalledWith('/companies/stripe/scan-stats')
      expect(result).toEqual(mockScanStats)
    })

    it('URL-encodes the company id', async () => {
      vi.mocked(apiClient.get).mockResolvedValue(mockScanStats)

      await fetchScanStats('my company')

      const [url] = vi.mocked(apiClient.get).mock.calls[0]!
      expect(url).toContain('my%20company')
    })

    it('propagates rejection when apiClient.get rejects', async () => {
      vi.mocked(apiClient.get).mockRejectedValue(new Error('Network error'))

      await expect(fetchScanStats('stripe')).rejects.toThrow('Network error')
    })
  })

  // ── fetchCompanyAnalytics ──────────────────────────────────────────────────

  describe('fetchCompanyAnalytics', () => {
    it('calls GET /companies/:id/analytics and returns data', async () => {
      vi.mocked(apiClient.get).mockResolvedValue(mockAnalytics)

      const result = await fetchCompanyAnalytics('stripe')

      expect(apiClient.get).toHaveBeenCalledWith('/companies/stripe/analytics')
      expect(result).toEqual(mockAnalytics)
    })

    it('URL-encodes the company id', async () => {
      vi.mocked(apiClient.get).mockResolvedValue(mockAnalytics)

      await fetchCompanyAnalytics('my company')

      const [url] = vi.mocked(apiClient.get).mock.calls[0]!
      expect(url).toContain('my%20company')
    })

    it('propagates rejection when apiClient.get rejects', async () => {
      vi.mocked(apiClient.get).mockRejectedValue(new Error('Server error'))

      await expect(fetchCompanyAnalytics('stripe')).rejects.toThrow('Server error')
    })
  })

  // ── triggerScan ────────────────────────────────────────────────────────────

  describe('triggerScan', () => {
    it('calls POST /companies/:id/scan', async () => {
      vi.mocked(apiClient.post).mockResolvedValue(undefined)

      await triggerScan('stripe')

      expect(apiClient.post).toHaveBeenCalledWith('/companies/stripe/scan')
    })

    it('URL-encodes the company id', async () => {
      vi.mocked(apiClient.post).mockResolvedValue(undefined)

      await triggerScan('my company')

      const [url] = vi.mocked(apiClient.post).mock.calls[0]!
      expect(url).toContain('my%20company')
    })

    it('propagates rejection when apiClient.post rejects', async () => {
      vi.mocked(apiClient.post).mockRejectedValue(new Error('Queue full'))

      await expect(triggerScan('stripe')).rejects.toThrow('Queue full')
    })
  })

  // ── fetchCompanies ─────────────────────────────────────────────────────────

  describe('fetchCompanies', () => {
    const page = {
      data: [] as Company[],
      total: 0,
      total_active: 0,
      page: 1,
      limit: 20,
    }

    it('sends only page + limit when no filters provided', async () => {
      vi.mocked(apiClient.get).mockResolvedValue(page)

      const result = await fetchCompanies()

      const [url] = vi.mocked(apiClient.get).mock.calls[0]!
      expect(url).toBe('/companies?page=1&limit=20')
      expect(result).toEqual(page)
    })

    it('serialises every filter field into the query string', async () => {
      vi.mocked(apiClient.get).mockResolvedValue(page)

      await fetchCompanies(
        {
          active: false,
          adapterTypes: ['greenhouse', 'lever'],
          search: 'stripe',
          lastScanStatus: 'error',
          sortBy: 'name',
          sortDir: 'desc',
        },
        2,
        50
      )

      const [url] = vi.mocked(apiClient.get).mock.calls[0]!
      expect(url).toContain('page=2')
      expect(url).toContain('limit=50')
      expect(url).toContain('active=false')
      expect(url).toContain('adapter_types=greenhouse')
      expect(url).toContain('adapter_types=lever')
      expect(url).toContain('search=stripe')
      expect(url).toContain('last_scan_status=error')
      expect(url).toContain('sort_by=name')
      expect(url).toContain('sort_dir=desc')
    })

    it('omits the active param when active is undefined', async () => {
      vi.mocked(apiClient.get).mockResolvedValue(page)

      await fetchCompanies({ adapterTypes: [] })

      const [url] = vi.mocked(apiClient.get).mock.calls[0]!
      expect(url).not.toContain('active=')
    })

    it('parses a populated company row through the schema', async () => {
      const populated = {
        ...page,
        data: [
          {
            id: 'stripe',
            name: 'Stripe',
            url: 'https://stripe.com',
            adapter_type: 'greenhouse',
            active: true,
            created_at: '2024-01-01T00:00:00Z',
            last_scan_at: null,
            last_scan_status: null,
            scan_history: [true, false],
          },
        ],
        total: 1,
        total_active: 1,
      }
      vi.mocked(apiClient.get).mockResolvedValue(populated)

      const result = await fetchCompanies()

      expect(result.data[0]!.name).toBe('Stripe')
      expect(result.total).toBe(1)
    })

    it('throws when the response shape is invalid', async () => {
      vi.mocked(apiClient.get).mockResolvedValue({ data: 'not-an-array' })

      await expect(fetchCompanies()).rejects.toThrow()
    })

    it('propagates rejection when apiClient.get rejects', async () => {
      vi.mocked(apiClient.get).mockRejectedValue(new Error('Network error'))

      await expect(fetchCompanies()).rejects.toThrow('Network error')
    })
  })

  // ── createCompany ──────────────────────────────────────────────────────────

  describe('createCompany', () => {
    const input = {
      id: 'stripe',
      name: 'Stripe',
      url: 'https://stripe.com',
      adapter_type: 'greenhouse',
      active: true,
    }

    it('calls POST /companies with the payload and returns the company', async () => {
      const created = { ...input, created_at: '2024-01-01T00:00:00Z' } as unknown as Company
      vi.mocked(apiClient.post).mockResolvedValue(created)

      const result = await createCompany(input)

      expect(apiClient.post).toHaveBeenCalledWith('/companies', input)
      expect(result).toEqual(created)
    })

    it('propagates rejection when apiClient.post rejects', async () => {
      vi.mocked(apiClient.post).mockRejectedValue(new Error('Conflict'))

      await expect(createCompany(input)).rejects.toThrow('Conflict')
    })
  })

  // ── updateCompany ──────────────────────────────────────────────────────────

  describe('updateCompany', () => {
    it('calls PATCH /companies/:id with the partial fields', async () => {
      vi.mocked(apiClient.patch).mockResolvedValue(undefined)

      await updateCompany('stripe', { name: 'Stripe Inc' })

      expect(apiClient.patch).toHaveBeenCalledWith('/companies/stripe', { name: 'Stripe Inc' })
    })

    it('URL-encodes the company id', async () => {
      vi.mocked(apiClient.patch).mockResolvedValue(undefined)

      await updateCompany('my company', { active: false })

      const [url] = vi.mocked(apiClient.patch).mock.calls[0]!
      expect(url).toContain('my%20company')
    })

    it('propagates rejection when apiClient.patch rejects', async () => {
      vi.mocked(apiClient.patch).mockRejectedValue(new Error('Bad request'))

      await expect(updateCompany('stripe', { name: 'x' })).rejects.toThrow('Bad request')
    })
  })

  // ── deleteCompany ──────────────────────────────────────────────────────────

  describe('deleteCompany', () => {
    it('calls DELETE /companies/:id', async () => {
      vi.mocked(apiClient.delete).mockResolvedValue(undefined)

      await deleteCompany('stripe')

      expect(apiClient.delete).toHaveBeenCalledWith('/companies/stripe')
    })

    it('URL-encodes the company id', async () => {
      vi.mocked(apiClient.delete).mockResolvedValue(undefined)

      await deleteCompany('my company')

      const [url] = vi.mocked(apiClient.delete).mock.calls[0]!
      expect(url).toContain('my%20company')
    })

    it('propagates rejection when apiClient.delete rejects', async () => {
      vi.mocked(apiClient.delete).mockRejectedValue(new Error('Not found'))

      await expect(deleteCompany('stripe')).rejects.toThrow('Not found')
    })
  })

  // ── triggerScanAll ─────────────────────────────────────────────────────────

  describe('triggerScanAll', () => {
    it('calls POST /companies/scan-all and parses the queued count', async () => {
      vi.mocked(apiClient.post).mockResolvedValue({ queued: 7 })

      const result = await triggerScanAll()

      expect(apiClient.post).toHaveBeenCalledWith('/companies/scan-all')
      expect(result).toEqual({ queued: 7 })
    })

    it('throws when the response shape is invalid', async () => {
      vi.mocked(apiClient.post).mockResolvedValue({ queued: 'lots' })

      await expect(triggerScanAll()).rejects.toThrow()
    })

    it('propagates rejection when apiClient.post rejects', async () => {
      vi.mocked(apiClient.post).mockRejectedValue(new Error('Server error'))

      await expect(triggerScanAll()).rejects.toThrow('Server error')
    })
  })

  // ── setAllCompaniesActive ──────────────────────────────────────────────────

  describe('setAllCompaniesActive', () => {
    it('calls PATCH /companies/active with the active flag', async () => {
      vi.mocked(apiClient.patch).mockResolvedValue(undefined)

      await setAllCompaniesActive(true)

      expect(apiClient.patch).toHaveBeenCalledWith('/companies/active', { active: true })
    })

    it('propagates rejection when apiClient.patch rejects', async () => {
      vi.mocked(apiClient.patch).mockRejectedValue(new Error('Forbidden'))

      await expect(setAllCompaniesActive(false)).rejects.toThrow('Forbidden')
    })
  })

  // ── fetchQueue ─────────────────────────────────────────────────────────────

  describe('fetchQueue', () => {
    const entry: QueueEntry = {
      id: 'q1',
      company_id: 'stripe',
      company_name: 'Stripe',
      adapter_type: 'greenhouse',
      status: 'queued',
      source: 'manual',
      priority: 1,
      queued_at: '2024-01-01T00:00:00Z',
      started_at: null,
      next_scan_at: null,
    }

    it('calls GET /companies/queue and parses entries', async () => {
      vi.mocked(apiClient.get).mockResolvedValue([entry])

      const result = await fetchQueue()

      expect(apiClient.get).toHaveBeenCalledWith('/companies/queue')
      expect(result).toEqual([entry])
    })

    it('parses an empty queue array', async () => {
      vi.mocked(apiClient.get).mockResolvedValue([])

      const result = await fetchQueue()

      expect(result).toEqual([])
    })

    it('throws when an entry has an invalid status', async () => {
      vi.mocked(apiClient.get).mockResolvedValue([{ ...entry, status: 'bogus' }])

      await expect(fetchQueue()).rejects.toThrow()
    })

    it('propagates rejection when apiClient.get rejects', async () => {
      vi.mocked(apiClient.get).mockRejectedValue(new Error('Network error'))

      await expect(fetchQueue()).rejects.toThrow('Network error')
    })
  })

  // ── removeFromQueue ────────────────────────────────────────────────────────

  describe('removeFromQueue', () => {
    it('calls DELETE /companies/queue/:entryId', async () => {
      vi.mocked(apiClient.delete).mockResolvedValue(undefined)

      await removeFromQueue('q1')

      expect(apiClient.delete).toHaveBeenCalledWith('/companies/queue/q1')
    })

    it('propagates rejection when apiClient.delete rejects', async () => {
      vi.mocked(apiClient.delete).mockRejectedValue(new Error('Not found'))

      await expect(removeFromQueue('q1')).rejects.toThrow('Not found')
    })
  })
})
