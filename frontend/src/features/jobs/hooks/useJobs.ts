import { useEffect, useMemo, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { fetchJobs, fetchJobCounts, fetchJobCompanies, type JobFilters } from '../api/jobs.api'
import type { SortingState } from '@/components/ui/data-table'
import { useDebouncedValue } from '@/lib/hooks/use-debounced-value'
import { jobQueryKeys } from '../queryKeys'
import { type TabKey } from '@/config/constants/jobStatus'
import {
  useJobUrlState,
  type DateFilter,
  type LocationFilter,
  type SortDir,
} from './useJobUrlState'

export type { TabKey } from '@/config/constants/jobStatus'
export type { DateFilter, LocationFilter } from './useJobUrlState'

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

  const filters = useMemo<JobFilters>(
    () => ({
      status: urlState.tab,
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
      sortBy,
      sortDir,
    }),
    [
      urlState.tab,
      urlState.q,
      urlState.companies,
      urlState.loc,
      urlState.exp,
      urlState.since,
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
        tab: filters.status,
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
        sort: filters.sortBy ?? '',
        dir: filters.sortDir ?? 'asc',
        limit: urlState.limit,
      }),
    [filters, urlState.limit]
  )

  // Cursor history in React state, seeded from sessionStorage on mount.
  // Reset to [''] whenever filters change (fingerprint changes).
  const [cursorHistory, setCursorHistory] = useState<string[]>(() =>
    restoreHistory(filterFingerprint)
  )

  // When filters change, reset cursor history. The useEffect compares the
  // fingerprint so this only fires on genuine filter/limit changes, not on
  // every render.
  useEffect(() => {
    setCursorHistory(restoreHistory(filterFingerprint))
  }, [filterFingerprint])

  const currentCursor = cursorHistory[pageIndex] ?? ''

  const query = useQuery({
    queryKey: jobQueryKeys.list(filters, urlState.limit, currentCursor),
    queryFn: ({ signal }) => fetchJobs(filters, currentCursor, urlState.limit, signal),
    staleTime: 30_000,
  })

  const currentPage = query.data
  const jobs = useMemo(() => currentPage?.items ?? [], [currentPage])
  const itemsOnPage = jobs.length

  // Approximate number of items before the current page.
  // With useQuery we don't accumulate pages, so pageIndex * limit is used
  // as an approximation. This is exact on non-sparse tabs and close on "new".
  const itemsBeforePage = useMemo(() => pageIndex * urlState.limit, [pageIndex, urlState.limit])

  const hasPrevPage = pageIndex > 0
  const hasNextPage = currentPage?.next_cursor ? currentPage.next_cursor !== '' : false

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

  function goToNextPage() {
    const nextCursor = query.data?.next_cursor
    if (!nextCursor) return // last page — no-op

    // Slice to current position + 1 to handle the case where the user went
    // back and is now going forward again (discards any stale future cursors).
    const extended = cursorHistory.slice(0, pageIndex + 1).concat(nextCursor)
    setCursorHistory(extended)
    persistHistory(filterFingerprint, extended)
    void setPageIndex(pageIndex + 1)
  }

  function goToPrevPage() {
    if (pageIndex > 0) void setPageIndex(pageIndex - 1)
    // The cursor for the previous page is already in cursorHistory — no fetch needed.
  }

  function goToFirstPage() {
    void setPageIndex(0)
  }

  function resetFilters() {
    clearHistory(filterFingerprint)
    setCursorHistory([''])
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
  function setLimit(limit: number) {
    void urlState.setLimit(limit)
  }

  return {
    // Query state
    jobs,
    hasNextPage,
    hasPrevPage,
    page: pageIndex + 1, // 1-indexed for display
    itemsOnPage,
    itemsBeforePage,
    companies,
    isLoading: query.isLoading,
    isError: query.isError,
    isFetching: query.isFetching,
    refetch: query.refetch,
    counts,

    // Tab
    tab: urlState.tab,
    setTab,

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
    resetFilters,

    // Sorting
    sorting,
    handleSortingChange,

    // Pagination
    goToNextPage,
    goToPrevPage,
    goToFirstPage,
    limit: urlState.limit,
    setLimit,
  }
}
