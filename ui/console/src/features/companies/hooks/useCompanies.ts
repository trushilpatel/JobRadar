import { useEffect, useMemo, useRef, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { fetchAdminCompanies } from '@/api/adminCompanies'
import type { SortingState } from '@/components/ui/data-table'
import { SCAN_STATUS_FILTER, ACTIVE_FILTER } from '@/features/companies/constants'
import type { ScanStatusFilter, ActiveFilter } from '@/features/companies/constants'
import { normalizeAdminCompany } from './normalizeAdminCompany'

export type { ActiveFilter, ScanStatusFilter }

export function useCompanies() {
  const [search, setSearch] = useState('')
  const [debouncedSearch, setDebouncedSearch] = useState('')
  const [adapters, setAdapters] = useState<string[]>([])
  const [activeFilter, setActiveFilter] = useState<ActiveFilter>(ACTIVE_FILTER.ALL)
  const [scanStatus, setScanStatus] = useState<ScanStatusFilter>(SCAN_STATUS_FILTER.ALL)
  const [sorting, setSorting] = useState<SortingState>([])
  const [page, setPage] = useState(1)
  const [limit, setLimit] = useState(20)
  const [autoRefresh, setAutoRefresh] = useState(
    () => localStorage.getItem('companies-auto-refresh') === 'true'
  )

  const debounceRef = useRef<ReturnType<typeof setTimeout> | null>(null)
  function handleSearchChange(v: string) {
    setSearch(v)
    if (debounceRef.current) clearTimeout(debounceRef.current)
    debounceRef.current = setTimeout(() => setDebouncedSearch(v), 300)
  }

  const sortBy = sorting[0]?.id
  const sortDir: 'asc' | 'desc' | undefined = sorting[0]
    ? sorting[0].desc ? 'desc' : 'asc'
    : undefined

  const filters = useMemo(
    () => ({
      search: debouncedSearch || undefined,
      adapterTypes: adapters.length > 0 ? adapters : undefined,
      active:
        activeFilter === ACTIVE_FILTER.ACTIVE
          ? true
          : activeFilter === ACTIVE_FILTER.INACTIVE
            ? false
            : undefined,
      scanStatus: scanStatus === SCAN_STATUS_FILTER.ALL ? undefined : scanStatus,
      sortBy,
      sortDir,
    }),
    [debouncedSearch, adapters, activeFilter, scanStatus, sortBy, sortDir]
  )

  useEffect(() => {
    // eslint-disable-next-line react-hooks/set-state-in-effect
    setPage(1)
  }, [filters])

  const { data, isLoading, isFetching, refetch } = useQuery({
    queryKey: ['admin-companies', filters, page, limit],
    queryFn: () => fetchAdminCompanies(filters, page, limit),
    staleTime: 30_000,
    refetchInterval: autoRefresh ? 30_000 : false,
  })

  const companies = useMemo(
    () => (data?.data ?? []).map(normalizeAdminCompany),
    [data?.data]
  )
  const total = data?.total ?? 0
  const totalActive = data?.total_active ?? 0

  const hasActiveFilters =
    search !== '' ||
    adapters.length > 0 ||
    activeFilter !== ACTIVE_FILTER.ALL ||
    scanStatus !== SCAN_STATUS_FILTER.ALL

  function handleSortingChange(newSorting: SortingState) {
    setSorting(newSorting)
    setPage(1)
  }

  function resetFilters() {
    setSearch('')
    setDebouncedSearch('')
    setAdapters([])
    setActiveFilter(ACTIVE_FILTER.ALL)
    setScanStatus(SCAN_STATUS_FILTER.ALL)
    setSorting([])
    setPage(1)
  }

  function toggleAutoRefresh() {
    setAutoRefresh((prev) => {
      const next = !prev
      localStorage.setItem('companies-auto-refresh', String(next))
      return next
    })
  }

  return {
    companies,
    total,
    totalActive,
    isLoading,
    isFetching,
    refetch,

    search,
    handleSearchChange,
    adapters,
    setAdapters,
    activeFilter,
    setActiveFilter,
    scanStatus,
    setScanStatus,

    sorting,
    handleSortingChange,

    page,
    setPage,
    limit,
    setLimit,

    hasActiveFilters,
    resetFilters,

    autoRefresh,
    toggleAutoRefresh,
  }
}
