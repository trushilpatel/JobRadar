import { useState, useMemo, useEffect, useRef } from 'react'
import { useQuery } from '@tanstack/react-query'
import { useLocation } from 'react-router-dom'
import { fetchJobs, fetchJobCounts, fetchJobCompanies, type JobFilters } from '@/api/jobs'
import type { SortingState } from '@/components/ui/data-table'

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
  // Single 3-way location filter — replaces the old pair of independent booleans
  const [locationFilter, setLocationFilter] = useState<LocationFilter>('any')
  const [expFilter, setExpFilter] = useState<string[]>([])
  const [dateFilter, setDateFilter] = useState<DateFilter>(initialDateFilter)

  // Search: raw value drives the input; debouncedSearch drives the API query
  const [searchQuery, setSearchQueryRaw] = useState('')
  const [debouncedSearch, setDebouncedSearch] = useState('')
  const debounceRef = useRef<ReturnType<typeof setTimeout> | null>(null)

  function setSearchQuery(value: string) {
    setSearchQueryRaw(value)
    if (debounceRef.current) clearTimeout(debounceRef.current)
    debounceRef.current = setTimeout(() => setDebouncedSearch(value), 300)
  }

  useEffect(() => {
    return () => {
      if (debounceRef.current) clearTimeout(debounceRef.current)
    }
  }, [])

  // Sorting
  const [sorting, setSorting] = useState<SortingState>([])

  // Cursor-based pagination: cursors[0]="" (first page), cursors[n] = next_cursor from page n-1
  const [cursors, setCursors] = useState<string[]>([''])
  const [limit, setLimit] = useState(20)
  const page = cursors.length // 1-indexed page number for display

  const sortBy = sorting[0]?.id
  const sortDir = sorting[0] ? (sorting[0].desc ? 'desc' : 'asc') : undefined

  // Stable filter object — debouncedSearch prevents a query per keystroke
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

  // Reset cursor to page 1 when filters change. Adjusting state during render
  // (storing previous value + comparing) is the React-recommended alternative to
  // useEffect for derived state resets — avoids a second render pass from the effect.
  const [prevFilters, setPrevFilters] = useState(filters)
  if (prevFilters !== filters) {
    setPrevFilters(filters)
    setCursors([''])
  }

  const currentCursor = cursors[cursors.length - 1]

  const query = useQuery({
    queryKey: ['jobs', filters, currentCursor, limit],
    queryFn: () => fetchJobs(filters, currentCursor, limit),
    staleTime: 30_000,
  })

  const countsQuery = useQuery({
    queryKey: ['jobs', 'counts'],
    queryFn: fetchJobCounts,
    staleTime: 30_000,
  })

  // Stable reference — avoids companies useMemo invalidating on every render
  // when query.data?.items is undefined (the `?? []` fallback creates a new array each time).
  const jobs = useMemo(() => query.data?.items ?? [], [query.data])
  const nextCursor = query.data?.next_cursor ?? ''
  const hasNextPage = nextCursor !== ''
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
    setSearchQueryRaw('')
    setDebouncedSearch('')
    if (debounceRef.current) clearTimeout(debounceRef.current)
    setSelectedCompanies([])
    setLocationFilter('any')
    setExpFilter([])
    setDateFilter('all')
    setSorting([])
    setCursors([''])
  }

  function handleSortingChange(newSorting: SortingState) {
    setSorting(newSorting)
    setCursors([''])
  }

  function goToNextPage() {
    if (hasNextPage) setCursors((prev) => [...prev, nextCursor])
  }

  function goToPrevPage() {
    if (cursors.length > 1) setCursors((prev) => prev.slice(0, -1))
  }

  function goToFirstPage() {
    setCursors([''])
  }

  return {
    // Query state
    jobs,
    hasNextPage,
    hasPrevPage: cursors.length > 1,
    page,
    companies,
    isLoading: query.isLoading,
    refetch: query.refetch,
    counts,

    // Tab
    tab,
    setTab,

    // Search — searchQuery is the raw input value; debouncedSearch drives the API
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
