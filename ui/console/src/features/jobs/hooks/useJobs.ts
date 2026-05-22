import { useEffect, useMemo, useState } from 'react'
import { useInfiniteQuery, useQuery } from '@tanstack/react-query'
import { fetchJobs, fetchJobCounts, fetchJobCompanies, type JobFilters } from '@/api/jobs'
import type { SortingState } from '@/components/ui/data-table'
import { useDebouncedValue } from '@/lib/use-debounced-value'
import { jobQueryKeys } from '@/features/jobs/queryKeys'
import { type TabKey } from '@/constants/jobStatus'
import {
  useJobUrlState,
  type DateFilter,
  type LocationFilter,
  type SortDir,
} from '@/features/jobs/hooks/useJobUrlState'

export type { TabKey } from '@/constants/jobStatus'
export type { DateFilter, LocationFilter } from '@/features/jobs/hooks/useJobUrlState'

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

  // Cursor-based pagination via useInfiniteQuery: pages are walked one at a
  // time; pageIndex is the currently displayed page within the fetched pages
  // array. Sourced from the URL so refresh and share-links land on the same
  // page.
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

  // Filters/limit changing is the URL setters' responsibility now (they each
  // reset `page` to 0). No local-state catch-up needed.

  const query = useInfiniteQuery({
    queryKey: jobQueryKeys.list(filters, urlState.limit),
    queryFn: ({ pageParam, signal }) => fetchJobs(filters, pageParam, urlState.limit, signal),
    initialPageParam: '',
    getNextPageParam: (last) => (last.next_cursor ? last.next_cursor : undefined),
    staleTime: 30_000,
  })

  const fetchedPages = useMemo(() => query.data?.pages ?? [], [query.data])
  const currentPage = fetchedPages[pageIndex]
  const jobs = useMemo(() => currentPage?.items ?? [], [currentPage])
  const itemsOnPage = jobs.length

  // Auto-snap: if a refetch returns fewer pages than the cursor (e.g. the
  // last item was dismissed off the current page), bounce the user to the
  // last available page rather than leaving them on a phantom empty page.
  useEffect(() => {
    if (query.isFetching) return
    if (fetchedPages.length === 0) {
      if (pageIndex !== 0) void setPageIndex(0)
      return
    }
    if (pageIndex > fetchedPages.length - 1) {
      void setPageIndex(Math.max(0, fetchedPages.length - 1))
    }
  }, [fetchedPages.length, pageIndex, query.isFetching, setPageIndex])

  // Cumulative number of items in pages BEFORE the current page. Drives the
  // pagination "Showing X–Y" display. Pages on the "new" tab are sparse
  // (per-user overlay filters items out after cursor pagination), so the
  // theoretical (page-1)*limit math under-counts. This is exact.
  const itemsBeforePage = useMemo(
    () => fetchedPages.slice(0, pageIndex).reduce((sum, p) => sum + (p.items?.length ?? 0), 0),
    [fetchedPages, pageIndex]
  )

  const hasPrevPage = pageIndex > 0
  const hasNextPage = pageIndex < fetchedPages.length - 1 || (query.hasNextPage ?? false)

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
  }

  function goToNextPage() {
    // Pre-fetched next page — advance immediately.
    if (pageIndex < fetchedPages.length - 1) {
      void setPageIndex(pageIndex + 1)
      return
    }
    // At the boundary — initiate fetch and advance eagerly. The new page lands
    // at fetchedPages[pageIndex+1] when the fetch resolves; until then the
    // table renders the loading state (currentPage is undefined).
    if (query.hasNextPage && !query.isFetchingNextPage) {
      void query.fetchNextPage()
      void setPageIndex(pageIndex + 1)
    }
  }

  function goToPrevPage() {
    if (pageIndex > 0) void setPageIndex(pageIndex - 1)
  }

  function goToFirstPage() {
    void setPageIndex(0)
  }

  function resetFilters() {
    urlState.resetFilters()
    setSearchInput('')
    // urlState.resetFilters already writes page: 0
  }

  // Wrappers that proxy URL-state setters but keep the existing useJobs
  // public surface so consumers (JobsPage, JobFilters) don't change shape.
  function setTab(tab: TabKey) {
    void urlState.setTab(tab)
  }
  function setSelectedCompanies(companies: string[]) {
    void urlState.setCompanies(companies)
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
    isFetchingNextPage: query.isFetchingNextPage,
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
