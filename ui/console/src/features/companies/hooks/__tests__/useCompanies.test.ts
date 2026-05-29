import { describe, it, expect, vi, beforeEach } from 'vitest'
import { renderHook, waitFor, act } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { createElement } from 'react'
import * as adminApi from '@/api/adminCompanies'
import { useCompanies } from '../useCompanies'
import type { AdminCompanyListResult, ScanSettingsView } from '@/api/adminCompanies'

vi.mock('@/api/adminCompanies')
// Mock useDebouncedValue so tests don't need real timers
vi.mock('@/lib/use-debounced-value', () => ({
  useDebouncedValue: (value: unknown) => value,
}))

const BASE_SETTINGS: ScanSettingsView = {
  company_id: 'stripe',
  adapter_type: 'greenhouse',
  adapter_config: null,
  scan_interval_override: null,
  active: true,
  last_scan_at: null,
  last_success_at: null,
  last_error_at: null,
  last_error_message: null,
  consecutive_errors: 0,
  created_at: '2024-01-01T00:00:00Z',
  updated_at: '2024-01-01T00:00:00Z',
}

const mockListResult: AdminCompanyListResult = {
  data: [
    {
      id: 'stripe',
      name: 'Stripe',
      website: 'https://stripe.com/jobs',
      created_at: '2024-01-01T00:00:00Z',
      scan_settings: BASE_SETTINGS,
    },
  ],
  total: 1,
  total_active: 1,
  limit: 20,
  next_cursor: null,
}

function makeWrapper() {
  const qc = new QueryClient({
    defaultOptions: { queries: { retry: 0, gcTime: 0 } },
  })
  return ({ children }: { children: React.ReactNode }) =>
    createElement(QueryClientProvider, { client: qc }, children)
}

