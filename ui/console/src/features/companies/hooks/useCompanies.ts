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
  const [limit, setLimit] = useState(20)
  const [cursor, setCursor] = useState('')
  const [cursorHistory, setCursorHistory] = useState<string[]>([])
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

  // Reset cursor when filters change
  const [prevFilters, setPrevFilters] = useState(filters)
  if (prevFilters !== filters) {
    setPrevFilters(filters)
    setCursor('')
    setCursorHistory([])
  }

  const { data, isLoading, isFetching, refetch } = useQuery({
    queryKey: companyQueryKeys.adminCompanies(filters, cursor, limit),
    queryFn: () => fetchAdminCompanies(filters, cursor, limit),
    staleTime: 10_000,
    refetchInterval: autoRefresh ? 10_000 : false,
  })

  const companies = useMemo(() => (data?.data ?? []).map(normalizeAdminCompany), [data?.data])
  const total = data?.total ?? 0
  const totalActive = data?.total_active ?? 0
  const nextCursor = data?.next_cursor ?? null

  const page = cursorHistory.length + 1
  const itemsOnPage = companies.length
  const itemsBeforePage = cursorHistory.length * limit

  const hasActiveFilters =
    search !== '' ||
    adapters.length > 0 ||
    activeFilter !== ACTIVE_FILTER.ALL ||
    scanStatus !== SCAN_STATUS_FILTER.ALL ||
    sorting.length > 0

  function goToNextPage() {
    if (!nextCursor) return
    setCursorHistory((h) => [...h, cursor])
    setCursor(nextCursor)
  }

  function goToPrevPage() {
    setCursorHistory((h) => {
      const prev = h[h.length - 1] ?? ''
      setCursor(prev)
      return h.slice(0, -1)
    })
  }

  function handleSortingChange(newSorting: SortingState) {
    setSorting(newSorting)
    setCursor('')
    setCursorHistory([])
  }

  function resetFilters() {
    setSearch('')
    setAdapters([])
    setActiveFilter(ACTIVE_FILTER.ALL)
    setScanStatus(SCAN_STATUS_FILTER.ALL)
    setSorting([])
    setCursor('')
    setCursorHistory([])
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

    limit,
    setLimit,
    nextCursor,
    hasPrev: cursorHistory.length > 0,
    goToNextPage,
    goToPrevPage,
    page,
    itemsOnPage,
    itemsBeforePage,

    hasActiveFilters,
    resetFilters,

    autoRefresh,
    toggleAutoRefresh,
  }
}
