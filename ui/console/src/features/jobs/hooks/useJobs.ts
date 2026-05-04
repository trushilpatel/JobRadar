import { useState, useMemo, useEffect, useRef } from 'react'
import { useQuery } from '@tanstack/react-query'
import { useLocation } from 'react-router-dom'
import { fetchJobs, fetchJobCounts, type JobFilters } from '@/api/jobs'
import type { SortingState } from '@/components/ui/data-table'

export type TabKey = 'new' | 'saved' | 'applied' | 'interviewing' | 'offer'
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

  // Pagination
  const [page, setPage] = useState(1)
  const [limit, setLimit] = useState(20)

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

  // Reset to page 1 when filters change
  useEffect(() => {
    setPage(1)
  }, [filters])

  const query = useQuery({
    queryKey: ['jobs', filters, page, limit],
    queryFn: () => fetchJobs(filters, page, limit),
    staleTime: 30_000,
  })

  const countsQuery = useQuery({
    queryKey: ['jobs', 'counts'],
    queryFn: fetchJobCounts,
    staleTime: 30_000,
  })

  // Stable reference — avoids companies useMemo invalidating on every render
  // when query.data?.data is undefined (the `?? []` fallback creates a new array each time).
  const jobs = useMemo(() => query.data?.data ?? [], [query.data])
  const total = query.data?.total ?? 0
  const counts: Partial<Record<TabKey, number>> = countsQuery.data
    ? {
        new: countsQuery.data.new,
        saved: countsQuery.data.saved,
        applied: countsQuery.data.applied,
        interviewing: countsQuery.data.interviewing,
        offer: countsQuery.data.offer,
      }
    : {}

  // ── Accumulated companies ──────────────────────────────────────────────────
  // Companies seen across all pages for the current filter context.
  // Resets when any non-company filter changes (tab, search, location, exp, date)
  // so the list stays relevant to the current query without a separate API call.
  const companyAccumKey = `${tab}|${debouncedSearch}|${locationFilter}|${expFilter.join(',')}|${dateFilter}`
  const seenCompaniesRef = useRef<{ key: string; names: Set<string> }>({
    key: '',
    names: new Set(),
  })
  if (seenCompaniesRef.current.key !== companyAccumKey) {
    seenCompaniesRef.current = { key: companyAccumKey, names: new Set() }
  }
  jobs.forEach((j) => {
    const name = j.company?.name ?? j.company_id
    if (name) seenCompaniesRef.current.names.add(name as string)
  })
  const companies = useMemo(
    () => Array.from(seenCompaniesRef.current.names).sort(),
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [jobs] // jobs changing means new companies may have arrived
  )

  function resetFilters() {
    setSearchQueryRaw('')
    setDebouncedSearch('')
    if (debounceRef.current) clearTimeout(debounceRef.current)
    setSelectedCompanies([])
    setLocationFilter('any')
    setExpFilter([])
    setDateFilter('all')
    setSorting([])
    setPage(1)
  }

  function handleSortingChange(newSorting: SortingState) {
    setSorting(newSorting)
    setPage(1)
  }

  return {
    // Query state
    jobs,
    total,
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
    page,
    setPage,
    limit,
    setLimit,
  }
}
