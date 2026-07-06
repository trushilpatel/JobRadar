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

// ── Fixtures ────────────────────────────────────────────────────────────────

const mockJob: Job = {
  id: 'j1',
  job_post_id: 'p1',
  company_id: 'c1',
  title: 'Software Engineer',
  location: 'New York',
  remote: false,
  url: 'https://example.com/job/1',
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
}

const mockPage: jobsApi.JobsPage = {
  items: [mockJob],
  next_cursor: '',
}

const mockCounts: jobsApi.JobCounts = {
  new: 1,
  saved: 0,
  applied: 0,
  interviewing: 0,
  offer: 0,
  rejected: 0,
}

// ── Wrapper ─────────────────────────────────────────────────────────────────

function makeWrapper(initialSearch = '') {
  const qc = new QueryClient({
    defaultOptions: { queries: { retry: 0, gcTime: 0 } },
  })
  const NuqsWrapper = withNuqsTestingAdapter({ searchParams: initialSearch, hasMemory: true })
  return ({ children }: { children: React.ReactNode }) =>
    createElement(
      MemoryRouter,
      {},
      createElement(QueryClientProvider, { client: qc }, createElement(NuqsWrapper, { children }))
    )
}

// ── Tests ────────────────────────────────────────────────────────────────────

describe('useJobs', () => {
  beforeEach(() => {
    vi.mocked(jobsApi.fetchJobs).mockResolvedValue(mockPage)
    vi.mocked(jobsApi.fetchJobCounts).mockResolvedValue(mockCounts)
    vi.mocked(jobsApi.fetchJobCompanies).mockResolvedValue([])
  })

  // ── Data loading ───────────────────────────────────────────────────────────

  it('returns jobs when query resolves', async () => {
    const { result } = renderHook(() => useJobs(), { wrapper: makeWrapper() })
    await waitFor(() => expect(result.current.isLoading).toBe(false))
    expect(result.current.jobs).toHaveLength(1)
    expect(result.current.jobs[0]?.id).toBe('j1')
    expect(result.current.jobs[0]?.title).toBe('Software Engineer')
  })

  it('starts in loading state', () => {
    const { result } = renderHook(() => useJobs(), { wrapper: makeWrapper() })
    expect(result.current.isLoading).toBe(true)
  })

  it('sets isLoading to false after data resolves', async () => {
    const { result } = renderHook(() => useJobs(), { wrapper: makeWrapper() })
    await waitFor(() => expect(result.current.isLoading).toBe(false))
  })

  it('returns empty jobs array when page has no items', async () => {
    vi.mocked(jobsApi.fetchJobs).mockResolvedValue({ items: [], next_cursor: '' })
    const { result } = renderHook(() => useJobs(), { wrapper: makeWrapper() })
    await waitFor(() => expect(result.current.isLoading).toBe(false))
    expect(result.current.jobs).toEqual([])
  })

  // ── Tab ────────────────────────────────────────────────────────────────────

  it('initial tab is "new"', () => {
    const { result } = renderHook(() => useJobs(), { wrapper: makeWrapper() })
    expect(result.current.tab).toBe(JOB_STATUS.NEW)
  })

  it('setTab changes the active tab', async () => {
    const { result } = renderHook(() => useJobs(), { wrapper: makeWrapper() })
    act(() => result.current.setTab(JOB_STATUS.SAVED))
    expect(result.current.tab).toBe(JOB_STATUS.SAVED)
  })

  // ── Tracking tab ─────────────────────────────────────────────────────────────

  it('defaults trackingStages to all 4 workflow stages', () => {
    const { result } = renderHook(() => useJobs(), { wrapper: makeWrapper('?tab=tracking') })
    expect(result.current.trackingStages).toEqual(['applied', 'interviewing', 'offer', 'rejected'])
  })

  it('sends statuses (not status) to fetchJobs when tab is tracking', async () => {
    renderHook(() => useJobs(), {
      wrapper: makeWrapper('?tab=tracking&trackingStages=applied,offer'),
    })
    await waitFor(() => {
      const lastCall = vi.mocked(jobsApi.fetchJobs).mock.calls.at(-1)
      expect(lastCall?.[0].statuses).toEqual(['applied', 'offer'])
    })
    const lastCall = vi.mocked(jobsApi.fetchJobs).mock.calls.at(-1)!
    expect(lastCall[0].status).toBeUndefined()
  })

  it('sends status (not statuses) on a non-tracking tab', async () => {
    renderHook(() => useJobs(), { wrapper: makeWrapper('?tab=saved') })
    await waitFor(() => {
      const lastCall = vi.mocked(jobsApi.fetchJobs).mock.calls.at(-1)
      expect(lastCall?.[0].status).toBe('saved')
    })
    const lastCall = vi.mocked(jobsApi.fetchJobs).mock.calls.at(-1)!
    expect(lastCall[0].statuses).toBeUndefined()
  })

  it('setTrackingStages updates trackingStages and re-queries with the new set', async () => {
    const { result } = renderHook(() => useJobs(), { wrapper: makeWrapper('?tab=tracking') })
    vi.mocked(jobsApi.fetchJobs).mockClear()
    act(() => result.current.setTrackingStages(['offer', 'rejected']))
    await waitFor(() => expect(result.current.trackingStages).toEqual(['offer', 'rejected']))
    await waitFor(() => expect(jobsApi.fetchJobs).toHaveBeenCalled())
    const lastCall = vi.mocked(jobsApi.fetchJobs).mock.calls.at(-1)!
    expect(lastCall[0].statuses).toEqual(['offer', 'rejected'])
  })

  it('counts.tracking sums the 4 workflow stage counts', async () => {
    vi.mocked(jobsApi.fetchJobCounts).mockResolvedValue({
      new: 1,
      saved: 2,
      applied: 3,
      interviewing: 4,
      offer: 5,
      rejected: 6,
    })
    const { result } = renderHook(() => useJobs(), { wrapper: makeWrapper() })
    await waitFor(() => expect(result.current.counts.tracking).toBe(18))
  })

  // ── Search ─────────────────────────────────────────────────────────────────

  it('initial searchQuery is empty string', () => {
    const { result } = renderHook(() => useJobs(), { wrapper: makeWrapper() })
    expect(result.current.searchQuery).toBe('')
  })

  it('setSearchQuery updates searchQuery', () => {
    const { result } = renderHook(() => useJobs(), { wrapper: makeWrapper() })
    act(() => result.current.setSearchQuery('react'))
    expect(result.current.searchQuery).toBe('react')
  })

  // ── Filters ────────────────────────────────────────────────────────────────

  it('initial selectedCompanies is empty', () => {
    const { result } = renderHook(() => useJobs(), { wrapper: makeWrapper() })
    expect(result.current.selectedCompanies).toEqual([])
  })

  it('initial locationFilter is "any"', () => {
    const { result } = renderHook(() => useJobs(), { wrapper: makeWrapper() })
    expect(result.current.locationFilter).toBe('any')
  })

  it('initial expFilter is empty', () => {
    const { result } = renderHook(() => useJobs(), { wrapper: makeWrapper() })
    expect(result.current.expFilter).toEqual([])
  })

  it('initial dateFilter is "all"', () => {
    const { result } = renderHook(() => useJobs(), { wrapper: makeWrapper() })
    expect(result.current.dateFilter).toBe('all')
  })

  it('initial visaFilter is "any"', () => {
    const { result } = renderHook(() => useJobs(), { wrapper: makeWrapper() })
    expect(result.current.visaFilter).toBe('any')
  })

  it('setSelectedCompanies updates selectedCompanies', () => {
    const { result } = renderHook(() => useJobs(), { wrapper: makeWrapper() })
    act(() => result.current.setSelectedCompanies(['Acme', 'Stripe']))
    expect(result.current.selectedCompanies).toEqual(['Acme', 'Stripe'])
  })

  it('setLocationFilter updates locationFilter', () => {
    const { result } = renderHook(() => useJobs(), { wrapper: makeWrapper() })
    act(() => result.current.setLocationFilter('remote'))
    expect(result.current.locationFilter).toBe('remote')
  })

  it('setExpFilter updates expFilter', () => {
    const { result } = renderHook(() => useJobs(), { wrapper: makeWrapper() })
    act(() => result.current.setExpFilter(['0-2', '2-5']))
    expect(result.current.expFilter).toEqual(['0-2', '2-5'])
  })

  it('setDateFilter updates dateFilter', () => {
    const { result } = renderHook(() => useJobs(), { wrapper: makeWrapper() })
    act(() => result.current.setDateFilter('7d'))
    expect(result.current.dateFilter).toBe('7d')
  })

  it('setDateFilter("24h") maps to daysAdded=1 on the resulting fetchJobs call', async () => {
    const { result } = renderHook(() => useJobs(), { wrapper: makeWrapper() })
    act(() => result.current.setDateFilter('24h'))
    await waitFor(() => expect(result.current.dateFilter).toBe('24h'))
    const lastCall = vi.mocked(jobsApi.fetchJobs).mock.calls.at(-1)!
    expect(lastCall[0].daysAdded).toBe(1)
  })

  it('setDateFilter("30d") maps to daysAdded=30 on the resulting fetchJobs call', async () => {
    const { result } = renderHook(() => useJobs(), { wrapper: makeWrapper() })
    act(() => result.current.setDateFilter('30d'))
    await waitFor(() => expect(result.current.dateFilter).toBe('30d'))
    const lastCall = vi.mocked(jobsApi.fetchJobs).mock.calls.at(-1)!
    expect(lastCall[0].daysAdded).toBe(30)
  })

  it('setLocationFilter("onsite") maps to onSite=true on the resulting fetchJobs call', async () => {
    const { result } = renderHook(() => useJobs(), { wrapper: makeWrapper() })
    act(() => result.current.setLocationFilter('onsite'))
    await waitFor(() => expect(result.current.locationFilter).toBe('onsite'))
    const lastCall = vi.mocked(jobsApi.fetchJobs).mock.calls.at(-1)!
    expect(lastCall[0].onSite).toBe(true)
  })

  it('setLocationFilter("hybrid") maps to hybrid=true on the resulting fetchJobs call', async () => {
    const { result } = renderHook(() => useJobs(), { wrapper: makeWrapper() })
    act(() => result.current.setLocationFilter('hybrid'))
    await waitFor(() => expect(result.current.locationFilter).toBe('hybrid'))
    const lastCall = vi.mocked(jobsApi.fetchJobs).mock.calls.at(-1)!
    expect(lastCall[0].hybrid).toBe(true)
  })

  it('setVisaFilter updates visaFilter', () => {
    const { result } = renderHook(() => useJobs(), { wrapper: makeWrapper() })
    act(() => result.current.setVisaFilter('true'))
    expect(result.current.visaFilter).toBe('true')
  })

  it('passes sponsorship=undefined to fetchJobs when visaFilter is "any"', async () => {
    const { result } = renderHook(() => useJobs(), { wrapper: makeWrapper() })
    await waitFor(() => expect(result.current.isLoading).toBe(false))
    const [filters] = vi.mocked(jobsApi.fetchJobs).mock.calls[0]!
    expect(filters.sponsorship).toBeUndefined()
  })

  it('passes sponsorship=true to fetchJobs when visaFilter is "true"', async () => {
    const { result } = renderHook(() => useJobs(), { wrapper: makeWrapper('?visa=true') })
    await waitFor(() => expect(result.current.isLoading).toBe(false))
    const lastCall = vi.mocked(jobsApi.fetchJobs).mock.calls.at(-1)!
    expect(lastCall[0].sponsorship).toBe(true)
  })

  it('passes sponsorship=false to fetchJobs when visaFilter is "false"', async () => {
    const { result } = renderHook(() => useJobs(), { wrapper: makeWrapper('?visa=false') })
    await waitFor(() => expect(result.current.isLoading).toBe(false))
    const lastCall = vi.mocked(jobsApi.fetchJobs).mock.calls.at(-1)!
    expect(lastCall[0].sponsorship).toBe(false)
  })

  // ── Reset filters ──────────────────────────────────────────────────────────

  it('resetFilters clears all filter state', () => {
    const { result } = renderHook(() => useJobs(), { wrapper: makeWrapper() })
    act(() => {
      result.current.setSearchQuery('react')
      result.current.setSelectedCompanies(['co-1'])
      result.current.setLocationFilter('remote')
      result.current.setExpFilter(['0-2'])
      result.current.setDateFilter('7d')
      result.current.setVisaFilter('true')
    })
    act(() => result.current.resetFilters())
    expect(result.current.searchQuery).toBe('')
    expect(result.current.selectedCompanies).toEqual([])
    expect(result.current.locationFilter).toBe('any')
    expect(result.current.expFilter).toEqual([])
    expect(result.current.dateFilter).toBe('all')
    expect(result.current.visaFilter).toBe('any')
  })

  // ── Counts ─────────────────────────────────────────────────────────────────

  it('counts are populated from fetchJobCounts response', async () => {
    const { result } = renderHook(() => useJobs(), { wrapper: makeWrapper() })
    await waitFor(() => expect(result.current.counts.new).toBe(1))
    expect(result.current.counts.saved).toBe(0)
    expect(result.current.counts.applied).toBe(0)
  })

  it('counts is empty object before fetchJobCounts resolves', () => {
    vi.mocked(jobsApi.fetchJobCounts).mockImplementation(
      () => new Promise(() => {}) // never resolves
    )
    const { result } = renderHook(() => useJobs(), { wrapper: makeWrapper() })
    expect(result.current.counts).toEqual({})
  })

  it('includes all tab count keys in counts', async () => {
    vi.mocked(jobsApi.fetchJobCounts).mockResolvedValue({
      new: 5,
      saved: 2,
      applied: 3,
      interviewing: 1,
      offer: 0,
      rejected: 4,
    })
    const { result } = renderHook(() => useJobs(), { wrapper: makeWrapper() })
    await waitFor(() => expect(result.current.counts.new).toBe(5))
    expect(result.current.counts.saved).toBe(2)
    expect(result.current.counts.applied).toBe(3)
    expect(result.current.counts.interviewing).toBe(1)
    expect(result.current.counts.offer).toBe(0)
    expect(result.current.counts.rejected).toBe(4)
  })

  // ── Companies (filter options) ─────────────────────────────────────────────

  it('companies defaults to empty array before fetch resolves', () => {
    vi.mocked(jobsApi.fetchJobCompanies).mockImplementation(
      () => new Promise(() => {}) // never resolves
    )
    const { result } = renderHook(() => useJobs(), { wrapper: makeWrapper() })
    expect(result.current.companies).toEqual([])
  })

  it('companies is populated after fetch resolves', async () => {
    vi.mocked(jobsApi.fetchJobCompanies).mockResolvedValue(['Acme', 'Stripe', 'Google'])
    const { result } = renderHook(() => useJobs(), { wrapper: makeWrapper() })
    await waitFor(() => expect(result.current.companies).toHaveLength(3))
    expect(result.current.companies).toEqual(['Acme', 'Stripe', 'Google'])
  })

  // ── Pagination ─────────────────────────────────────────────────────────────

  it('hasNextPage is false when next_cursor is empty', async () => {
    vi.mocked(jobsApi.fetchJobs).mockResolvedValue({ items: [mockJob], next_cursor: '' })
    const { result } = renderHook(() => useJobs(), { wrapper: makeWrapper() })
    await waitFor(() => expect(result.current.isLoading).toBe(false))
    expect(result.current.hasNextPage).toBe(false)
  })

  it('hasNextPage is true when next_cursor is non-empty', async () => {
    vi.mocked(jobsApi.fetchJobs).mockResolvedValue({ items: [mockJob], next_cursor: 'cursor-abc' })
    const { result } = renderHook(() => useJobs(), { wrapper: makeWrapper() })
    await waitFor(() => expect(result.current.isLoading).toBe(false))
    expect(result.current.hasNextPage).toBe(true)
  })

  // ── Sorting ────────────────────────────────────────────────────────────────

  it('initial sorting is empty', () => {
    const { result } = renderHook(() => useJobs(), { wrapper: makeWrapper() })
    expect(result.current.sorting).toEqual([])
  })

  it('handleSortingChange updates sorting', () => {
    const { result } = renderHook(() => useJobs(), { wrapper: makeWrapper() })
    act(() => result.current.handleSortingChange([{ id: 'added_at', desc: true }]))
    expect(result.current.sorting).toEqual([{ id: 'added_at', desc: true }])
  })

  // ── Limit ──────────────────────────────────────────────────────────────────

  it('initial limit is 20', () => {
    const { result } = renderHook(() => useJobs(), { wrapper: makeWrapper() })
    expect(result.current.limit).toBe(20)
  })

  // ── itemsOnPage ──────────────────────────────────────────────────────────

  it('itemsOnPage reflects number of jobs on current page', async () => {
    const { result } = renderHook(() => useJobs(), { wrapper: makeWrapper() })
    await waitFor(() => expect(result.current.isLoading).toBe(false))
    expect(result.current.itemsOnPage).toBe(1)
  })
})
