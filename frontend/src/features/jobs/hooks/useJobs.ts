import { useEffect, useMemo, useRef, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { fetchJobs, fetchJobCounts, fetchJobCompanies, type JobFilters } from '../api/jobs.api'
import type { SortingState } from '@/components/ui/data-table'
import { useDebouncedValue } from '@/lib/hooks/use-debounced-value'
import { jobQueryKeys } from '../queryKeys'
import { type JobStatus, type TabKey, type TrackingStage } from '@/config/constants/jobStatus'
import type { Job } from '@/types'
import {
  useJobUrlState,
  type DateFilter,
  type LocationFilter,
  type SortDir,
  type VisaFilter,
} from './useJobUrlState'

export type { TabKey, TrackingStage } from '@/config/constants/jobStatus'
export type { DateFilter, LocationFilter, VisaFilter } from './useJobUrlState'

// ── Cursor history helpers ────────────────────────────────────────────────────
//
// cursorHistory[i] is the cursor string that fetches page (i+1).
// Index 0 always holds '' (first page, no cursor needed).
// Persisted to sessionStorage keyed by a filter fingerprint so same-session
// back/forward navigation restores page data without refetching.

const CURSOR_STORAGE_KEY_PREFIX = 'jobradar:cursor-history:'

function restoreHistory(key: string): string[] {
  try {
    const stored = sessionStorage.getItem(CURSOR_STORAGE_KEY_PREFIX + key)
    if (!stored) return ['']
    const parsed = JSON.parse(stored) as unknown
    if (Array.isArray(parsed) && parsed.every((x) => typeof x === 'string')) {
      return parsed as string[]
    }
  } catch {
    // ignore — sessionStorage may be unavailable or contain corrupt data
  }
  return ['']
}

function persistHistory(key: string, history: string[]): void {
  try {
    sessionStorage.setItem(CURSOR_STORAGE_KEY_PREFIX + key, JSON.stringify(history))
  } catch {
    // ignore — sessionStorage may be unavailable
  }
}

function clearHistory(key: string): void {
  try {
    sessionStorage.removeItem(CURSOR_STORAGE_KEY_PREFIX + key)
  } catch {
    // ignore
  }
}

export function useJobs() {
  const urlState = useJobUrlState()

  // Search: raw input value is local so we don't write the URL on every
  // keystroke. The debounced value is mirrored into the URL, which then
  // drives the API query.
  const [searchInput, setSearchInput] = useState(urlState.q)
  const debouncedSearch = useDebouncedValue(searchInput, 300)

  // Sync the input to the URL state when it lands at rest (debounced) and
  // sync FROM the URL when an external change happens (e.g. nav, reset).
  useEffect(() => {
    if (debouncedSearch !== urlState.q) {
      void urlState.setQ(debouncedSearch)
    }
    // We only want to react to local input changes (debounced); reading
    // urlState.q here would create a feedback loop.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [debouncedSearch])

  useEffect(() => {
    // External resets (resetFilters) propagate URL -> input. Only sync when
    // the URL changes to a value different from the current input rest state.
    if (urlState.q !== debouncedSearch && urlState.q !== searchInput) {
      setSearchInput(urlState.q)
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [urlState.q])

  // Sorting bridge between TanStack Table's SortingState and the URL.
  const sorting: SortingState = useMemo(
    () => (urlState.sort ? [{ id: urlState.sort, desc: urlState.dir === 'desc' }] : []),
    [urlState.sort, urlState.dir]
  )

  // Cursor-based pagination via useQuery + explicit cursor history.
  // pageIndex is the currently displayed page (0-indexed). Sourced from the
  // URL so refresh and share-links land on the same page.
  const pageIndex = urlState.page
  const setPageIndex = urlState.setPage

  const sortBy = urlState.sort || undefined
  const sortDir: 'asc' | 'desc' | undefined = urlState.sort ? urlState.dir : undefined

  // The Tracking tab is a frontend-only synthetic value (never sent to the
  // backend as `status=tracking`) — it translates into the combined
  // `statuses` param instead. See config/constants/jobStatus.ts.
  const isTracking = urlState.tab === 'tracking'

  const filters = useMemo<JobFilters>(
    () => ({
      status: isTracking ? undefined : (urlState.tab as JobStatus),
      statuses: isTracking ? (urlState.trackingStages as TrackingStage[]) : undefined,
      search: urlState.q || undefined,
      companies: urlState.companies.length > 0 ? urlState.companies : undefined,
      remote: urlState.loc === 'remote' || undefined,
      onSite: urlState.loc === 'onsite' || undefined,
      hybrid: urlState.loc === 'hybrid' || undefined,
      experienceRanges: urlState.exp.length > 0 ? urlState.exp : undefined,
      daysAdded:
        urlState.since === '24h'
          ? 1
          : urlState.since === '7d'
            ? 7
            : urlState.since === '30d'
              ? 30
              : undefined,
      sponsorship: urlState.visa === 'any' ? undefined : urlState.visa === 'true',
      sortBy,
      sortDir,
    }),
    [
      isTracking,
      urlState.tab,
      urlState.trackingStages,
      urlState.q,
      urlState.companies,
      urlState.loc,
      urlState.exp,
      urlState.since,
      urlState.visa,
      sortBy,
      sortDir,
    ]
  )

  // ── Cursor history (replaces useInfiniteQuery) ────────────────────────────
  //
  // Build a stable fingerprint from filters + limit for sessionStorage key.
  const filterFingerprint = useMemo(
    () =>
      JSON.stringify({
        tab: urlState.tab,
        statuses: (filters.statuses ?? []).slice().sort(),
        q: filters.search ?? '',
        companies: (filters.companies ?? []).slice().sort(),
        loc: filters.remote
          ? 'remote'
          : filters.onSite
            ? 'onsite'
            : filters.hybrid
              ? 'hybrid'
              : 'any',
        exp: (filters.experienceRanges ?? []).slice().sort(),
        since: filters.daysAdded ?? 0,
        visa: filters.sponsorship === undefined ? 'any' : String(filters.sponsorship),
        sort: filters.sortBy ?? '',
        dir: filters.sortDir ?? 'asc',
        limit: urlState.limit,
      }),
    [filters, urlState.tab, urlState.limit]
  )

  // Cursor history in React state, seeded from sessionStorage on mount.
  // If the URL has page > 0 on mount (e.g. browser back restored the param),
  // start fresh instead of restoring — loading mid-stream pages into an empty
  // accumulated list would show only those few items.
  const [cursorHistory, setCursorHistory] = useState<string[]>(() => {
    if (urlState.page > 0) {
      clearHistory(filterFingerprint)
      return ['']
    }
    return restoreHistory(filterFingerprint)
  })

  // Mirror the page reset to the URL once on mount so the address bar matches.
  const mountedRef = useRef(false)
  useEffect(() => {
    if (!mountedRef.current) {
      mountedRef.current = true
      if (urlState.page > 0) void urlState.setPage(0)
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  // Accumulated jobs across every fetched page — infinite scroll, hand-rolled
  // on top of useQuery + cursor history (NOT useInfiniteQuery, which is
  // forbidden for this project's cursor-nav pattern — see frontend/CLAUDE.md).
  // mergedCursorsRef guards against double-appending the same page: effects
  // re-run on unrelated re-renders even when `currentPage` hasn't changed.
  const [accumulatedJobs, setAccumulatedJobs] = useState<Job[]>([])
  const mergedCursorsRef = useRef<Set<string>>(new Set())

  // When filters change, reset cursor history AND the accumulated list — a
  // new filter set starts a fresh infinite-scroll session from the top.
  useEffect(() => {
    setCursorHistory(restoreHistory(filterFingerprint))
    setAccumulatedJobs([])
    mergedCursorsRef.current = new Set()
  }, [filterFingerprint])

  const currentCursor = cursorHistory[pageIndex] ?? ''

  const query = useQuery({
    queryKey: jobQueryKeys.list(filters, urlState.limit, currentCursor),
    queryFn: ({ signal }) => fetchJobs(filters, currentCursor, urlState.limit, signal),
    staleTime: 30_000,
  })

  const currentPage = query.data

  // Merge each newly-fetched page into the accumulated list exactly once.
  // The first page (`currentCursor === ''`) always REPLACES rather than
  // appends — covers both the initial load and the page right after a reset.
  useEffect(() => {
    if (!currentPage) return
    if (currentCursor === '') {
      mergedCursorsRef.current = new Set([''])
      setAccumulatedJobs(currentPage.items)
      return
    }
    if (mergedCursorsRef.current.has(currentCursor)) return
    mergedCursorsRef.current.add(currentCursor)
    setAccumulatedJobs((prev) => [...prev, ...currentPage.items])
  }, [currentPage, currentCursor])

  const jobs = accumulatedJobs
  const itemsOnPage = jobs.length

  const hasNextPage = currentPage?.next_cursor ? currentPage.next_cursor !== '' : false
  // True only while fetching a page BEYOND the first — the first page uses
  // the table's own full-height loading state instead of the sentinel row.
  const isFetchingMore = query.isFetching && currentCursor !== ''

  const countsQuery = useQuery({
    queryKey: jobQueryKeys.counts(),
    queryFn: fetchJobCounts,
    staleTime: 30_000,
  })

  const counts: Partial<Record<TabKey, number>> = countsQuery.data
    ? {
        new: countsQuery.data.new,
        saved: countsQuery.data.saved,
        applied: countsQuery.data.applied,
        interviewing: countsQuery.data.interviewing,
        offer: countsQuery.data.offer,
        rejected: countsQuery.data.rejected,
        // Combined Tracking-tab badge — sum of the 4 workflow stages. Computed
        // client-side from the existing per-status counts; no backend change.
        tracking:
          countsQuery.data.applied +
          countsQuery.data.interviewing +
          countsQuery.data.offer +
          countsQuery.data.rejected,
      }
    : {}

  const companiesQuery = useQuery({
    queryKey: jobQueryKeys.companies(),
    queryFn: fetchJobCompanies,
    staleTime: 5 * 60_000,
  })
  const companies = companiesQuery.data ?? []

  function handleSortingChange(newSorting: SortingState) {
    const next = newSorting[0]
    if (!next) {
      void urlState.setSort('')
      void urlState.setDir('asc')
    } else {
      const dir: SortDir = next.desc ? 'desc' : 'asc'
      void urlState.setSort(next.id)
      void urlState.setDir(dir)
    }
    // urlState setters already reset page to 0.
    // filterFingerprint will change → useEffect resets cursorHistory.
  }

  // Fetches the next page and appends it to the accumulated list (via the
  // merge effect above) — the infinite-scroll sentinel's only entry point.
  // No-ops while already fetching or once the last page has been reached.
  function loadMore() {
    if (query.isFetching || !hasNextPage) return
    const nextCursor = query.data?.next_cursor
    if (!nextCursor) return

    const extended = cursorHistory.slice(0, pageIndex + 1).concat(nextCursor)
    setCursorHistory(extended)
    persistHistory(filterFingerprint, extended)
    void setPageIndex(pageIndex + 1)
  }

  function resetFilters() {
    clearHistory(filterFingerprint)
    setCursorHistory([''])
    setAccumulatedJobs([])
    mergedCursorsRef.current = new Set()
    urlState.resetFilters()
    setSearchInput('')
    // urlState.resetFilters already writes page: 0
  }

  // Wrappers that proxy URL-state setters but keep the existing useJobs
  // public surface so consumers (JobsPage, JobFilters) don't change shape.
  // Each of these changes filters → filterFingerprint changes → useEffect
  // resets cursorHistory automatically.
  function setTab(tab: TabKey) {
    void urlState.setTab(tab)
  }
  function setSelectedCompanies(cos: string[]) {
    void urlState.setCompanies(cos)
  }
  function setLocationFilter(loc: LocationFilter) {
    void urlState.setLoc(loc)
  }
  function setExpFilter(exp: string[]) {
    void urlState.setExp(exp)
  }
  function setDateFilter(since: DateFilter) {
    void urlState.setSince(since)
  }
  function setVisaFilter(visa: VisaFilter) {
    void urlState.setVisa(visa)
  }
  function setTrackingStages(stages: TrackingStage[]) {
    void urlState.setTrackingStages(stages)
  }

  return {
    // Query state
    jobs,
    hasNextPage,
    itemsOnPage,
    companies,
    isLoading: query.isLoading,
    isError: query.isError,
    isFetching: query.isFetching,
    isFetchingMore,
    refetch: query.refetch,
    counts,

    // Tab
    tab: urlState.tab,
    setTab,
    trackingStages: urlState.trackingStages as TrackingStage[],
    setTrackingStages,

    // Open job preview (URL-driven)
    openJobId: urlState.openJobId,
    setOpenJobId: urlState.setOpenJobId,

    // Search — raw input value is local; URL holds the debounced value
    searchQuery: searchInput,
    setSearchQuery: setSearchInput,

    // Filters
    selectedCompanies: urlState.companies,
    setSelectedCompanies,
    locationFilter: urlState.loc,
    setLocationFilter,
    expFilter: urlState.exp,
    setExpFilter,
    dateFilter: urlState.since,
    setDateFilter,
    visaFilter: urlState.visa,
    setVisaFilter,
    resetFilters,

    // Sorting
    sorting,
    handleSortingChange,

    // Infinite scroll
    loadMore,
    limit: urlState.limit,
  }
}
