import { useState, useMemo } from 'react'
import { useInfiniteQuery, useQuery } from '@tanstack/react-query'
import { useLocation } from 'react-router-dom'
import { fetchJobs, fetchJobCounts, fetchJobCompanies, type JobFilters } from '@/api/jobs'
import type { SortingState } from '@/components/ui/data-table'
import { useDebouncedValue } from '@/lib/use-debounced-value'

export type TabKey = 'new' | 'saved' | 'applied' | 'interviewing' | 'offer' | 'rejected'
export type DateFilter = 'all' | '24h' | '7d' | '30d'
export type LocationFilter = 'any' | 'remote' | 'onsite'

export function useJobs() {
  const location = useLocation()
  const initialDateFilter: DateFilter =
    (location.state as { dateFilter?: DateFilter } | null)?.dateFilter ?? 'all'

  // Tab
  const [tab, setTab] = useState<TabKey>('new')

  // Filter state
  const initialCompanies = (location.state as { companies?: string[] } | null)?.companies ?? []
  const [selectedCompanies, setSelectedCompanies] = useState<string[]>(initialCompanies)
  const [locationFilter, setLocationFilter] = useState<LocationFilter>('any')
  const [expFilter, setExpFilter] = useState<string[]>([])
  const [dateFilter, setDateFilter] = useState<DateFilter>(initialDateFilter)

  // Search: raw value drives the input; debounced value drives the API query
  const [searchQuery, setSearchQuery] = useState('')
  const debouncedSearch = useDebouncedValue(searchQuery, 300)

  // Sorting
  const [sorting, setSorting] = useState<SortingState>([])

  // Cursor-based pagination via useInfiniteQuery: pages are walked one at a time;
  // pageIndex is the currently displayed page within the fetched pages array.
  const [pageIndex, setPageIndex] = useState(0)
  const [limit, setLimit] = useState(20)

  const sortBy = sorting[0]?.id
  const sortDir = sorting[0] ? (sorting[0].desc ? 'desc' : 'asc') : undefined

  const filters = useMemo<JobFilters>(
    () => ({
      status: tab,
      search: debouncedSearch || undefined,
      companies: selectedCompanies.length > 0 ? selectedCompanies : undefined,
      remote: locationFilter === 'remote' || undefined,
      onSite: locationFilter === 'onsite' || undefined,
      experienceRanges: expFilter.length > 0 ? expFilter : undefined,
      daysAdded:
        dateFilter === '24h' ? 1 : dateFilter === '7d' ? 7 : dateFilter === '30d' ? 30 : undefined,
      sortBy,
      sortDir,
    }),
    [tab, debouncedSearch, selectedCompanies, locationFilter, expFilter, dateFilter, sortBy, sortDir]
  )

  // Reset pageIndex when filters/limit change. Adjusting state during render
  // (storing previous value + comparing) is the React-recommended alternative
  // to useEffect for derived state resets.
  const [prevKey, setPrevKey] = useState({ filters, limit })
  if (prevKey.filters !== filters || prevKey.limit !== limit) {
    setPrevKey({ filters, limit })
    setPageIndex(0)
  }

  const query = useInfiniteQuery({
    queryKey: ['jobs', filters, limit],
    queryFn: ({ pageParam, signal }) => fetchJobs(filters, pageParam, limit, signal),
    initialPageParam: '',
    getNextPageParam: (last) => (last.next_cursor ? last.next_cursor : undefined),
    staleTime: 30_000,
  })

  const fetchedPages = query.data?.pages ?? []
  const currentPage = fetchedPages[pageIndex]
  const jobs = useMemo(() => currentPage?.items ?? [], [currentPage])
  const itemsOnPage = jobs.length

  const hasPrevPage = pageIndex > 0
  const hasNextPage =
    pageIndex < fetchedPages.length - 1 || (query.hasNextPage ?? false)

  const countsQuery = useQuery({
    queryKey: ['jobs', 'counts'],
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
    queryKey: ['jobs', 'companies'],
    queryFn: fetchJobCompanies,
    staleTime: 5 * 60_000,
  })
  const companies = companiesQuery.data ?? []

  function resetFilters() {
    setSearchQuery('')
    setSelectedCompanies([])
    setLocationFilter('any')
    setExpFilter([])
    setDateFilter('all')
    setSorting([])
    setPageIndex(0)
  }

  function handleSortingChange(newSorting: SortingState) {
    setSorting(newSorting)
    setPageIndex(0)
  }

  function goToNextPage() {
    if (pageIndex < fetchedPages.length - 1) {
      setPageIndex((i) => i + 1)
    } else if (query.hasNextPage && !query.isFetchingNextPage) {
      void query.fetchNextPage().then((res) => {
        // Advance only if the fetch added a new page
        if ((res.data?.pages.length ?? 0) > fetchedPages.length) {
          setPageIndex((i) => i + 1)
        }
      })
    }
  }

  function goToPrevPage() {
    if (pageIndex > 0) setPageIndex((i) => i - 1)
  }

  function goToFirstPage() {
    setPageIndex(0)
  }

  return {
    // Query state
    jobs,
    hasNextPage,
    hasPrevPage,
    page: pageIndex + 1, // 1-indexed for display
    itemsOnPage,
    companies,
    isLoading: query.isLoading,
    isFetchingNextPage: query.isFetchingNextPage,
    refetch: query.refetch,
    counts,

    // Tab
    tab,
    setTab,

    // Search
    searchQuery,
    setSearchQuery,

    // Filters
    selectedCompanies,
    setSelectedCompanies,
    locationFilter,
    setLocationFilter,
    expFilter,
    setExpFilter,
    dateFilter,
    setDateFilter,
    resetFilters,

    // Sorting
    sorting,
    handleSortingChange,

    // Pagination
    goToNextPage,
    goToPrevPage,
    goToFirstPage,
    limit,
    setLimit,
  }
}
