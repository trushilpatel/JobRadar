import { describe, it, expect, vi, beforeEach } from 'vitest'

vi.mock('@/lib/core/api-client', () => ({
  apiClient: { get: vi.fn(), patch: vi.fn() },
}))

import { apiClient } from '@/lib/core/api-client'
import {
  fetchProgress,
  fetchMarket,
  fetchHeatmap,
  fetchInterviews,
  fetchVelocity,
  fetchTechTrends,
  updateApplicationStage,
} from '@/features/dashboard/api/dashboard.api'

const getMock = vi.mocked(apiClient.get)
const patchMock = vi.mocked(apiClient.patch)

beforeEach(() => {
  vi.clearAllMocks()
})

describe('fetchProgress', () => {
  it('normalises a populated payload', async () => {
    getMock.mockResolvedValue({
      applied_count: 5,
      awaiting_reply: 3,
      interviewing_count: 2,
      interviewing_this_week: 1,
      offer_count: 4,
    })
    const res = await fetchProgress()
    expect(res).toEqual({
      applied_count: 5,
      awaiting_reply: 3,
      interviewing_count: 2,
      interviewing_this_week: 1,
      offer_count: 4,
    })
    expect(getMock).toHaveBeenCalledWith('/dashboard/progress')
  })

  it('returns all-zero defaults when the server returns null', async () => {
    getMock.mockResolvedValue(null)
    const res = await fetchProgress()
    expect(res).toEqual({
      applied_count: 0,
      awaiting_reply: 0,
      interviewing_count: 0,
      interviewing_this_week: 0,
      offer_count: 0,
    })
  })

  it('propagates an apiClient error', async () => {
    getMock.mockRejectedValue(new Error('down'))
    await expect(fetchProgress()).rejects.toThrow('down')
  })
})

describe('fetchMarket', () => {
  it('normalises a populated payload', async () => {
    getMock.mockResolvedValue({
      new_since_yesterday: 12,
      total_open: 340,
      added_this_week: 50,
      added_last_week: 40,
      weekly_delta: 10,
    })
    const res = await fetchMarket()
    expect(res.total_open).toBe(340)
    expect(res.weekly_delta).toBe(10)
    expect(getMock).toHaveBeenCalledWith('/dashboard/market')
  })

  it('returns all-zero defaults when the server returns null', async () => {
    getMock.mockResolvedValue(null)
    const res = await fetchMarket()
    expect(res).toEqual({
      new_since_yesterday: 0,
      total_open: 0,
      added_this_week: 0,
      added_last_week: 0,
      weekly_delta: 0,
    })
  })
})

describe('fetchHeatmap', () => {
  it('returns the parsed array', async () => {
    getMock.mockResolvedValue([{ week_start: '2024-01-01', job_count: 5 }])
    const res = await fetchHeatmap()
    expect(res).toEqual([{ week_start: '2024-01-01', job_count: 5 }])
    expect(getMock).toHaveBeenCalledWith('/dashboard/heatmap')
  })

  it('returns [] when the server returns null', async () => {
    getMock.mockResolvedValue(null)
    await expect(fetchHeatmap()).resolves.toEqual([])
  })
})

describe('fetchInterviews', () => {
  it('returns the parsed rows', async () => {
    const row = {
      application_id: 1,
      job_id: 'j1',
      company_name: 'Acme',
      job_title: 'SWE',
      stage: 'phone',
      display_stage: 'Phone',
      stage_at: '2024-01-01',
    }
    getMock.mockResolvedValue([row])
    await expect(fetchInterviews()).resolves.toEqual([row])
    expect(getMock).toHaveBeenCalledWith('/dashboard/interviews')
  })

  it('returns [] when the server returns null', async () => {
    getMock.mockResolvedValue(null)
    await expect(fetchInterviews()).resolves.toEqual([])
  })
})

describe('fetchVelocity', () => {
  const row = {
    company_id: 'c1',
    company_name: 'Acme',
    week_current: 8,
    week_1: 5,
    week_2: 3,
    week_3: 2,
    multiplier: 1.6,
    velocity_tag: 'rising' as const,
  }

  it('passes the default limit of 10', async () => {
    getMock.mockResolvedValue([row])
    await fetchVelocity()
    expect(getMock).toHaveBeenCalledWith('/dashboard/velocity?limit=10')
  })

  it('passes an explicit limit', async () => {
    getMock.mockResolvedValue([row])
    await fetchVelocity(25)
    expect(getMock).toHaveBeenCalledWith('/dashboard/velocity?limit=25')
  })

  it('returns [] when the server returns null', async () => {
    getMock.mockResolvedValue(null)
    await expect(fetchVelocity()).resolves.toEqual([])
  })
})

describe('fetchTechTrends', () => {
  it('normalises a populated payload', async () => {
    getMock.mockResolvedValue({
      total_jobs: 500,
      rising: [],
      new_entrants: [],
    })
    const res = await fetchTechTrends()
    expect(res).toEqual({ total_jobs: 500, rising: [], new_entrants: [] })
    expect(getMock).toHaveBeenCalledWith('/dashboard/tech-trends')
  })

  it('returns zero/empty defaults when the server returns null', async () => {
    getMock.mockResolvedValue(null)
    const res = await fetchTechTrends()
    expect(res).toEqual({ total_jobs: 0, rising: [], new_entrants: [] })
  })

  it('fills defaults when total_jobs is absent', async () => {
    getMock.mockResolvedValue({})
    const res = await fetchTechTrends()
    expect(res).toEqual({ total_jobs: 0, rising: [], new_entrants: [] })
  })
})

describe('updateApplicationStage', () => {
  it('patches the application stage', async () => {
    patchMock.mockResolvedValue(undefined)
    await updateApplicationStage(7, 'offer')
    expect(patchMock).toHaveBeenCalledWith('/applications/7/stage', { stage: 'offer' })
  })

  it('propagates an apiClient error', async () => {
    patchMock.mockRejectedValue(new Error('boom'))
    await expect(updateApplicationStage(7, 'offer')).rejects.toThrow('boom')
  })
})
