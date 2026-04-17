import { useState, useMemo, useEffect } from 'react'
import { useQuery } from '@tanstack/react-query'
import { fetchJobs, fetchJobCounts, type JobFilters } from '@/api/jobs'

export type TabKey = 'new' | 'saved' | 'applied' | 'interviewing' | 'offer'
export type DateFilter = 'all' | '7d' | '30d'

export function useJobs() {
  // Tab
  const [tab, setTab] = useState<TabKey>('new')

  // Filter state
  const [selectedCompanies, setSelectedCompanies] = useState<string[]>([])
  const [remoteFilter, setRemoteFilter] = useState(false)
  const [onSiteFilter, setOnSiteFilter] = useState(false)
  const [expFilter, setExpFilter] = useState<string[]>([])
  const [dateFilter, setDateFilter] = useState<DateFilter>('all')

  // Search
  const [searchQuery, setSearchQuery] = useState('')

  // Pagination
  const [page, setPage] = useState(1)
  const [limit, setLimit] = useState(20)

  // Stable filter object — only changes when a filter value changes
  const filters = useMemo<JobFilters>(
    () => ({
      status: tab,
      companies: selectedCompanies.length > 0 ? selectedCompanies : undefined,
      remote: remoteFilter || undefined,
      onSite: onSiteFilter || undefined,
      experienceRanges: expFilter.length > 0 ? expFilter : undefined,
      daysAdded: dateFilter === '7d' ? 7 : dateFilter === '30d' ? 30 : undefined,
    }),
    [tab, selectedCompanies, remoteFilter, onSiteFilter, expFilter, dateFilter]
  )

  // Reset to page 1 when filters change
  useEffect(() => {
    // eslint-disable-next-line react-hooks/set-state-in-effect
    setPage(1)
  }, [filters])

  const query = useQuery({
    queryKey: ['jobs', filters, page, limit],
    queryFn: () => fetchJobs(filters, page, limit),
  })

  const countsQuery = useQuery({
    queryKey: ['jobs', 'counts'],
    queryFn: fetchJobCounts,
  })

  const jobs = query.data?.data ?? []
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

  // Companies for the multi-select dropdown — derived from current page results
  const companies = useMemo(() => {
    const names = Array.from(
      new Set(jobs.map((j) => j.company?.name ?? j.company_id).filter(Boolean))
    )
    return (names as string[]).sort()
  }, [jobs])

  function resetFilters() {
    setSearchQuery('')
    setSelectedCompanies([])
    setRemoteFilter(false)
    setOnSiteFilter(false)
    setExpFilter([])
    setDateFilter('all')
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

    // Search
    searchQuery,
    setSearchQuery,

    // Filters
    selectedCompanies,
    setSelectedCompanies,
    remoteFilter,
    setRemoteFilter,
    onSiteFilter,
    setOnSiteFilter,
    expFilter,
    setExpFilter,
    dateFilter,
    setDateFilter,
    resetFilters,

    // Pagination
    page,
    setPage,
    limit,
    setLimit,
  }
}
