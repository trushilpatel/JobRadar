import { describe, it, expect, vi, beforeEach } from 'vitest'
import { apiClient } from '@/lib/api-client'
import {
  getAdapterConfigs,
  updateAdapterConfig,
  updateScanIntervalOverride,
  getCompanyScanConfig,
} from '@/api/adapterConfigs'
import type { AdapterConfig, EffectiveScanConfig, Company } from '@/types'

vi.mock('@/lib/api-client')

const mockAdapterConfig: AdapterConfig = {
  adapter_type: 'greenhouse',
  scan_interval_minutes: 1440,
  inter_company_delay_s: 3,
  last_scheduled_at: null,
  updated_at: '2026-01-01T00:00:00Z',
}

const mockEffectiveScanConfig: EffectiveScanConfig = {
  adapter_type: 'greenhouse',
  effective_scan_interval_minutes: 720,
  inter_company_delay_s: 3,
  company_override: 720,
}

const mockCompany: Company = {
  id: 'stripe',
  name: 'Stripe',
  url: 'https://stripe.com/jobs',
  adapter_type: 'greenhouse',
  active: true,
  created_at: '2024-01-01T00:00:00Z',
  last_scan_at: null,
  last_scan_status: null,
}

describe('adapterConfigs API', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  // ── getAdapterConfigs ───────────────────────────────────────────────────────

  describe('getAdapterConfigs', () => {
    it('calls GET /admin/adapter-configs', async () => {
      vi.mocked(apiClient.get).mockResolvedValue([mockAdapterConfig])
      await getAdapterConfigs()
      expect(apiClient.get).toHaveBeenCalledWith('/admin/adapter-configs')
    })

    it('returns the list of adapter configs', async () => {
      vi.mocked(apiClient.get).mockResolvedValue([mockAdapterConfig])
      const result = await getAdapterConfigs()
      expect(result).toHaveLength(1)
      expect(result[0]?.adapter_type).toBe('greenhouse')
    })

    it('normalizes null response to empty array', async () => {
      vi.mocked(apiClient.get).mockResolvedValue(null)
      const result = await getAdapterConfigs()
      expect(result).toEqual([])
    })

    it('propagates rejection when apiClient.get rejects', async () => {
      vi.mocked(apiClient.get).mockRejectedValue(new Error('Network error'))
      await expect(getAdapterConfigs()).rejects.toThrow('Network error')
    })
  })

  // ── updateAdapterConfig ─────────────────────────────────────────────────────

  describe('updateAdapterConfig', () => {
    it('calls PATCH /admin/adapter-configs/:adapter_type with data', async () => {
      vi.mocked(apiClient.patch).mockResolvedValue(mockAdapterConfig)
      await updateAdapterConfig('greenhouse', {
        scan_interval_minutes: 720,
        inter_company_delay_s: 5,
      })
      expect(apiClient.patch).toHaveBeenCalledWith('/admin/adapter-configs/greenhouse', {
        scan_interval_minutes: 720,
        inter_company_delay_s: 5,
      })
    })

    it('URL-encodes adapter_type with special characters', async () => {
      vi.mocked(apiClient.patch).mockResolvedValue(mockAdapterConfig)
      await updateAdapterConfig('my adapter', {
        scan_interval_minutes: 1440,
        inter_company_delay_s: 0,
      })
      const [url] = vi.mocked(apiClient.patch).mock.calls[0]!
      expect(url).toContain('my%20adapter')
    })

    it('returns the updated adapter config', async () => {
      const updated = { ...mockAdapterConfig, scan_interval_minutes: 720 }
      vi.mocked(apiClient.patch).mockResolvedValue(updated)
      const result = await updateAdapterConfig('greenhouse', {
        scan_interval_minutes: 720,
        inter_company_delay_s: 3,
      })
      expect(result.scan_interval_minutes).toBe(720)
    })

    it('propagates rejection when apiClient.patch rejects', async () => {
      vi.mocked(apiClient.patch).mockRejectedValue(new Error('Server error'))
      await expect(
        updateAdapterConfig('greenhouse', { scan_interval_minutes: 720, inter_company_delay_s: 3 })
      ).rejects.toThrow('Server error')
    })
  })

  // ── updateScanIntervalOverride ──────────────────────────────────────────────

  describe('updateScanIntervalOverride', () => {
    it('calls PATCH /admin/companies/:id/scan-interval-override', async () => {
      vi.mocked(apiClient.patch).mockResolvedValue(mockCompany)
      await updateScanIntervalOverride('stripe', 720)
      expect(apiClient.patch).toHaveBeenCalledWith(
        '/admin/companies/stripe/scan-interval-override',
        { scan_interval_minutes_override: 720 }
      )
    })

    it('sends null to clear the override', async () => {
      vi.mocked(apiClient.patch).mockResolvedValue(mockCompany)
      await updateScanIntervalOverride('stripe', null)
      expect(apiClient.patch).toHaveBeenCalledWith(
        '/admin/companies/stripe/scan-interval-override',
        { scan_interval_minutes_override: null }
      )
    })

    it('URL-encodes the company id', async () => {
      vi.mocked(apiClient.patch).mockResolvedValue(mockCompany)
      await updateScanIntervalOverride('my company', 1440)
      const [url] = vi.mocked(apiClient.patch).mock.calls[0]!
      expect(url).toContain('my%20company')
    })

    it('propagates rejection when apiClient.patch rejects', async () => {
      vi.mocked(apiClient.patch).mockRejectedValue(new Error('Forbidden'))
      await expect(updateScanIntervalOverride('stripe', 720)).rejects.toThrow('Forbidden')
    })
  })

  // ── getCompanyScanConfig ────────────────────────────────────────────────────

  describe('getCompanyScanConfig', () => {
    it('calls GET /admin/companies/:id/scan-config', async () => {
      vi.mocked(apiClient.get).mockResolvedValue(mockEffectiveScanConfig)
      await getCompanyScanConfig('stripe')
      expect(apiClient.get).toHaveBeenCalledWith('/admin/companies/stripe/scan-config')
    })

    it('URL-encodes the company id', async () => {
      vi.mocked(apiClient.get).mockResolvedValue(mockEffectiveScanConfig)
      await getCompanyScanConfig('my company')
      const [url] = vi.mocked(apiClient.get).mock.calls[0]!
      expect(url).toContain('my%20company')
    })

    it('returns the effective scan config', async () => {
      vi.mocked(apiClient.get).mockResolvedValue(mockEffectiveScanConfig)
      const result = await getCompanyScanConfig('stripe')
      expect(result.adapter_type).toBe('greenhouse')
      expect(result.effective_scan_interval_minutes).toBe(720)
      expect(result.company_override).toBe(720)
    })

    it('returns config with null override when no company override is set', async () => {
      const noOverride: EffectiveScanConfig = { ...mockEffectiveScanConfig, company_override: null }
      vi.mocked(apiClient.get).mockResolvedValue(noOverride)
      const result = await getCompanyScanConfig('stripe')
      expect(result.company_override).toBeNull()
    })

    it('propagates rejection when apiClient.get rejects', async () => {
      vi.mocked(apiClient.get).mockRejectedValue(new Error('Not found'))
      await expect(getCompanyScanConfig('stripe')).rejects.toThrow('Not found')
    })
  })
})
