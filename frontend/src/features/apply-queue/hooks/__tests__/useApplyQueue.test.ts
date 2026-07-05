import { describe, it, expect, vi, beforeEach } from 'vitest'
import { renderHook, waitFor, act } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { createElement } from 'react'
import * as applyQueueApi from '@/features/apply-queue/api/apply-queue.api'
import { useApplyQueue } from '../useApplyQueue'
import type { ApplyQueuePage } from '@/features/apply-queue/api/apply-queue.api'

vi.mock('@/features/apply-queue/api/apply-queue.api')

function makeWrapper() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: 0, gcTime: 0 } } })
  return ({ children }: { children: React.ReactNode }) =>
    createElement(QueryClientProvider, { client: qc }, children)
}

const item = {
  job_id: 'job-1',
  job_title: 'Senior Engineer',
  company: 'Acme',
  job_url: 'https://acme.example/jobs/1',
  status: 'saved' as const,
  resume_id: null,
  resume_label: null,
  recommended_resume_id: null,
  recommended_resume_label: null,
  location: null,
  remote: false,
  salary_range: null,
  added_at: '2024-01-01T00:00:00Z',
}

const emptyPage: ApplyQueuePage = { items: [], next_cursor: '' }

describe('useApplyQueue', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    sessionStorage.clear()
  })

  it('returns items when the query resolves', async () => {
    vi.mocked(applyQueueApi.fetchApplyQueue).mockResolvedValue({
      items: [item],
      next_cursor: '',
    })
    const { result } = renderHook(() => useApplyQueue(), { wrapper: makeWrapper() })
    await waitFor(() => expect(result.current.isLoading).toBe(false))
    expect(result.current.items).toHaveLength(1)
    expect(result.current.items[0]?.job_id).toBe('job-1')
  })

  it('defaults to filter "all" and the first page', () => {
    vi.mocked(applyQueueApi.fetchApplyQueue).mockResolvedValue(emptyPage)
    const { result } = renderHook(() => useApplyQueue(), { wrapper: makeWrapper() })
    expect(result.current.filter).toBe('all')
    expect(result.current.page).toBe(1)
    expect(result.current.hasPrevPage).toBe(false)
  })

  it('goToNextPage advances the page when next_cursor is present', async () => {
    vi.mocked(applyQueueApi.fetchApplyQueue).mockResolvedValue({
      items: [item],
      next_cursor: 'cursor-2',
    })
    const { result } = renderHook(() => useApplyQueue(), { wrapper: makeWrapper() })
    await waitFor(() => expect(result.current.isLoading).toBe(false))

    act(() => result.current.goToNextPage())

    expect(result.current.page).toBe(2)
    expect(result.current.hasPrevPage).toBe(true)
  })

  it('goToNextPage is a no-op when there is no next_cursor', async () => {
    vi.mocked(applyQueueApi.fetchApplyQueue).mockResolvedValue(emptyPage)
    const { result } = renderHook(() => useApplyQueue(), { wrapper: makeWrapper() })
    await waitFor(() => expect(result.current.isLoading).toBe(false))

    act(() => result.current.goToNextPage())

    expect(result.current.page).toBe(1)
  })

  it('goToPrevPage navigates back to the first page', async () => {
    vi.mocked(applyQueueApi.fetchApplyQueue).mockResolvedValue({
      items: [item],
      next_cursor: 'cursor-2',
    })
    const { result } = renderHook(() => useApplyQueue(), { wrapper: makeWrapper() })
    await waitFor(() => expect(result.current.isLoading).toBe(false))

    act(() => result.current.goToNextPage())
    expect(result.current.hasPrevPage).toBe(true)

    act(() => result.current.goToPrevPage())
    expect(result.current.page).toBe(1)
    expect(result.current.hasPrevPage).toBe(false)
  })

  it('goToFirstPage resets to page 1', async () => {
    vi.mocked(applyQueueApi.fetchApplyQueue).mockResolvedValue({
      items: [item],
      next_cursor: 'cursor-2',
    })
    const { result } = renderHook(() => useApplyQueue(), { wrapper: makeWrapper() })
    await waitFor(() => expect(result.current.isLoading).toBe(false))

    act(() => result.current.goToNextPage())
    act(() => result.current.goToFirstPage())

    expect(result.current.page).toBe(1)
  })

  it('changing the filter resets pagination back to page 1', async () => {
    vi.mocked(applyQueueApi.fetchApplyQueue).mockResolvedValue({
      items: [item],
      next_cursor: 'cursor-2',
    })
    const { result } = renderHook(() => useApplyQueue(), { wrapper: makeWrapper() })
    await waitFor(() => expect(result.current.isLoading).toBe(false))

    act(() => result.current.goToNextPage())
    expect(result.current.page).toBe(2)

    act(() => result.current.setFilter('saved'))

    expect(result.current.filter).toBe('saved')
    expect(result.current.page).toBe(1)
    expect(result.current.hasPrevPage).toBe(false)
  })

  it('changing the limit resets pagination back to page 1', async () => {
    vi.mocked(applyQueueApi.fetchApplyQueue).mockResolvedValue({
      items: [item],
      next_cursor: 'cursor-2',
    })
    const { result } = renderHook(() => useApplyQueue(), { wrapper: makeWrapper() })
    await waitFor(() => expect(result.current.isLoading).toBe(false))

    act(() => result.current.goToNextPage())
    expect(result.current.page).toBe(2)

    act(() => result.current.setLimit(50))

    expect(result.current.limit).toBe(50)
    expect(result.current.page).toBe(1)
  })

  it('surfaces an error state when the fetch rejects', async () => {
    vi.mocked(applyQueueApi.fetchApplyQueue).mockRejectedValue(new Error('network error'))
    const { result } = renderHook(() => useApplyQueue(), { wrapper: makeWrapper() })
    await waitFor(() => expect(result.current.isError).toBe(true))
    expect(result.current.items).toEqual([])
  })

  it('persists cursor history to sessionStorage across next-page navigation', async () => {
    vi.mocked(applyQueueApi.fetchApplyQueue).mockResolvedValue({
      items: [item],
      next_cursor: 'cursor-2',
    })
    const { result } = renderHook(() => useApplyQueue(), { wrapper: makeWrapper() })
    await waitFor(() => expect(result.current.isLoading).toBe(false))

    act(() => result.current.goToNextPage())

    const keys = Object.keys(sessionStorage).filter((k) =>
      k.startsWith('jobradar:apply-queue-cursor-history:')
    )
    expect(keys.length).toBeGreaterThan(0)
  })

  it('refetches on remount even when the cached data is still within staleTime', async () => {
    vi.mocked(applyQueueApi.fetchApplyQueue).mockResolvedValue({
      items: [item],
      next_cursor: '',
    })
    // Reuse the SAME QueryClient (and default gcTime, so cache survives
    // unmount) across two renders to prove refetchOnMount:'always' overrides
    // the fresh 30s staleTime window — without it, this second mount would
    // reuse the cached data with zero additional fetchApplyQueue calls.
    const qc = new QueryClient({ defaultOptions: { queries: { retry: 0 } } })
    const wrapper = ({ children }: { children: React.ReactNode }) =>
      createElement(QueryClientProvider, { client: qc }, children)

    const { result, unmount } = renderHook(() => useApplyQueue(), { wrapper })
    await waitFor(() => expect(result.current.isLoading).toBe(false))
    expect(applyQueueApi.fetchApplyQueue).toHaveBeenCalledTimes(1)

    unmount()

    renderHook(() => useApplyQueue(), { wrapper })
    await waitFor(() => expect(applyQueueApi.fetchApplyQueue).toHaveBeenCalledTimes(2))
  })
})
