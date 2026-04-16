import { useEffect, useMemo, useRef, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { fetchCompanies, fetchQueue, type CompanyFilters } from '@/api/companies'

export type ActiveFilter = 'all' | 'active' | 'inactive'
export type StatusFilter = 'none' | 'error' | 'running' | 'queued'

export function useCompanies() {
  const [search, setSearch] = useState('')
  const [debouncedSearch, setDebouncedSearch] = useState('')
  const [adapters, setAdapters] = useState<string[]>([])
  const [activeFilter, setActiveFilter] = useState<ActiveFilter>('all')
  const [statusFilter, setStatusFilter] = useState<StatusFilter>('none')

  const [page, setPage] = useState(1)
  const [limit, setLimit] = useState(20)

  const debounceRef = useRef<ReturnType<typeof setTimeout> | null>(null)
  function handleSearchChange(v: string) {
    setSearch(v)
    if (debounceRef.current) clearTimeout(debounceRef.current)
    debounceRef.current = setTimeout(() => setDebouncedSearch(v), 300)
  }

  // Server-side filters — only those the API supports.
  // statusFilter + multi-adapter are applied client-side on the current page.
  const serverFilters = useMemo<CompanyFilters>(
    () => ({
      search: debouncedSearch || undefined,
      adapterType: adapters.length === 1 ? adapters[0] : undefined,
      active: activeFilter === 'active' ? true : activeFilter === 'inactive' ? false : undefined,
    }),
    [debouncedSearch, adapters, activeFilter]
  )

  useEffect(() => {
    setPage(1)
  }, [serverFilters, statusFilter])

  const companiesQuery = useQuery({
    queryKey: ['companies', serverFilters, page, limit],
    queryFn: () => fetchCompanies(serverFilters, page, limit),
    refetchInterval: 30_000,
  })

  const queueQuery = useQuery({
    queryKey: ['companies', 'queue'],
    queryFn: fetchQueue,
    refetchInterval: 15_000,
  })

  const queueMap = useMemo(() => {
    const m = new Map<string, 'queued' | 'processing'>()
    for (const e of queueQuery.data ?? []) m.set(e.company_id, e.status)
    return m
  }, [queueQuery.data])

  const rawCompanies = companiesQuery.data?.data ?? []
  const total = companiesQuery.data?.total ?? 0
  const totalActive = companiesQuery.data?.total_active ?? 0

  // Client-side refinement
  const companies = useMemo(() => {
    let list = rawCompanies
    if (adapters.length > 1) {
      list = list.filter((c) => adapters.includes(c.adapter_type))
    }
    if (statusFilter !== 'none') {
      list = list.filter((c) => {
        const q = queueMap.get(c.id)
        if (statusFilter === 'running') return q === 'processing'
        if (statusFilter === 'queued') return q === 'queued'
        if (statusFilter === 'error') return c.last_scan_status === 'error'
        return true
      })
    }
    return list
  }, [rawCompanies, adapters, statusFilter, queueMap])

  // Counts for filter bar / KPI strip.
  // running/queued are derived from queueMap cross-referenced with rawCompanies so
  // the counts stay consistent with what the status filter will actually show on
  // the current page. Using queueMap.values() globally would show counts that don't
  // match the page, making the filter appear broken when the matching company is
  // on a different page.
  const counts = useMemo(() => {
    const errors = rawCompanies.filter((c) => c.last_scan_status === 'error').length
    const running = rawCompanies.filter((c) => queueMap.get(c.id) === 'processing').length
    const queued = rawCompanies.filter((c) => queueMap.get(c.id) === 'queued').length
    return { errors, running, queued }
  }, [rawCompanies, queueMap])

  function resetFilters() {
    setSearch('')
    setDebouncedSearch('')
    setAdapters([])
    setActiveFilter('all')
    setStatusFilter('none')
    setPage(1)
  }

  return {
    companies,
    total,
    totalActive,
    counts,
    queueMap,
    loading: companiesQuery.isLoading,
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

    page,
    setPage,
    limit,
    setLimit,

    resetFilters,
  }
}
