import { describe, it, expect, vi, beforeEach } from 'vitest'
import { renderHook, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { createElement } from 'react'
import { useAdapterConfigs } from '../hooks/useAdapterConfigs'
import type { AdapterConfig } from '@/api/adapterConfigs'

const mockConfigs: AdapterConfig[] = [
  {
    adapter_type: 'greenhouse',
    scan_interval_minutes: 1440,
    inter_company_delay_s: 3,
    last_scheduled_at: null,
    updated_at: '2026-04-24T00:00:00Z',
  },
  {
    adapter_type: 'google',
    scan_interval_minutes: 1440,
    inter_company_delay_s: 30,
    last_scheduled_at: '2026-04-24T12:00:00Z',
    updated_at: '2026-04-24T00:00:00Z',
  },
]

vi.mock('@/api/adapterConfigs', () => ({
  getAdapterConfigs: vi.fn(),
}))

import { getAdapterConfigs } from '@/api/adapterConfigs'
const mockGetAdapterConfigs = vi.mocked(getAdapterConfigs)

function makeWrapper() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  return ({ children }: { children: React.ReactNode }) =>
    createElement(QueryClientProvider, { client }, children)
}

describe('useAdapterConfigs', () => {
  beforeEach(() => {
    mockGetAdapterConfigs.mockReset()
  })

  it('fetches and returns adapter configs', async () => {
    mockGetAdapterConfigs.mockResolvedValue(mockConfigs)

    const { result } = renderHook(() => useAdapterConfigs(), {
      wrapper: makeWrapper(),
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))

    expect(result.current.data).toHaveLength(2)
    expect(result.current.data?.[0].adapter_type).toBe('greenhouse')
    expect(result.current.data?.[1].inter_company_delay_s).toBe(30)
  })

  it('returns correct shape with all required fields', async () => {
    mockGetAdapterConfigs.mockResolvedValue(mockConfigs)

    const { result } = renderHook(() => useAdapterConfigs(), {
      wrapper: makeWrapper(),
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))

    const config = result.current.data?.[0]
    expect(config).toHaveProperty('adapter_type')
    expect(config).toHaveProperty('scan_interval_minutes')
    expect(config).toHaveProperty('inter_company_delay_s')
    expect(config).toHaveProperty('last_scheduled_at')
    expect(config).toHaveProperty('updated_at')
  })

  it('handles fetch error', async () => {
    mockGetAdapterConfigs.mockRejectedValue(new Error('Network error'))

    const { result } = renderHook(() => useAdapterConfigs(), {
      wrapper: makeWrapper(),
    })

    await waitFor(() => expect(result.current.isError).toBe(true))
    expect(result.current.data).toBeUndefined()
  })
})
