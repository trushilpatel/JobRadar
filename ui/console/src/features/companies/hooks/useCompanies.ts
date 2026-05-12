import { useMemo, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { fetchAdminCompanies } from '@/api/adminCompanies'
import type { SortingState } from '@/components/ui/data-table'
import { SCAN_STATUS_FILTER, ACTIVE_FILTER, STORAGE_KEYS } from '@/features/companies/constants'
import type { ScanStatusFilter, ActiveFilter } from '@/features/companies/constants'
import { companyQueryKeys } from '@/features/companies/queryKeys'
import { normalizeAdminCompany } from './normalizeAdminCompany'
import { useDebouncedValue } from '@/lib/use-debounced-value'

export type { ActiveFilter, ScanStatusFilter }

export function useCompanies() {
  const [search, setSearch] = useState('')
  const debouncedSearch = useDebouncedValue(search, 300)
  const [adapters, setAdapters] = useState<string[]>([])
  const [activeFilter, setActiveFilter] = useState<ActiveFilter>(ACTIVE_FILTER.ALL)
  const [scanStatus, setScanStatus] = useState<ScanStatusFilter>(SCAN_STATUS_FILTER.ALL)
  const [sorting, setSorting] = useState<SortingState>([])
  const [page, setPage] = useState(1)
  const [limit, setLimit] = useState(20)
  const [autoRefresh, setAutoRefresh] = useState(
    () => localStorage.getItem(STORAGE_KEYS.AUTO_REFRESH) === 'true'
  )

  const sortBy = sorting[0]?.id
  const sortDir: 'asc' | 'desc' | undefined = sorting[0]
    ? sorting[0].desc
      ? 'desc'
      : 'asc'
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

  const [prevFilters, setPrevFilters] = useState(filters)
  if (prevFilters !== filters) {
    setPrevFilters(filters)
    setPage(1)
  }

  const { data, isLoading, isFetching, refetch } = useQuery({
    queryKey: companyQueryKeys.adminCompanies(filters, page, limit),
    queryFn: () => fetchAdminCompanies(filters, page, limit),
    staleTime: 10_000,
    refetchInterval: autoRefresh ? 10_000 : false,
  })

  const companies = useMemo(() => (data?.data ?? []).map(normalizeAdminCompany), [data?.data])
  const total = data?.total ?? 0
  const totalActive = data?.total_active ?? 0

  const hasActiveFilters =
    search !== '' ||
    adapters.length > 0 ||
    activeFilter !== ACTIVE_FILTER.ALL ||
    scanStatus !== SCAN_STATUS_FILTER.ALL ||
    sorting.length > 0

  function handleSortingChange(newSorting: SortingState) {
    setSorting(newSorting)
    setPage(1)
  }

  function resetFilters() {
    setSearch('')
    setAdapters([])
    setActiveFilter(ACTIVE_FILTER.ALL)
    setScanStatus(SCAN_STATUS_FILTER.ALL)
    setSorting([])
    setPage(1)
  }

  function toggleAutoRefresh() {
    setAutoRefresh((prev) => {
      const next = !prev
      localStorage.setItem(STORAGE_KEYS.AUTO_REFRESH, String(next))
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
    handleSearchChange: setSearch,
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
