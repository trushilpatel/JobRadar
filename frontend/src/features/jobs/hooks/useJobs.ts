import { useState, useMemo, useEffect } from 'react'
import { useQuery } from '@tanstack/react-query'
import { fetchJobs, type JobFilters } from '@/api/jobs'

export type TabKey = 'new' | 'saved' | 'applied'
export type DateFilter = 'all' | '7d' | '30d'

export function useJobs() {
  // Tab
  const [tab, setTab] = useState<TabKey>('new')

  // Filter state — mirrors existing defaults in JobsPage
  const [selectedCompanies, setSelectedCompanies] = useState<string[]>([])
  const [remoteFilter, setRemoteFilter]               = useState(false)
  const [employTypes, setEmployTypes]                 = useState<string[]>(['full_time'])
  const [sponsorshipFilter, setSponsorshipFilter]     = useState(true)
  const [noSponsorshipFilter, setNoSponsorshipFilter] = useState(false)
  const [noRequirementsFilter, setNoRequirementsFilter] = useState(true)
  const [citizenshipReqFilter, setCitizenshipReqFilter] = useState(false)
  const [expFilter, setExpFilter]                     = useState<string[]>(['0-2', '3-5'])
  const [dateFilter, setDateFilter]                   = useState<DateFilter>('all')

  // Pagination
  const [page, setPage]   = useState(1)
  const [limit, setLimit] = useState(20)

  // Stable filter object — only changes when a filter value changes
  const filters = useMemo<JobFilters>(() => ({
    status: tab,
    companies: selectedCompanies.length > 0 ? selectedCompanies : undefined,
    remote: remoteFilter || undefined,
    employTypes: employTypes.length > 0 ? employTypes : undefined,
    sponsorship: sponsorshipFilter ? true : noSponsorshipFilter ? false : undefined,
    noRequirements: noRequirementsFilter || undefined,
    requiresCitizen: citizenshipReqFilter ? true : undefined,
    experienceRanges: expFilter.length > 0 ? expFilter : undefined,
    daysAdded: dateFilter === '7d' ? 7 : dateFilter === '30d' ? 30 : undefined,
  }), [
    tab, selectedCompanies, remoteFilter, employTypes,
    sponsorshipFilter, noSponsorshipFilter, noRequirementsFilter,
    citizenshipReqFilter, expFilter, dateFilter,
  ])

  // Reset to page 1 when filters change
  useEffect(() => { setPage(1) }, [filters])

  const query = useQuery({
    queryKey: ['jobs', filters, page, limit],
    queryFn: () => fetchJobs(filters, page, limit),
  })

  const jobs  = query.data?.data  ?? []
  const total = query.data?.total ?? 0

  // Companies for the multi-select dropdown — derived from current page results
  const companies = useMemo(() => {
    const names = Array.from(
      new Set(jobs.map(j => j.company?.name ?? j.company_id).filter(Boolean))
    )
    return (names as string[]).sort()
  }, [jobs])

  function resetFilters() {
    setSelectedCompanies([])
    setRemoteFilter(false)
    setSponsorshipFilter(true)
    setNoSponsorshipFilter(false)
    setNoRequirementsFilter(true)
    setCitizenshipReqFilter(false)
    setExpFilter(['0-2', '3-5'])
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

    // Tab
    tab, setTab,

    // Filters
    selectedCompanies, setSelectedCompanies,
    remoteFilter,      setRemoteFilter,
    employTypes,       setEmployTypes,
    sponsorshipFilter,    setSponsorshipFilter,
    noSponsorshipFilter,  setNoSponsorshipFilter,
    noRequirementsFilter, setNoRequirementsFilter,
    citizenshipReqFilter, setCitizenshipReqFilter,
    expFilter,  setExpFilter,
    dateFilter, setDateFilter,
    resetFilters,

    // Pagination
    page, setPage,
    limit, setLimit,
  }
}