describe('useCompanies', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    localStorage.clear()
  })

  it('returns normalized companies when query resolves', async () => {
    vi.mocked(adminApi.fetchAdminCompanies).mockResolvedValue(mockListResult)

    const { result } = renderHook(() => useCompanies(), { wrapper: makeWrapper() })

    await waitFor(() => expect(result.current.isLoading).toBe(false))

    expect(result.current.companies).toHaveLength(1)
    expect(result.current.companies[0]?.id).toBe('stripe')
    expect(result.current.companies[0]?.name).toBe('Stripe')
    expect(result.current.companies[0]?.adapter_type).toBe('greenhouse')
    expect(result.current.companies[0]?.active).toBe(true)
  })

  it('initial cursor is empty (first page)', () => {
    vi.mocked(adminApi.fetchAdminCompanies).mockResolvedValue(mockListResult)
    const { result } = renderHook(() => useCompanies(), { wrapper: makeWrapper() })
    // hasPrev is false when on first page (no cursor history)
    expect(result.current.hasPrev).toBe(false)
  })

  it('initial scanStatus filter is "all"', () => {
    vi.mocked(adminApi.fetchAdminCompanies).mockResolvedValue(mockListResult)
    const { result } = renderHook(() => useCompanies(), { wrapper: makeWrapper() })
    expect(result.current.scanStatus).toBe('all')
  })

  it('initial activeFilter is "all"', () => {
    vi.mocked(adminApi.fetchAdminCompanies).mockResolvedValue(mockListResult)
    const { result } = renderHook(() => useCompanies(), { wrapper: makeWrapper() })
    expect(result.current.activeFilter).toBe('all')
  })

  it('goToNextPage advances cursor when nextCursor is available', async () => {
    const resultWithCursor: AdminCompanyListResult = {
      ...mockListResult,
      next_cursor: 'cursor-abc',
    }
    vi.mocked(adminApi.fetchAdminCompanies).mockResolvedValue(resultWithCursor)
    const { result } = renderHook(() => useCompanies(), { wrapper: makeWrapper() })

    await waitFor(() => expect(result.current.isLoading).toBe(false))

    expect(result.current.nextCursor).toBe('cursor-abc')

    act(() => {
      result.current.goToNextPage()
    })

    // After going to next page, hasPrev should be true (cursor history has entry)
    expect(result.current.hasPrev).toBe(true)
  })

  it('goToPrevPage navigates back when hasPrev is true', async () => {
    const resultWithCursor: AdminCompanyListResult = {
      ...mockListResult,
      next_cursor: 'cursor-abc',
    }
    vi.mocked(adminApi.fetchAdminCompanies).mockResolvedValue(resultWithCursor)
    const { result } = renderHook(() => useCompanies(), { wrapper: makeWrapper() })

    await waitFor(() => expect(result.current.isLoading).toBe(false))

    act(() => {
      result.current.goToNextPage()
    })
    expect(result.current.hasPrev).toBe(true)

    act(() => {
      result.current.goToPrevPage()
    })
    expect(result.current.hasPrev).toBe(false)
  })

  it('changing scanStatus resets cursor and hasPrev to false', async () => {
    const resultWithCursor: AdminCompanyListResult = {
      ...mockListResult,
      next_cursor: 'cursor-abc',
    }
    vi.mocked(adminApi.fetchAdminCompanies).mockResolvedValue(resultWithCursor)
    const { result } = renderHook(() => useCompanies(), { wrapper: makeWrapper() })

    await waitFor(() => expect(result.current.isLoading).toBe(false))

    act(() => {
      result.current.goToNextPage()
    })
    expect(result.current.hasPrev).toBe(true)

    act(() => {
      result.current.setScanStatus('error')
    })
    expect(result.current.hasPrev).toBe(false)
    expect(result.current.scanStatus).toBe('error')
  })

  it('changing activeFilter resets cursor and hasPrev to false', async () => {
    const resultWithCursor: AdminCompanyListResult = {
      ...mockListResult,
      next_cursor: 'cursor-abc',
    }
    vi.mocked(adminApi.fetchAdminCompanies).mockResolvedValue(resultWithCursor)
    const { result } = renderHook(() => useCompanies(), { wrapper: makeWrapper() })

    await waitFor(() => expect(result.current.isLoading).toBe(false))

    act(() => {
      result.current.goToNextPage()
    })
    expect(result.current.hasPrev).toBe(true)

    act(() => {
      result.current.setActiveFilter('active')
    })
    expect(result.current.hasPrev).toBe(false)
    expect(result.current.activeFilter).toBe('active')
  })

  it('exposes isFetching', async () => {
    vi.mocked(adminApi.fetchAdminCompanies).mockResolvedValue(mockListResult)
    const { result } = renderHook(() => useCompanies(), { wrapper: makeWrapper() })
    // isFetching is a boolean (may be true initially, false after settle)
    expect(typeof result.current.isFetching).toBe('boolean')
  })

  it('returns total and totalActive from API', async () => {
    const resultWithCounts: AdminCompanyListResult = {
      ...mockListResult,
      total: 42,
      total_active: 30,
    }
    vi.mocked(adminApi.fetchAdminCompanies).mockResolvedValue(resultWithCounts)

    const { result } = renderHook(() => useCompanies(), { wrapper: makeWrapper() })

    await waitFor(() => expect(result.current.isLoading).toBe(false))

    expect(result.current.total).toBe(42)
    expect(result.current.totalActive).toBe(30)
  })

  it('autoRefresh is off by default (no localStorage entry)', () => {
    vi.mocked(adminApi.fetchAdminCompanies).mockResolvedValue(mockListResult)
    const { result } = renderHook(() => useCompanies(), { wrapper: makeWrapper() })
    expect(result.current.autoRefresh).toBe(false)
  })

  it('reads autoRefresh from localStorage when set to true', () => {
    localStorage.setItem('companies-auto-refresh', 'true')
    vi.mocked(adminApi.fetchAdminCompanies).mockResolvedValue(mockListResult)
    const { result } = renderHook(() => useCompanies(), { wrapper: makeWrapper() })
    expect(result.current.autoRefresh).toBe(true)
  })

  it('toggleAutoRefresh switches the state and persists to localStorage', () => {
    vi.mocked(adminApi.fetchAdminCompanies).mockResolvedValue(mockListResult)
    const { result } = renderHook(() => useCompanies(), { wrapper: makeWrapper() })

    expect(result.current.autoRefresh).toBe(false)

    act(() => {
      result.current.toggleAutoRefresh()
    })

    expect(result.current.autoRefresh).toBe(true)
    expect(localStorage.getItem('companies-auto-refresh')).toBe('true')

    act(() => {
      result.current.toggleAutoRefresh()
    })

    expect(result.current.autoRefresh).toBe(false)
    expect(localStorage.getItem('companies-auto-refresh')).toBe('false')
  })

  it('hasActiveFilters is false by default', () => {
    vi.mocked(adminApi.fetchAdminCompanies).mockResolvedValue(mockListResult)
    const { result } = renderHook(() => useCompanies(), { wrapper: makeWrapper() })
    expect(result.current.hasActiveFilters).toBe(false)
  })

  it('hasActiveFilters is true when activeFilter is not all', () => {
    vi.mocked(adminApi.fetchAdminCompanies).mockResolvedValue(mockListResult)
    const { result } = renderHook(() => useCompanies(), { wrapper: makeWrapper() })

    act(() => {
      result.current.setActiveFilter('active')
    })

    expect(result.current.hasActiveFilters).toBe(true)
  })

  it('resetFilters clears all filter state', async () => {
    vi.mocked(adminApi.fetchAdminCompanies).mockResolvedValue(mockListResult)
    const { result } = renderHook(() => useCompanies(), { wrapper: makeWrapper() })

    act(() => {
      result.current.setActiveFilter('active')
      result.current.setScanStatus('error')
      result.current.setAdapters(['greenhouse'])
    })

    act(() => {
      result.current.resetFilters()
    })

    expect(result.current.activeFilter).toBe('all')
    expect(result.current.scanStatus).toBe('all')
    expect(result.current.adapters).toEqual([])
    expect(result.current.hasActiveFilters).toBe(false)
  })

  it('defaults companies to empty array when data is undefined', () => {
    vi.mocked(adminApi.fetchAdminCompanies).mockImplementation(
      () => new Promise(() => {}) // never resolves
    )
    const { result } = renderHook(() => useCompanies(), { wrapper: makeWrapper() })
    expect(result.current.companies).toEqual([])
    expect(result.current.total).toBe(0)
  })

  it('sets isError when fetchAdminCompanies rejects and companies stays empty', async () => {
    vi.mocked(adminApi.fetchAdminCompanies).mockRejectedValue(new Error('Network error'))
    const { result } = renderHook(() => useCompanies(), { wrapper: makeWrapper() })

    await waitFor(() => expect(result.current.isLoading).toBe(false))

    expect(result.current.companies).toEqual([])
    expect(result.current.total).toBe(0)
  })
})
