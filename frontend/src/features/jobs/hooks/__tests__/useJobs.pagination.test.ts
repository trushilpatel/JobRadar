import { describe, it, expect, vi, beforeEach } from 'vitest'
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

describe('useJobs — pagination + sorting branches', () => {
  beforeEach(() => {
    vi.clearAllMocks()
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

  it('handleSortingChange with empty array clears sort and resets dir', async () => {
    vi.mocked(jobsApi.fetchJobs).mockResolvedValue({ items: [makeJob('a')], next_cursor: '' })
    // Seed a sort via the URL so clearing it exercises the empty-next branch.
    const { result } = renderHook(() => useJobs(), {
      wrapper: makeWrapper(undefined, '?sort=title&dir=desc'),
    })
    expect(result.current.sorting).toEqual([{ id: 'title', desc: true }])
    act(() => result.current.handleSortingChange([]))
    expect(result.current.sorting).toEqual([])
  })

  it('goToNextPage fetches the next page at the boundary and advances', async () => {
    vi.mocked(jobsApi.fetchJobs).mockImplementation(async (_filters, cursor) => {
      if (!cursor) return { items: [makeJob('a')], next_cursor: 'cur-2' }
      return { items: [makeJob('b')], next_cursor: '' }
    })
    const { result } = renderHook(() => useJobs(), { wrapper: makeWrapper() })
    await waitFor(() => expect(result.current.isLoading).toBe(false))
    expect(result.current.page).toBe(1)
    expect(result.current.hasNextPage).toBe(true)

    await act(async () => {
      result.current.goToNextPage()
    })

    await waitFor(() => expect(result.current.jobs[0]?.id).toBe('b'))
    expect(result.current.page).toBe(2)
  })

  it('goToNextPage advances immediately when the next page is already fetched', async () => {
    vi.mocked(jobsApi.fetchJobs).mockImplementation(async (_filters, cursor) => {
      if (!cursor) return { items: [makeJob('a')], next_cursor: 'cur-2' }
      return { items: [makeJob('b')], next_cursor: '' }
    })

    // Use a shared QueryClient (gcTime/staleTime=Infinity so the cache survives
    // unmount and is still warm when phase 2 mounts).
    const sharedQC = new QueryClient({
      defaultOptions: { queries: { retry: 0, gcTime: Infinity, staleTime: Infinity } },
    })

    // Phase 1: populate the TanStack Query infinite cache with 2 pages.
    const { result: r1, unmount } = renderHook(() => useJobs(), {
      wrapper: makeWrapper(sharedQC),
    })
    await waitFor(() => expect(r1.current.isLoading).toBe(false))
    await act(async () => {
      r1.current.goToNextPage()
    }) // boundary fetch → page 2
    await waitFor(() => expect(r1.current.jobs[0]?.id).toBe('b'))
    unmount()

    // Phase 2: fresh nuqs instance (page=0, first-ever visit) + warm cache that
    // already has both pages. goToNextPage() hits the "advance immediately"
    // branch (fetchedPages.length=2, pageIndex=0 → 0 < 1) with no nuqs history
    // to trigger the setState-during-render edge case.
    const { result: r2 } = renderHook(() => useJobs(), {
      wrapper: makeWrapper(sharedQC),
    })
    await waitFor(() => expect(r2.current.isLoading).toBe(false))
    expect(r2.current.hasNextPage).toBe(true)

    await act(async () => {
      r2.current.goToNextPage()
    })
    await waitFor(() => expect(r2.current.page).toBe(2))
    expect(r2.current.jobs[0]?.id).toBe('b')
  })

  it('isLoadingPage is true while eagerly advanced beyond fetched pages', async () => {
    type PageResult = { items: ReturnType<typeof makeJob>[]; next_cursor: string }
    let resolveNextPage!: (v: PageResult) => void
    const nextPagePromise = new Promise<PageResult>((res) => {
      resolveNextPage = res
    })

    vi.mocked(jobsApi.fetchJobs).mockImplementation(async (_filters, cursor) => {
      if (!cursor) return { items: [makeJob('a')], next_cursor: 'cur-2' }
      return nextPagePromise
    })

    const { result } = renderHook(() => useJobs(), { wrapper: makeWrapper() })
    await waitFor(() => expect(result.current.isLoading).toBe(false))

    // Advance to page 2 — fetch is now in-flight.
    act(() => {
      result.current.goToNextPage()
    })

    // isLoadingPage must be true while the in-flight fetch hasn't resolved yet.
    await waitFor(() => expect(result.current.isLoadingPage).toBe(true))
    expect(result.current.jobs).toHaveLength(0)

    // Resolve the fetch — isLoadingPage should clear and jobs appear.
    act(() => {
      resolveNextPage({ items: [makeJob('b')], next_cursor: '' })
    })
    await waitFor(() => expect(result.current.isLoadingPage).toBe(false))
    await waitFor(() => expect(result.current.jobs[0]?.id).toBe('b'))
  })

  it('goToNextPage is a no-op when there is no next page', async () => {
    vi.mocked(jobsApi.fetchJobs).mockResolvedValue({ items: [makeJob('a')], next_cursor: '' })
    const { result } = renderHook(() => useJobs(), { wrapper: makeWrapper() })
    await waitFor(() => expect(result.current.isLoading).toBe(false))
    expect(result.current.hasNextPage).toBe(false)
    act(() => result.current.goToNextPage())
    expect(result.current.page).toBe(1)
  })

  it('goToPrevPage steps back from a later page', async () => {
    vi.mocked(jobsApi.fetchJobs).mockImplementation(async (_filters, cursor) => {
      if (!cursor) return { items: [makeJob('a')], next_cursor: 'cur-2' }
      return { items: [makeJob('b')], next_cursor: '' }
    })
    const { result } = renderHook(() => useJobs(), { wrapper: makeWrapper() })
    await waitFor(() => expect(result.current.isLoading).toBe(false))
    await act(async () => {
      result.current.goToNextPage()
    })
    await waitFor(() => expect(result.current.page).toBe(2), { timeout: 5000 })
    await act(async () => {
      result.current.goToPrevPage()
    })
    await waitFor(() => expect(result.current.page).toBe(1), { timeout: 5000 })
  })

  it('itemsBeforePage accumulates items from earlier pages', async () => {
    vi.mocked(jobsApi.fetchJobs).mockImplementation(async (_filters, cursor) => {
      if (!cursor) return { items: [makeJob('a'), makeJob('a2')], next_cursor: 'cur-2' }
      return { items: [makeJob('b')], next_cursor: '' }
    })
    const { result } = renderHook(() => useJobs(), { wrapper: makeWrapper() })
    await waitFor(() => expect(result.current.isLoading).toBe(false))
    await act(async () => {
      result.current.goToNextPage()
    })
    await waitFor(() => expect(result.current.page).toBe(2), { timeout: 5000 })
    expect(result.current.itemsBeforePage).toBe(2)
  })
})
