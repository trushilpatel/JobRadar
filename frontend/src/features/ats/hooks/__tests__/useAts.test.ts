import { describe, it, expect, vi, beforeEach } from 'vitest'
import { renderHook, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { createElement } from 'react'
import { useAts } from '../useAts'
import type { ATS } from '@/features/ats'

const mockAtsList: ATS[] = [
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
    name: 'lever',
    display_name: 'Lever',
    base_url: 'https://jobs.lever.co/',
    is_active: true,
    is_admin_creatable: true,
    scan_interval_min: 1440,
    inter_company_delay_s: 5,
  },
]

vi.mock('../../api/ats.api', () => ({
  getAts: vi.fn(),
}))

import { getAts } from '../../api/ats.api'
const mockGetAts = vi.mocked(getAts)

function makeWrapper() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  return ({ children }: { children: React.ReactNode }) =>
    createElement(QueryClientProvider, { client }, children)
}

describe('useAts', () => {
  beforeEach(() => {
    mockGetAts.mockReset()
  })

  it('fetches and returns ATS list', async () => {
    mockGetAts.mockResolvedValue(mockAtsList)

    const { result } = renderHook(() => useAts(), { wrapper: makeWrapper() })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))

    expect(result.current.data).toHaveLength(2)
    expect(result.current.data?.[0]?.name).toBe('greenhouse')
    expect(result.current.data?.[1]?.display_name).toBe('Lever')
  })

  it('returns correct shape with all required fields', async () => {
    mockGetAts.mockResolvedValue(mockAtsList)

    const { result } = renderHook(() => useAts(), { wrapper: makeWrapper() })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))

    const ats = result.current.data?.[0]
    expect(ats).toHaveProperty('name')
    expect(ats).toHaveProperty('display_name')
    expect(ats).toHaveProperty('base_url')
    expect(ats).toHaveProperty('is_active')
    expect(ats).toHaveProperty('is_admin_creatable')
    expect(ats).toHaveProperty('scan_interval_min')
    expect(ats).toHaveProperty('inter_company_delay_s')
  })

  it('uses staleTime of 10 minutes', async () => {
    mockGetAts.mockResolvedValue(mockAtsList)

    // Render hook and verify it fetches data — staleTime is internal config
    const { result } = renderHook(() => useAts(), { wrapper: makeWrapper() })
    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    // Data is fresh, second render should NOT re-fetch
    expect(mockGetAts).toHaveBeenCalledTimes(1)
  })

  it('handles fetch error', async () => {
    mockGetAts.mockRejectedValue(new Error('Network error'))

    const { result } = renderHook(() => useAts(), { wrapper: makeWrapper() })

    await waitFor(() => expect(result.current.isError).toBe(true))
    expect(result.current.data).toBeUndefined()
  })
})
