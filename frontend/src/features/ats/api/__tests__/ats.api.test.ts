import { describe, it, expect, vi, beforeEach } from 'vitest'
import { apiClient } from '@/lib/core/api-client'
import { getAts, updateAts } from '../ats.api'

vi.mock('@/lib/core/api-client', () => ({
  apiClient: {
    get: vi.fn(),
    patch: vi.fn(),
  },
}))

const mockAts = [
  {
    name: 'greenhouse',
    display_name: 'Greenhouse',
    base_url: 'https://boards-api.greenhouse.io/v1/boards/',
    is_active: true,
    is_admin_creatable: true,
    scan_interval_min: 1440,
    inter_company_delay_s: 3,
  },
  {
    name: 'google',
    display_name: 'Google',
    base_url: '',
    is_active: true,
    is_admin_creatable: false,
    scan_interval_min: 720,
    inter_company_delay_s: 30,
  },
]

describe('getAts', () => {
  beforeEach(() => {
    vi.mocked(apiClient.get).mockReset()
  })

  it('fetches and parses ATS list', async () => {
    vi.mocked(apiClient.get).mockResolvedValue(mockAts)

    const result = await getAts()

    expect(apiClient.get).toHaveBeenCalledWith('/admin/ats')
    expect(result).toHaveLength(2)
    expect(result[0]).toEqual(mockAts[0])
    expect(result[1]).toEqual(mockAts[1])
  })

  it('returns empty array when response is null', async () => {
    vi.mocked(apiClient.get).mockResolvedValue(null)

    const result = await getAts()
    expect(result).toEqual([])
  })

  it('throws ZodError when response has invalid shape', async () => {
    vi.mocked(apiClient.get).mockResolvedValue([
      { name: 'bad' }, // missing required fields
    ])

    await expect(getAts()).rejects.toThrow()
  })

  it('validates all 7 required ATS fields', async () => {
    vi.mocked(apiClient.get).mockResolvedValue([
      {
        name: 'lever',
        display_name: 'Lever',
        base_url: 'https://jobs.lever.co/',
        is_active: true,
        is_admin_creatable: true,
        scan_interval_min: 1440,
        inter_company_delay_s: 5,
      },
    ])

    const result = await getAts()
    const ats = result[0]!
    expect(ats).toHaveProperty('name')
    expect(ats).toHaveProperty('display_name')
    expect(ats).toHaveProperty('base_url')
    expect(ats).toHaveProperty('is_active')
    expect(ats).toHaveProperty('is_admin_creatable')
    expect(ats).toHaveProperty('scan_interval_min')
    expect(ats).toHaveProperty('inter_company_delay_s')
  })
})

describe('updateAts', () => {
  beforeEach(() => {
    vi.mocked(apiClient.patch).mockReset()
  })

  it('calls PATCH /admin/ats/:name with data', async () => {
    const updated = { ...mockAts[0]! }
    vi.mocked(apiClient.patch).mockResolvedValue(updated)

    const result = await updateAts('greenhouse', { scan_interval_min: 720 })

    expect(apiClient.patch).toHaveBeenCalledWith('/admin/ats/greenhouse', {
      scan_interval_min: 720,
    })
    expect(result).toEqual(updated)
  })

  it('parses and returns the updated ATS', async () => {
    const updated = { ...mockAts[0]!, scan_interval_min: 720 }
    vi.mocked(apiClient.patch).mockResolvedValue(updated)

    const result = await updateAts('greenhouse', { scan_interval_min: 720 })
    expect(result.scan_interval_min).toBe(720)
  })

  it('throws ZodError when PATCH response has invalid shape', async () => {
    vi.mocked(apiClient.patch).mockResolvedValue({ name: 'bad' })

    await expect(updateAts('greenhouse', { scan_interval_min: 720 })).rejects.toThrow()
  })
})
