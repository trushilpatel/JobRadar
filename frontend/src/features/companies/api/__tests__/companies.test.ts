import { describe, it, expect, vi, beforeEach } from 'vitest'
import { apiClient } from '@/lib/core/api-client'
import {
  fetchScanStats,
  triggerScan,
  triggerScanAll,
  fetchCompanyAnalytics,
} from '../companies.api'

vi.mock('@/lib/core/api-client', () => ({
  apiClient: {
    get: vi.fn(),
    post: vi.fn(),
    patch: vi.fn(),
    delete: vi.fn(),
  },
}))

const mockedGet = vi.mocked(apiClient.get)
const mockedPost = vi.mocked(apiClient.post)

describe('companies.api', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  describe('fetchScanStats', () => {
    it('parses scan stats with a non-empty recent_runs array (no `id` field)', async () => {
      // Regression: the backend ScanRun DTO never sends `id`. The schema must
      // parse real runs that lack it — previously `id: z.number()` threw here,
      // breaking the detail-drawer Logs panel for every scanned company.
      mockedGet.mockResolvedValue({
        total_scans: 3,
        total_fetched: 120,
        total_new: 8,
        error_count: 1,
        avg_duration_ms: 1500,
        new_today: 2,
        new_this_month: 8,
        recent_runs: [
          {
            company_id: 'c1',
            scanned_at: '2025-01-02T00:00:00Z',
            jobs_fetched: 40,
            jobs_new: 3,
            error: null,
            duration_ms: 1400,
          },
          {
            company_id: 'c1',
            scanned_at: '2025-01-01T00:00:00Z',
            jobs_fetched: 0,
            jobs_new: 0,
            error: 'boom',
            duration_ms: null,
          },
        ],
      })

      const result = await fetchScanStats('c1')

      expect(mockedGet).toHaveBeenCalledWith('/companies/c1/scan-stats')
      expect(result.recent_runs).toHaveLength(2)
      expect(result.recent_runs[0]!.scanned_at).toBe('2025-01-02T00:00:00Z')
      expect(result.recent_runs[1]!.error).toBe('boom')
    })

    it('parses scan stats with empty recent_runs', async () => {
      mockedGet.mockResolvedValue({
        total_scans: 0,
        total_fetched: 0,
        total_new: 0,
        error_count: 0,
        avg_duration_ms: null,
        new_today: 0,
        new_this_month: 0,
        recent_runs: [],
      })

      const result = await fetchScanStats('c1')

      expect(result.recent_runs).toHaveLength(0)
      expect(result.avg_duration_ms).toBeNull()
    })

    it('url-encodes the company id', async () => {
      mockedGet.mockResolvedValue({
        total_scans: 0,
        total_fetched: 0,
        total_new: 0,
        error_count: 0,
        avg_duration_ms: null,
        new_today: 0,
        new_this_month: 0,
        recent_runs: [],
      })

      await fetchScanStats('a/b')

      expect(mockedGet).toHaveBeenCalledWith('/companies/a%2Fb/scan-stats')
    })

    it('propagates rejection when apiClient.get rejects', async () => {
      mockedGet.mockRejectedValue(new Error('Network error'))
      await expect(fetchScanStats('c1')).rejects.toThrow('Network error')
    })
  })

  describe('triggerScan', () => {
    it('posts to the scan endpoint', async () => {
      mockedPost.mockResolvedValue(undefined)
      await triggerScan('c1')
      expect(mockedPost).toHaveBeenCalledWith('/companies/c1/scan')
    })

    it('url-encodes the company id', async () => {
      mockedPost.mockResolvedValue(undefined)
      await triggerScan('a b')
      expect(mockedPost).toHaveBeenCalledWith('/companies/a%20b/scan')
    })
  })

  describe('triggerScanAll', () => {
    it('posts to scan-all and parses the queued count', async () => {
      // apiClient unwraps the `{data}` envelope, so the function receives the
      // inner `{queued}` payload from the backend's 202 response.
      mockedPost.mockResolvedValue({ queued: 7 })
      const result = await triggerScanAll()
      expect(mockedPost).toHaveBeenCalledWith('/companies/scan-all')
      expect(result.queued).toBe(7)
    })

    it('throws when the response shape is invalid', async () => {
      mockedPost.mockResolvedValue({ queued: 'lots' })
      await expect(triggerScanAll()).rejects.toThrow()
    })
  })

  describe('fetchCompanyAnalytics', () => {
    it('fetches and parses analytics', async () => {
      mockedGet.mockResolvedValue({
        monthly: [{ month: '2025-01', new: 5, removed: 1 }],
        velocity: { this_week: 3, last_week: 2 },
      })

      const result = await fetchCompanyAnalytics('c1')

      expect(mockedGet).toHaveBeenCalledWith('/companies/c1/analytics')
      expect(result.monthly).toHaveLength(1)
      expect(result.velocity.this_week).toBe(3)
    })

    it('propagates rejection when apiClient.get rejects', async () => {
      mockedGet.mockRejectedValue(new Error('Server error'))
      await expect(fetchCompanyAnalytics('c1')).rejects.toThrow('Server error')
    })
  })
})
