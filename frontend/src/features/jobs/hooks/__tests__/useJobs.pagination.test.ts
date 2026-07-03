import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { renderHook, waitFor, act } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { createElement } from 'react'
import { MemoryRouter } from 'react-router-dom'
import { withNuqsTestingAdapter } from 'nuqs/adapters/testing'
import * as jobsApi from '@/features/jobs/api/jobs.api'
import { useJobs } from '../useJobs'
import { JOB_STATUS } from '@/config/constants/jobStatus'
import type { Job } from '@/types'

vi.mock('@/features/jobs/api/jobs.api')

function makeJob(id: string): Job {
  return {
    id,
    job_post_id: `p-${id}`,
    company_id: 'c1',
    title: `Title ${id}`,
    location: 'New York',
    remote: false,
    url: `https://example.com/job/${id}`,
    department: null,
    employ_type: null,
    experience_years: null,
    tech_stack: null,
    salary_range: null,
    llm_summary: null,
    sponsorship: false,
    requires_citizen: false,
    security_clearance: false,
    role_brief: null,
    responsibilities: null,
    status: JOB_STATUS.NEW,
    saved: false,
    notes: null,
    user_state: {
      status: JOB_STATUS.NEW,
      notes: null,
      saved_at: null,
      applied_at: null,
      dismissed_at: null,
      dismissal_reason: null,
      dismissal_note: null,
    },
    added_at: '2024-01-01T00:00:00Z',
    company: {
      id: 'c1',
      name: 'Acme Corp',
      url: 'https://acme.com',
      adapter_type: 'greenhouse',
      active: true,
      created_at: '2024-01-01T00:00:00Z',
      last_scan_at: null,
      last_scan_status: null,
    },
  } as Job
}

function makeWrapper(qc?: QueryClient, initialSearch = '') {
  const client = qc ?? new QueryClient({ defaultOptions: { queries: { retry: 0, gcTime: 0 } } })
  const NuqsWrapper = withNuqsTestingAdapter({ searchParams: initialSearch, hasMemory: true })
  return ({ children }: { children: React.ReactNode }) =>
    createElement(
      MemoryRouter,
      {},
      createElement(QueryClientProvider, { client }, createElement(NuqsWrapper, { children }))
    )
}

// ── useJobs — cursor history pagination ──────────────────────────────────────

