import { describe, it, expect, vi, beforeEach } from 'vitest'
import { renderHook, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { createElement } from 'react'
import * as companiesApi from '@/features/companies/api/companies.api'
import { useCompanyAnalytics } from '../useCompanyAnalytics'
import type { CompanyAnalytics } from '@/types'

vi.mock('@/features/companies/api/companies.api')

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

function makeWrapper() {
  const qc = new QueryClient({
    defaultOptions: { queries: { retry: 0, gcTime: 0 } },
  })
  return ({ children }: { children: React.ReactNode }) =>
    createElement(QueryClientProvider, { client: qc }, children)
}

describe('useCompanyAnalytics', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('returns analytics data when fetchCompanyAnalytics resolves', async () => {
    vi.mocked(companiesApi.fetchCompanyAnalytics).mockResolvedValue(mockAnalytics)

    const { result } = renderHook(() => useCompanyAnalytics('stripe'), {
      wrapper: makeWrapper(),
    })

    await waitFor(() => expect(result.current.isLoading).toBe(false))

    expect(result.current.data).toEqual(mockAnalytics)
    expect(result.current.isError).toBe(false)
  })

  it('sets isError when fetchCompanyAnalytics rejects', async () => {
    vi.mocked(companiesApi.fetchCompanyAnalytics).mockRejectedValue(new Error('Server error'))

    const { result } = renderHook(() => useCompanyAnalytics('stripe'), {
      wrapper: makeWrapper(),
    })

    await waitFor(() => expect(result.current.isLoading).toBe(false))

    expect(result.current.isError).toBe(true)
    expect(result.current.data).toBeUndefined()
  })

  it('does not fetch when companyId is empty string', () => {
    vi.mocked(companiesApi.fetchCompanyAnalytics).mockResolvedValue(mockAnalytics)

    const { result } = renderHook(() => useCompanyAnalytics(''), {
      wrapper: makeWrapper(),
    })

    // enabled: false when companyId is empty
    expect(companiesApi.fetchCompanyAnalytics).not.toHaveBeenCalled()
    expect(result.current.data).toBeUndefined()
  })
})
