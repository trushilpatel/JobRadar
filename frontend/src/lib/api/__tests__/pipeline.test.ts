import { describe, it, expect, vi, beforeEach } from 'vitest'

vi.mock('@/lib/core/api-client', () => ({
  apiClient: { get: vi.fn(), post: vi.fn() },
}))

import { apiClient } from '@/lib/core/api-client'
import { fetchPipelineStats, triggerRetry } from '@/lib/api/pipeline'

const getMock = vi.mocked(apiClient.get)
const postMock = vi.mocked(apiClient.post)

const emptyRetries = { '1m': 0, '10m': 0, '1h': 0, dlq: 0 }

beforeEach(() => {
  vi.clearAllMocks()
})

describe('fetchPipelineStats', () => {
  it('normalises a fully populated payload', async () => {
    getMock.mockResolvedValue({
      total_companies: 10,
      active_companies: 8,
      queued: 2,
      crawling: 1,
      awaiting_filter: 3,
      filter_retries: { '1m': 1, '10m': 2, '1h': 0, dlq: 0 },
      awaiting_enrichment: 4,
      enricher_retries: { '1m': 0, '10m': 0, '1h': 1, dlq: 2 },
      new_jobs_24h: 20,
      errors_24h: 1,
    })
    const stats = await fetchPipelineStats()
    expect(stats.total_companies).toBe(10)
    expect(stats.filter_retries).toEqual({ '1m': 1, '10m': 2, '1h': 0, dlq: 0 })
    expect(stats.enricher_retries.dlq).toBe(2)
    expect(getMock).toHaveBeenCalledWith('/admin/dashboard/pipeline')
  })

  it('returns all-zero defaults when the server returns null', async () => {
    getMock.mockResolvedValue(null)
    const stats = await fetchPipelineStats()
    expect(stats).toEqual({
      total_companies: 0,
      active_companies: 0,
      queued: 0,
      crawling: 0,
      awaiting_filter: 0,
      filter_retries: emptyRetries,
      awaiting_enrichment: 0,
      enricher_retries: emptyRetries,
      new_jobs_24h: 0,
      errors_24h: 0,
    })
  })

  it('fills empty retry tiers when retry objects are absent', async () => {
    getMock.mockResolvedValue({ total_companies: 5 })
    const stats = await fetchPipelineStats()
    expect(stats.total_companies).toBe(5)
    expect(stats.filter_retries).toEqual(emptyRetries)
    expect(stats.enricher_retries).toEqual(emptyRetries)
  })

  it('defaults missing retry-tier keys to 0', async () => {
    getMock.mockResolvedValue({ filter_retries: { '1m': 5 } })
    const stats = await fetchPipelineStats()
    expect(stats.filter_retries).toEqual({ '1m': 5, '10m': 0, '1h': 0, dlq: 0 })
  })

  it('propagates an apiClient error', async () => {
    getMock.mockRejectedValue(new Error('down'))
    await expect(fetchPipelineStats()).rejects.toThrow('down')
  })
})

describe('triggerRetry', () => {
  it('posts the worker + tier and parses the drained count', async () => {
    postMock.mockResolvedValue({ drained: 7 })
    await expect(triggerRetry('filter', '1m')).resolves.toEqual({ drained: 7 })
    expect(postMock).toHaveBeenCalledWith('/admin/dashboard/pipeline/retry', {
      worker: 'filter',
      tier: '1m',
    })
  })

  it('supports the enricher worker and dlq tier', async () => {
    postMock.mockResolvedValue({ drained: 0 })
    await triggerRetry('enricher', 'dlq')
    expect(postMock).toHaveBeenCalledWith('/admin/dashboard/pipeline/retry', {
      worker: 'enricher',
      tier: 'dlq',
    })
  })

  it('throws when the response is missing the drained field', async () => {
    postMock.mockResolvedValue({})
    await expect(triggerRetry('filter', '10m')).rejects.toThrow()
  })

  it('propagates an apiClient error', async () => {
    postMock.mockRejectedValue(new Error('boom'))
    await expect(triggerRetry('filter', '1h')).rejects.toThrow('boom')
  })
})