describe('useJobs — cursor history pagination', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    sessionStorage.clear()
    vi.mocked(jobsApi.fetchJobCounts).mockResolvedValue({
      new: 0,
      saved: 0,
      applied: 0,
      interviewing: 0,
      offer: 0,
      rejected: 0,
    })
    vi.mocked(jobsApi.fetchJobCompanies).mockResolvedValue([])
  })

  afterEach(() => {
    sessionStorage.clear()
  })

  // ── 1. Initial load ─────────────────────────────────────────────────────────

  it('starts in loading state and populates jobs from first page', async () => {
    vi.mocked(jobsApi.fetchJobs).mockResolvedValue({ items: [makeJob('a')], next_cursor: '' })
    const { result } = renderHook(() => useJobs(), { wrapper: makeWrapper() })
    expect(result.current.isLoading).toBe(true)
    await waitFor(() => expect(result.current.isLoading).toBe(false))
    expect(result.current.jobs).toHaveLength(1)
    expect(result.current.jobs[0]?.id).toBe('a')
    expect(result.current.page).toBe(1)
  })

  // ── 2. goToNextPage appends cursor and advances pageIndex ───────────────────

  it('goToNextPage appends next_cursor to history and advances to page 2', async () => {
    vi.mocked(jobsApi.fetchJobs).mockImplementation(async (_filters, cursor) => {
      if (!cursor) return { items: [makeJob('a')], next_cursor: 'cur-2' }
      return { items: [makeJob('b')], next_cursor: '' }
    })
    const { result } = renderHook(() => useJobs(), { wrapper: makeWrapper() })
    await waitFor(() => expect(result.current.isLoading).toBe(false))
    expect(result.current.hasNextPage).toBe(true)

    await act(async () => {
      result.current.goToNextPage()
    })

    await waitFor(() => expect(result.current.jobs[0]?.id).toBe('b'))
    expect(result.current.page).toBe(2)
  })

  // ── 3. goToNextPage at last page is a no-op ─────────────────────────────────

  it('goToNextPage is a no-op when next_cursor is empty', async () => {
    vi.mocked(jobsApi.fetchJobs).mockResolvedValue({ items: [makeJob('a')], next_cursor: '' })
    const { result } = renderHook(() => useJobs(), { wrapper: makeWrapper() })
    await waitFor(() => expect(result.current.isLoading).toBe(false))
    expect(result.current.hasNextPage).toBe(false)

    act(() => result.current.goToNextPage())

    expect(result.current.page).toBe(1)
  })

  // ── 4. goToPrevPage decrements pageIndex ────────────────────────────────────

  it('goToPrevPage returns to page 1 after advancing to page 2', async () => {
    vi.mocked(jobsApi.fetchJobs).mockImplementation(async (_filters, cursor) => {
      if (!cursor) return { items: [makeJob('a')], next_cursor: 'cur-2' }
      return { items: [makeJob('b')], next_cursor: '' }
    })
    const { result } = renderHook(() => useJobs(), { wrapper: makeWrapper() })
    await waitFor(() => expect(result.current.isLoading).toBe(false))

    await act(async () => {
      result.current.goToNextPage()
    })
    await waitFor(() => expect(result.current.page).toBe(2))

    await act(async () => {
      result.current.goToPrevPage()
    })
    await waitFor(() => expect(result.current.page).toBe(1))
    // Going back uses the cached cursor — fetchJobs is called with '' again
    await waitFor(() => expect(result.current.jobs[0]?.id).toBe('a'))
  })

  // ── 5. goToPrevPage on page 1 is a no-op ───────────────────────────────────

  it('goToPrevPage does not go below page 1', async () => {
    vi.mocked(jobsApi.fetchJobs).mockResolvedValue({ items: [makeJob('a')], next_cursor: '' })
    const { result } = renderHook(() => useJobs(), { wrapper: makeWrapper() })

    act(() => result.current.goToPrevPage())

    expect(result.current.page).toBe(1)
  })

  // ── 6. Filter change resets cursorHistory ───────────────────────────────────

  it('filter change resets cursorHistory to [empty-string] and page to 1', async () => {
    vi.mocked(jobsApi.fetchJobs).mockImplementation(async (_filters, cursor) => {
      if (!cursor) return { items: [makeJob('a')], next_cursor: 'cur-2' }
      return { items: [makeJob('b')], next_cursor: '' }
    })
    const { result } = renderHook(() => useJobs(), { wrapper: makeWrapper() })
    await waitFor(() => expect(result.current.isLoading).toBe(false))

    await act(async () => {
      result.current.goToNextPage()
    })
    await waitFor(() => expect(result.current.page).toBe(2))

    // Change a filter — should reset back to page 1
    act(() => result.current.setLocationFilter('remote'))

    await waitFor(() => expect(result.current.page).toBe(1))
  })

  // ── 6b. Visa filter change resets cursorHistory and page ────────────────────

  it('visa filter change resets cursorHistory to [empty-string] and page to 1', async () => {
    vi.mocked(jobsApi.fetchJobs).mockImplementation(async (_filters, cursor) => {
      if (!cursor) return { items: [makeJob('a')], next_cursor: 'cur-2' }
      return { items: [makeJob('b')], next_cursor: '' }
    })
    const { result } = renderHook(() => useJobs(), { wrapper: makeWrapper() })
    await waitFor(() => expect(result.current.isLoading).toBe(false))

    await act(async () => {
      result.current.goToNextPage()
    })
    await waitFor(() => expect(result.current.page).toBe(2))

    // Change the visa filter — should reset back to page 1 (fingerprint change).
    act(() => result.current.setVisaFilter('true'))

    await waitFor(() => expect(result.current.page).toBe(1))
  })

  // ── 7. Tab change resets cursorHistory and page ─────────────────────────────

  it('tab change resets cursorHistory and returns to page 1', async () => {
    vi.mocked(jobsApi.fetchJobs).mockImplementation(async (_filters, cursor) => {
      if (!cursor) return { items: [makeJob('a')], next_cursor: 'cur-2' }
      return { items: [makeJob('b')], next_cursor: '' }
    })
    const { result } = renderHook(() => useJobs(), { wrapper: makeWrapper() })
    await waitFor(() => expect(result.current.isLoading).toBe(false))

    await act(async () => {
      result.current.goToNextPage()
    })
    await waitFor(() => expect(result.current.page).toBe(2))

    act(() => result.current.setTab(JOB_STATUS.SAVED))

    await waitFor(() => expect(result.current.page).toBe(1))
    expect(result.current.tab).toBe(JOB_STATUS.SAVED)
  })

  // ── 8. Limit change resets cursorHistory and page ──────────────────────────

  it('limit change resets cursorHistory and returns to page 1', async () => {
    vi.mocked(jobsApi.fetchJobs).mockImplementation(async (_filters, cursor) => {
      if (!cursor) return { items: [makeJob('a')], next_cursor: 'cur-2' }
      return { items: [makeJob('b')], next_cursor: '' }
    })
    const { result } = renderHook(() => useJobs(), { wrapper: makeWrapper() })
    await waitFor(() => expect(result.current.isLoading).toBe(false))

    await act(async () => {
      result.current.goToNextPage()
    })
    await waitFor(() => expect(result.current.page).toBe(2))

    act(() => result.current.setLimit(50))

    await waitFor(() => expect(result.current.page).toBe(1))
    expect(result.current.limit).toBe(50)
  })

  // ── 9. goToFirstPage resets to page 1 ──────────────────────────────────────

  it('goToFirstPage returns to page 1 from any page', async () => {
    vi.mocked(jobsApi.fetchJobs).mockImplementation(async (_filters, cursor) => {
      if (!cursor) return { items: [makeJob('a')], next_cursor: 'cur-2' }
      return { items: [makeJob('b')], next_cursor: '' }
    })
    const { result } = renderHook(() => useJobs(), { wrapper: makeWrapper() })
    await waitFor(() => expect(result.current.isLoading).toBe(false))

    await act(async () => {
      result.current.goToNextPage()
    })
    await waitFor(() => expect(result.current.page).toBe(2))

    act(() => result.current.goToFirstPage())

    await waitFor(() => expect(result.current.page).toBe(1))
    await waitFor(() => expect(result.current.jobs[0]?.id).toBe('a'))
  })

  // ── 10. itemsBeforePage = pageIndex * limit ─────────────────────────────────

  it('itemsBeforePage returns pageIndex * limit', async () => {
    vi.mocked(jobsApi.fetchJobs).mockImplementation(async (_filters, cursor) => {
      if (!cursor) return { items: [makeJob('a'), makeJob('a2')], next_cursor: 'cur-2' }
      return { items: [makeJob('b')], next_cursor: '' }
    })
    const { result } = renderHook(() => useJobs(), { wrapper: makeWrapper() })
    await waitFor(() => expect(result.current.isLoading).toBe(false))
    expect(result.current.itemsBeforePage).toBe(0) // pageIndex=0 * limit=20 = 0

    await act(async () => {
      result.current.goToNextPage()
    })
    await waitFor(() => expect(result.current.page).toBe(2))

    // pageIndex=1 * limit=20 = 20
    expect(result.current.itemsBeforePage).toBe(20)
  })

  // ── 11. Regression: no page-1 snap when clicking Next rapidly ───────────────
  //
  // With the old useInfiniteQuery approach, calling goToNextPage() could leave
  // a render cycle where pageIndex was N+1 but isFetchingNextPage was still
  // false, making isLoadingPage=false and jobs=[]. The Pagination H7 effect
  // would then fire onFirst() → snap to page 1.
  //
  // With the new approach there is NO intermediate state: setCursorHistory +
  // setPageIndex are both synchronous React state updates that batch in the
  // same render. By the time the next render occurs, both are consistent.

  it('no snap to page 1 — page advances synchronously to 2 on goToNextPage', async () => {
    vi.mocked(jobsApi.fetchJobs).mockImplementation(async (_filters, cursor) => {
      if (!cursor) return { items: [makeJob('a')], next_cursor: 'cur-2' }
      return { items: [makeJob('b')], next_cursor: '' }
    })
    const { result } = renderHook(() => useJobs(), { wrapper: makeWrapper() })
    await waitFor(() => expect(result.current.isLoading).toBe(false))

    // Call goToNextPage and immediately check page without waiting for the fetch.
    act(() => {
      result.current.goToNextPage()
    })

    // pageIndex advances to 2 synchronously — there is no intermediate state
    // where page=1 (the old snap-back race).
    expect(result.current.page).toBe(2)

    // The fetch eventually completes and jobs update.
    await waitFor(() => expect(result.current.jobs[0]?.id).toBe('b'))
  })

  // ── 12. hasNextPage is true when query returns non-empty next_cursor ─────────

  it('hasNextPage is true when query returns non-empty next_cursor', async () => {
    vi.mocked(jobsApi.fetchJobs).mockResolvedValue({
      items: [makeJob('a')],
      next_cursor: 'cursor-abc',
    })
    const { result } = renderHook(() => useJobs(), { wrapper: makeWrapper() })
    await waitFor(() => expect(result.current.isLoading).toBe(false))
    expect(result.current.hasNextPage).toBe(true)
  })

  // ── 13. hasNextPage is false when query returns empty next_cursor ────────────

  it('hasNextPage is false when query returns empty next_cursor', async () => {
    vi.mocked(jobsApi.fetchJobs).mockResolvedValue({ items: [makeJob('a')], next_cursor: '' })
    const { result } = renderHook(() => useJobs(), { wrapper: makeWrapper() })
    await waitFor(() => expect(result.current.isLoading).toBe(false))
    expect(result.current.hasNextPage).toBe(false)
  })

  // ── 14. hasPrevPage ─────────────────────────────────────────────────────────

  it('hasPrevPage is false on page 1 and true on page 2+', async () => {
    vi.mocked(jobsApi.fetchJobs).mockImplementation(async (_filters, cursor) => {
      if (!cursor) return { items: [makeJob('a')], next_cursor: 'cur-2' }
      return { items: [makeJob('b')], next_cursor: '' }
    })
    const { result } = renderHook(() => useJobs(), { wrapper: makeWrapper() })
    await waitFor(() => expect(result.current.isLoading).toBe(false))
    expect(result.current.hasPrevPage).toBe(false)

    await act(async () => {
      result.current.goToNextPage()
    })
    await waitFor(() => expect(result.current.page).toBe(2))

    expect(result.current.hasPrevPage).toBe(true)
  })

  // ── 15. sessionStorage restore ──────────────────────────────────────────────

  it('restores cursorHistory from sessionStorage when fingerprint matches', async () => {
    vi.mocked(jobsApi.fetchJobs).mockImplementation(async (_filters, cursor) => {
      if (!cursor) return { items: [makeJob('a')], next_cursor: 'cur-2' }
      if (cursor === 'cur-2') return { items: [makeJob('b')], next_cursor: '' }
      return { items: [], next_cursor: '' }
    })

    // Phase 1: Navigate to page 2 so cursorHistory gets persisted.
    const { result: r1, unmount } = renderHook(() => useJobs(), { wrapper: makeWrapper() })
    await waitFor(() => expect(r1.current.isLoading).toBe(false))
    await act(async () => {
      r1.current.goToNextPage()
    })
    await waitFor(() => expect(r1.current.page).toBe(2))
    unmount()

    // Phase 2: A new hook instance with the same default filters. The URL
    // page param starts at 0 (page 1), but sessionStorage has ['', 'cur-2'].
    // On page 1 we still get the '' cursor → page 1 data is shown.
    // goToNextPage() on page 2 now uses the cached cursor without a new fetch.
    const qc2 = new QueryClient({
      defaultOptions: { queries: { retry: 0, gcTime: Infinity, staleTime: Infinity } },
    })
    const { result: r2 } = renderHook(() => useJobs(), { wrapper: makeWrapper(qc2) })
    await waitFor(() => expect(r2.current.isLoading).toBe(false))

    // goToNextPage uses the stored cursor 'cur-2' from sessionStorage
    await act(async () => {
      r2.current.goToNextPage()
    })
    await waitFor(() => expect(r2.current.page).toBe(2))
    await waitFor(() => expect(r2.current.jobs[0]?.id).toBe('b'))
  })

  // ── Sorting branch ───────────────────────────────────────────────────────────

  it('handleSortingChange with empty array clears sort and resets dir', async () => {
    vi.mocked(jobsApi.fetchJobs).mockResolvedValue({ items: [makeJob('a')], next_cursor: '' })
    const { result } = renderHook(() => useJobs(), {
      wrapper: makeWrapper(undefined, '?sort=title&dir=desc'),
    })
    expect(result.current.sorting).toEqual([{ id: 'title', desc: true }])
    act(() => result.current.handleSortingChange([]))
    expect(result.current.sorting).toEqual([])
  })

  it('handleSortingChange resets to page 1', async () => {
    vi.mocked(jobsApi.fetchJobs).mockImplementation(async (_filters, cursor) => {
      if (!cursor) return { items: [makeJob('a')], next_cursor: 'cur-2' }
      return { items: [makeJob('b')], next_cursor: '' }
    })
    const { result } = renderHook(() => useJobs(), { wrapper: makeWrapper() })
    await waitFor(() => expect(result.current.isLoading).toBe(false))

    await act(async () => {
      result.current.goToNextPage()
    })
    await waitFor(() => expect(result.current.page).toBe(2))

    act(() => result.current.handleSortingChange([{ id: 'added_at', desc: true }]))

    await waitFor(() => expect(result.current.page).toBe(1))
  })
})
