import { describe, it, expect, vi, beforeEach } from 'vitest'
import {
  getAdapterConfigs,
  updateAdapterConfig,
  updateScanIntervalOverride,
  getCompanyScanConfig,
} from '../adapterConfigs'

// Mock the api-client at the fetch boundary
vi.mock('@/lib/api-client', () => ({
  apiClient: {
    get: vi.fn(),
    patch: vi.fn(),
  },
}))

import { apiClient } from '@/lib/api-client'

describe('getAdapterConfigs', () => {
  beforeEach(() => vi.clearAllMocks())

  it('returns empty array when server returns null', async () => {
    vi.mocked(apiClient.get).mockResolvedValue(null)
    const result = await getAdapterConfigs()
    expect(result).toEqual([])
  })

  it('returns adapter configs on success', async () => {
    const configs = [
      {
        adapter_type: 'workday',
        scan_interval_minutes: 60,
        inter_company_delay_s: 2,
        last_scheduled_at: null,
        updated_at: '2026-01-01',
      },
    ]
    vi.mocked(apiClient.get).mockResolvedValue(configs)
    const result = await getAdapterConfigs()
    expect(result).toEqual(configs)
  })

  it('calls the correct endpoint', async () => {
    vi.mocked(apiClient.get).mockResolvedValue([])
    await getAdapterConfigs()
    expect(apiClient.get).toHaveBeenCalledWith(expect.stringContaining('adapter-configs'))
  })
})

const validAdapterConfig = {
  adapter_type: 'greenhouse',
  scan_interval_minutes: 60,
  inter_company_delay_s: 2,
  last_scheduled_at: null,
  updated_at: '2026-01-01T00:00:00Z',
}

describe('updateAdapterConfig', () => {
  beforeEach(() => vi.clearAllMocks())

  it('encodes adapter_type with special characters', async () => {
    vi.mocked(apiClient.patch).mockResolvedValue(validAdapterConfig)
    await updateAdapterConfig('type/with-slash', {
      scan_interval_minutes: 60,
      inter_company_delay_s: 2,
    })
    expect(apiClient.patch).toHaveBeenCalledWith(
      expect.stringContaining('type%2Fwith-slash'),
      expect.any(Object)
    )
  })

  it('sends the correct data payload', async () => {
    vi.mocked(apiClient.patch).mockResolvedValue(validAdapterConfig)
    const data = { scan_interval_minutes: 120, inter_company_delay_s: 5 }
    await updateAdapterConfig('greenhouse', data)
    expect(apiClient.patch).toHaveBeenCalledWith(expect.any(String), data)
  })
})

const validCompany = {
  id: 'company-1',
  name: 'Test Company',
  url: 'https://example.com',
  adapter_type: 'greenhouse',
  active: true,
  created_at: '2024-01-01T00:00:00Z',
  last_scan_at: null,
  last_scan_status: null,
}

describe('updateScanIntervalOverride', () => {
  beforeEach(() => vi.clearAllMocks())

  it('sends null for clearing override', async () => {
    vi.mocked(apiClient.patch).mockResolvedValue(validCompany)
    await updateScanIntervalOverride('company-1', null)
    expect(apiClient.patch).toHaveBeenCalledWith(expect.stringContaining('company-1'), {
      scan_interval_minutes_override: null,
    })
  })

  it('sends numeric value for setting override', async () => {
    vi.mocked(apiClient.patch).mockResolvedValue(validCompany)
    await updateScanIntervalOverride('company-1', 120)
    expect(apiClient.patch).toHaveBeenCalledWith(expect.any(String), {
      scan_interval_minutes_override: 120,
    })
  })

  it('URL-encodes the company ID', async () => {
    vi.mocked(apiClient.patch).mockResolvedValue(validCompany)
    await updateScanIntervalOverride('my company', 60)
    expect(apiClient.patch).toHaveBeenCalledWith(
      expect.stringContaining('my%20company'),
      expect.any(Object)
    )
  })
})

describe('getCompanyScanConfig', () => {
  beforeEach(() => vi.clearAllMocks())

  it('returns effective scan config', async () => {
    const config = {
      adapter_type: 'workday',
      effective_scan_interval_minutes: 60,
      inter_company_delay_s: 2,
      company_override: null,
    }
    vi.mocked(apiClient.get).mockResolvedValue(config)
    const result = await getCompanyScanConfig('company-1')
    expect(result).toEqual(config)
  })

  it('URL-encodes the company ID', async () => {
    vi.mocked(apiClient.get).mockResolvedValue({
      adapter_type: 'greenhouse',
      effective_scan_interval_minutes: 60,
      inter_company_delay_s: 2,
      company_override: null,
    })
    await getCompanyScanConfig('my company')
    expect(apiClient.get).toHaveBeenCalledWith(expect.stringContaining('my%20company'))
  })
})
