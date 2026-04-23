import type { Company, ScanStats, QueueEntry } from '@/types'
import { apiClient } from '@/lib/api-client'

export interface CompanyFilters {
  active?: boolean // true = active only, false = inactive only, undefined = all
  adapterTypes?: string[] // e.g. ['greenhouse', 'lever']
  search?: string // ILIKE name match
  lastScanStatus?: string // e.g. 'error'
  sortBy?: string
  sortDir?: 'asc' | 'desc'
}

export interface CompaniesPage {
  data: Company[]
  total: number // total matching filter
  total_active: number // always: total active companies (for header stats)
  page: number
  limit: number
}

export async function fetchCompanies(
  filters: CompanyFilters = {},
  page = 1,
  limit = 20
): Promise<CompaniesPage> {
  const p = new URLSearchParams()
  p.set('page', String(page))
  p.set('limit', String(limit))
  if (filters.active !== undefined) p.set('active', String(filters.active))
  filters.adapterTypes?.forEach((at) => p.append('adapter_types', at))
  if (filters.search) p.set('search', filters.search)
  if (filters.lastScanStatus) p.set('last_scan_status', filters.lastScanStatus)
  if (filters.sortBy) p.set('sort_by', filters.sortBy)
  if (filters.sortDir) p.set('sort_dir', filters.sortDir)

  return apiClient.get<CompaniesPage>(`/companies?${p}`)
}

export async function createCompany(data: {
  id: string
  name: string
  url: string
  adapter_type: string
  active: boolean
}): Promise<Company> {
  return apiClient.post<Company>('/companies', data)
}

export async function updateCompany(
  id: string,
  fields: Partial<Pick<Company, 'name' | 'url' | 'adapter_type' | 'active'>>
): Promise<void> {
  await apiClient.patch<void>(`/companies/${encodeURIComponent(id)}`, fields)
}

export async function deleteCompany(id: string): Promise<void> {
  await apiClient.delete<void>(`/companies/${encodeURIComponent(id)}`)
}

export async function fetchScanStats(id: string): Promise<ScanStats> {
  return apiClient.get<ScanStats>(`/companies/${encodeURIComponent(id)}/scan-stats`)
}

export async function triggerScan(id: string): Promise<void> {
  await apiClient.post<void>(`/companies/${encodeURIComponent(id)}/scan`)
}

export async function triggerScanAll(): Promise<{ queued: number }> {
  return apiClient.post<{ queued: number }>('/companies/scan-all')
}

export async function setAllCompaniesActive(active: boolean): Promise<void> {
  await apiClient.patch<void>('/companies/active', { active })
}

export async function fetchQueue(): Promise<QueueEntry[]> {
  return apiClient.get<QueueEntry[]>('/companies/queue')
}

export async function removeFromQueue(entryId: string): Promise<void> {
  await apiClient.delete<void>(`/companies/queue/${entryId}`)
}
