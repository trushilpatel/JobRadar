import { describe, it, expect, vi, beforeEach } from 'vitest'
import { renderHook, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { createElement } from 'react'
import * as companiesApi from '@/api/companies'
import { useCompanyScanStats } from '../useCompanyScanStats'
import type { ScanStats } from '@/types'

vi.mock('@/api/companies')

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

function makeWrapper() {
  const qc = new QueryClient({
    defaultOptions: { queries: { retry: 0, gcTime: 0 } },
  })
  return ({ children }: { children: React.ReactNode }) =>
    createElement(QueryClientProvider, { client: qc }, children)
}

describe('useCompanyScanStats', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('returns scan stats when fetchScanStats resolves', async () => {
    vi.mocked(companiesApi.fetchScanStats).mockResolvedValue(mockScanStats)

    const { result } = renderHook(() => useCompanyScanStats('stripe'), {
      wrapper: makeWrapper(),
    })

    await waitFor(() => expect(result.current.isLoading).toBe(false))

    expect(result.current.data).toEqual(mockScanStats)
    expect(result.current.isError).toBe(false)
  })

  it('sets isError when fetchScanStats rejects', async () => {
    vi.mocked(companiesApi.fetchScanStats).mockRejectedValue(new Error('Not found'))

    const { result } = renderHook(() => useCompanyScanStats('stripe'), {
      wrapper: makeWrapper(),
    })

    await waitFor(() => expect(result.current.isLoading).toBe(false))

    expect(result.current.isError).toBe(true)
    expect(result.current.data).toBeUndefined()
  })

  it('does not fetch when companyId is empty string', () => {
    vi.mocked(companiesApi.fetchScanStats).mockResolvedValue(mockScanStats)

    const { result } = renderHook(() => useCompanyScanStats(''), {
      wrapper: makeWrapper(),
    })

    // enabled: false when companyId is empty — query stays in pending/loading state,
    // never calls fetchScanStats
    expect(companiesApi.fetchScanStats).not.toHaveBeenCalled()
    expect(result.current.data).toBeUndefined()
  })
})
