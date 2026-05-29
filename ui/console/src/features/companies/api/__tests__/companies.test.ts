import { describe, it, expect, vi, beforeEach } from 'vitest'
import { apiClient } from '@/lib/core/api-client'
import {
  fetchScanStats,
  fetchCompanyAnalytics,
  triggerScan,
} from '@/features/companies/api/companies.api'
import type { ScanStats, CompanyAnalytics } from '@/types'

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
})
