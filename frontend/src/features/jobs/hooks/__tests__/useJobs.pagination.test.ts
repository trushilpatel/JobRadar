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

// ── useJobs — infinite scroll (cursor history + accumulation) ───────────────

describe('useJobs — infinite scroll accumulation', () => {
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

  it('starts in loading state and populates jobs from the first page', async () => {
    vi.mocked(jobsApi.fetchJobs).mockResolvedValue({ items: [makeJob('a')], next_cursor: '' })
    const { result } = renderHook(() => useJobs(), { wrapper: makeWrapper() })
    expect(result.current.isLoading).toBe(true)
    await waitFor(() => expect(result.current.isLoading).toBe(false))
    expect(result.current.jobs).toHaveLength(1)
    expect(result.current.jobs[0]?.id).toBe('a')
  })

  // ── 2. loadMore appends the next page to the accumulated list ───────────────

  it('loadMore appends the next page onto the existing jobs instead of replacing them', async () => {
    vi.mocked(jobsApi.fetchJobs).mockImplementation(async (_filters, cursor) => {
      if (!cursor) return { items: [makeJob('a')], next_cursor: 'cur-2' }
      return { items: [makeJob('b')], next_cursor: '' }
    })
    const { result } = renderHook(() => useJobs(), { wrapper: makeWrapper() })
    await waitFor(() => expect(result.current.isLoading).toBe(false))
    expect(result.current.hasNextPage).toBe(true)
    expect(result.current.jobs.map((j) => j.id)).toEqual(['a'])

    await act(async () => {
      result.current.loadMore()
    })

    await waitFor(() => expect(result.current.jobs.map((j) => j.id)).toEqual(['a', 'b']))
    expect(result.current.hasNextPage).toBe(false)
  })

  // ── 3. loadMore at the last page is a no-op ─────────────────────────────────

  it('loadMore is a no-op when there is no next_cursor', async () => {
    vi.mocked(jobsApi.fetchJobs).mockResolvedValue({ items: [makeJob('a')], next_cursor: '' })
    const { result } = renderHook(() => useJobs(), { wrapper: makeWrapper() })
    await waitFor(() => expect(result.current.isLoading).toBe(false))
    expect(result.current.hasNextPage).toBe(false)

    act(() => result.current.loadMore())

    expect(result.current.jobs.map((j) => j.id)).toEqual(['a'])
    expect(jobsApi.fetchJobs).toHaveBeenCalledTimes(1)
  })

  // ── 4. loadMore while already fetching is a no-op (no duplicate requests) ───

  it('loadMore does not fire a second request while one is already in flight', async () => {
    let resolveSecond!: (v: jobsApi.JobsPage) => void
    vi.mocked(jobsApi.fetchJobs).mockImplementation(async (_filters, cursor) => {
      if (!cursor) return { items: [makeJob('a')], next_cursor: 'cur-2' }
      return new Promise((resolve) => {
        resolveSecond = resolve
      })
    })
    const { result } = renderHook(() => useJobs(), { wrapper: makeWrapper() })
    await waitFor(() => expect(result.current.isLoading).toBe(false))

    act(() => result.current.loadMore())
    await waitFor(() => expect(result.current.isFetchingMore).toBe(true))

    // A second loadMore while the first is still pending must not re-fire.
    act(() => result.current.loadMore())
    expect(jobsApi.fetchJobs).toHaveBeenCalledTimes(2)

    resolveSecond({ items: [makeJob('b')], next_cursor: '' })
    await waitFor(() => expect(result.current.jobs).toHaveLength(2))
  })

  // ── 5. Filter change resets the accumulated list ────────────────────────────

  it('filter change resets the accumulated jobs back to a fresh first page', async () => {
    vi.mocked(jobsApi.fetchJobs).mockImplementation(async (_filters, cursor) => {
      if (!cursor) return { items: [makeJob('a')], next_cursor: 'cur-2' }
      return { items: [makeJob('b')], next_cursor: '' }
    })
    const { result } = renderHook(() => useJobs(), { wrapper: makeWrapper() })
    await waitFor(() => expect(result.current.isLoading).toBe(false))

    await act(async () => {
      result.current.loadMore()
    })
    await waitFor(() => expect(result.current.jobs).toHaveLength(2))

    vi.mocked(jobsApi.fetchJobs).mockResolvedValue({ items: [makeJob('c')], next_cursor: '' })
    act(() => result.current.setLocationFilter('remote'))

    await waitFor(() => expect(result.current.jobs.map((j) => j.id)).toEqual(['c']))
  })

  // ── 5b. Visa filter change resets the accumulated list ──────────────────────

  it('visa filter change resets the accumulated jobs', async () => {
    vi.mocked(jobsApi.fetchJobs).mockImplementation(async (_filters, cursor) => {
      if (!cursor) return { items: [makeJob('a')], next_cursor: 'cur-2' }
      return { items: [makeJob('b')], next_cursor: '' }
    })
    const { result } = renderHook(() => useJobs(), { wrapper: makeWrapper() })
    await waitFor(() => expect(result.current.isLoading).toBe(false))

    await act(async () => {
      result.current.loadMore()
    })
    await waitFor(() => expect(result.current.jobs).toHaveLength(2))

    vi.mocked(jobsApi.fetchJobs).mockResolvedValue({ items: [makeJob('c')], next_cursor: '' })
    act(() => result.current.setVisaFilter('true'))

    await waitFor(() => expect(result.current.jobs.map((j) => j.id)).toEqual(['c']))
  })

  // ── 6. Tab change resets the accumulated list ───────────────────────────────

  it('tab change resets the accumulated jobs', async () => {
    vi.mocked(jobsApi.fetchJobs).mockImplementation(async (_filters, cursor) => {
      if (!cursor) return { items: [makeJob('a')], next_cursor: 'cur-2' }
      return { items: [makeJob('b')], next_cursor: '' }
    })
    const { result } = renderHook(() => useJobs(), { wrapper: makeWrapper() })
    await waitFor(() => expect(result.current.isLoading).toBe(false))

    await act(async () => {
      result.current.loadMore()
    })
    await waitFor(() => expect(result.current.jobs).toHaveLength(2))

    vi.mocked(jobsApi.fetchJobs).mockResolvedValue({ items: [makeJob('c')], next_cursor: '' })
    act(() => result.current.setTab(JOB_STATUS.SAVED))

    await waitFor(() => expect(result.current.jobs.map((j) => j.id)).toEqual(['c']))
    expect(result.current.tab).toBe(JOB_STATUS.SAVED)
  })

  // ── 7. sessionStorage restore still works for the underlying cursor history ──

  it('restores cursor history from sessionStorage when the fingerprint matches', async () => {
    vi.mocked(jobsApi.fetchJobs).mockImplementation(async (_filters, cursor) => {
      if (!cursor) return { items: [makeJob('a')], next_cursor: 'cur-2' }
      if (cursor === 'cur-2') return { items: [makeJob('b')], next_cursor: '' }
      return { items: [], next_cursor: '' }
    })

    // Phase 1: load more so cursorHistory gets persisted.
    const { result: r1, unmount } = renderHook(() => useJobs(), { wrapper: makeWrapper() })
    await waitFor(() => expect(r1.current.isLoading).toBe(false))
    await act(async () => {
      r1.current.loadMore()
    })
    await waitFor(() => expect(r1.current.jobs).toHaveLength(2))
    unmount()

    // Phase 2: a fresh hook instance with the same default filters restores the
    // persisted cursor history — loadMore uses the cached 'cur-2' cursor.
    const qc2 = new QueryClient({
      defaultOptions: { queries: { retry: 0, gcTime: Infinity, staleTime: Infinity } },
    })
    const { result: r2 } = renderHook(() => useJobs(), { wrapper: makeWrapper(qc2) })
    await waitFor(() => expect(r2.current.isLoading).toBe(false))

    await act(async () => {
      r2.current.loadMore()
    })
    await waitFor(() => expect(r2.current.jobs.map((j) => j.id)).toEqual(['a', 'b']))
  })

  // ── 8. hasNextPage semantics ─────────────────────────────────────────────────

  it('hasNextPage is true when the query returns a non-empty next_cursor', async () => {
    vi.mocked(jobsApi.fetchJobs).mockResolvedValue({
      items: [makeJob('a')],
      next_cursor: 'cursor-abc',
    })
    const { result } = renderHook(() => useJobs(), { wrapper: makeWrapper() })
    await waitFor(() => expect(result.current.isLoading).toBe(false))
    expect(result.current.hasNextPage).toBe(true)
  })

  it('hasNextPage is false when the query returns an empty next_cursor', async () => {
    vi.mocked(jobsApi.fetchJobs).mockResolvedValue({ items: [makeJob('a')], next_cursor: '' })
    const { result } = renderHook(() => useJobs(), { wrapper: makeWrapper() })
    await waitFor(() => expect(result.current.isLoading).toBe(false))
    expect(result.current.hasNextPage).toBe(false)
  })

  // ── 9. isFetchingMore ────────────────────────────────────────────────────────

  it('isFetchingMore is false during the initial load, true only while fetching a later page', async () => {
    vi.mocked(jobsApi.fetchJobs).mockImplementation(async (_filters, cursor) => {
      if (!cursor) return { items: [makeJob('a')], next_cursor: 'cur-2' }
      return { items: [makeJob('b')], next_cursor: '' }
    })
    const { result } = renderHook(() => useJobs(), { wrapper: makeWrapper() })
    expect(result.current.isFetchingMore).toBe(false)
    await waitFor(() => expect(result.current.isLoading).toBe(false))
    expect(result.current.isFetchingMore).toBe(false)

    act(() => result.current.loadMore())
    await waitFor(() => expect(result.current.isFetchingMore).toBe(true))
    await waitFor(() => expect(result.current.isFetchingMore).toBe(false))
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

  it('handleSortingChange resets the accumulated jobs', async () => {
    vi.mocked(jobsApi.fetchJobs).mockImplementation(async (_filters, cursor) => {
      if (!cursor) return { items: [makeJob('a')], next_cursor: 'cur-2' }
      return { items: [makeJob('b')], next_cursor: '' }
    })
    const { result } = renderHook(() => useJobs(), { wrapper: makeWrapper() })
    await waitFor(() => expect(result.current.isLoading).toBe(false))

    await act(async () => {
      result.current.loadMore()
    })
    await waitFor(() => expect(result.current.jobs).toHaveLength(2))

    vi.mocked(jobsApi.fetchJobs).mockResolvedValue({ items: [makeJob('c')], next_cursor: '' })
    act(() => result.current.handleSortingChange([{ id: 'added_at', desc: true }]))

    await waitFor(() => expect(result.current.jobs.map((j) => j.id)).toEqual(['c']))
  })
})
