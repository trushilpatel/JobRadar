import { useEffect, useMemo, useRef, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { fetchCompanies, fetchQueue, type CompanyFilters } from '@/api/companies'
import type { SortingState } from '@/components/ui/data-table'
import { QUEUE_STATUS, SCAN_STATUS, STATUS_FILTER, ACTIVE_FILTER } from '@/features/companies/constants'

export type ActiveFilter = 'all' | 'active' | 'inactive'
export type StatusFilter = 'none' | 'error' | 'running' | 'queued'

export function useCompanies() {
  const [search, setSearch] = useState('')
  const [debouncedSearch, setDebouncedSearch] = useState('')
  const [adapters, setAdapters] = useState<string[]>([])
  const [activeFilter, setActiveFilter] = useState<ActiveFilter>(ACTIVE_FILTER.ALL)
  const [statusFilter, setStatusFilter] = useState<StatusFilter>(STATUS_FILTER.NONE)
  const [sorting, setSorting] = useState<SortingState>([])

  const [page, setPage] = useState(1)
  const [limit, setLimit] = useState(20)

  const debounceRef = useRef<ReturnType<typeof setTimeout> | null>(null)
  function handleSearchChange(v: string) {
    setSearch(v)
    if (debounceRef.current) clearTimeout(debounceRef.current)
    debounceRef.current = setTimeout(() => setDebouncedSearch(v), 300)
  }

  const sortBy = sorting[0]?.id
  const sortDir = sorting[0] ? (sorting[0].desc ? 'desc' : 'asc') : undefined

  // Server-side filters. 'error' is server-side; 'running'/'queued' require the
  // queue table and remain client-side refinements below.
  const serverFilters = useMemo<CompanyFilters>(
    () => ({
      search: debouncedSearch || undefined,
      adapterTypes: adapters.length > 0 ? adapters : undefined,
      active:
        activeFilter === ACTIVE_FILTER.ACTIVE
          ? true
          : activeFilter === ACTIVE_FILTER.INACTIVE
            ? false
            : undefined,
      lastScanStatus: statusFilter === STATUS_FILTER.ERROR ? SCAN_STATUS.ERROR : undefined,
      sortBy,
      sortDir,
    }),
    [debouncedSearch, adapters, activeFilter, statusFilter, sortBy, sortDir]
  )

  useEffect(() => {
    // eslint-disable-next-line react-hooks/set-state-in-effect
    setPage(1)
  }, [serverFilters, statusFilter])

  // running/queued are client-side filters that need the full dataset — fetch all
  // so the filter is not limited to the current page.
  const effectiveLimit =
    statusFilter === STATUS_FILTER.RUNNING || statusFilter === STATUS_FILTER.QUEUED ? 9999 : limit

  const companiesQuery = useQuery({
    queryKey: ['companies', serverFilters, page, effectiveLimit],
    queryFn: () => fetchCompanies(serverFilters, page, effectiveLimit),
    refetchInterval: 30_000,
  })

  const queueQuery = useQuery({
    queryKey: ['companies', 'queue'],
    queryFn: fetchQueue,
    refetchInterval: 15_000,
  })

  const queueMap = useMemo(() => {
    const m = new Map<string, 'queued' | 'running'>()
    for (const e of queueQuery.data ?? []) m.set(e.company_id, e.status)
    return m
  }, [queueQuery.data])

  const rawCompanies = companiesQuery.data?.data ?? []
  const total = companiesQuery.data?.total ?? 0
  const totalActive = companiesQuery.data?.total_active ?? 0

  // Client-side refinement for running/queued only — these require queue data.
  const companies = useMemo(() => {
    if (statusFilter === STATUS_FILTER.RUNNING) {
      return rawCompanies.filter((c) => queueMap.get(c.id) === QUEUE_STATUS.RUNNING)
    }
    if (statusFilter === STATUS_FILTER.QUEUED) {
      return rawCompanies.filter((c) => queueMap.get(c.id) === QUEUE_STATUS.QUEUED)
    }
    return rawCompanies
  }, [rawCompanies, statusFilter, queueMap])

  // Counts for filter bar / KPI strip.
  const counts = useMemo(() => {
    const errors = rawCompanies.filter((c) => c.last_scan_status === SCAN_STATUS.ERROR).length
    const running = rawCompanies.filter((c) => queueMap.get(c.id) === QUEUE_STATUS.RUNNING).length
    const queued = rawCompanies.filter((c) => queueMap.get(c.id) === QUEUE_STATUS.QUEUED).length
    return { errors, running, queued }
  }, [rawCompanies, queueMap])

  function handleSortingChange(newSorting: SortingState) {
    setSorting(newSorting)
    setPage(1)
  }

  function resetFilters() {
    setSearch('')
    setDebouncedSearch('')
    setAdapters([])
    setActiveFilter(ACTIVE_FILTER.ALL)
    setStatusFilter(STATUS_FILTER.NONE)
    setSorting([])
    setPage(1)
  }

  return {
    companies,
    total,
    totalActive,
    counts,
    queueMap,
    loading:
      companiesQuery.isLoading ||
      ((statusFilter === STATUS_FILTER.RUNNING || statusFilter === STATUS_FILTER.QUEUED) &&
        queueQuery.isLoading),
    refetch: async () => {
      await Promise.all([companiesQuery.refetch(), queueQuery.refetch()])
    },

    search,
    handleSearchChange,
    adapters,
    setAdapters,
    activeFilter,
    setActiveFilter,
    statusFilter,
    setStatusFilter,

    sorting,
    handleSortingChange,

    page,
    setPage,
    limit,
    setLimit,

    resetFilters,
  }
}
