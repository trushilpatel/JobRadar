import { apiClient } from '@/lib/api-client'
import type { ADAPTER_TYPES } from '@/types'

export interface ScanSettingsView {
  company_id: string
  adapter_type: string
  adapter_config: Record<string, unknown> | null
  scan_interval_override: number | null
  active: boolean
  last_scan_at: string | null
  last_success_at: string | null
  last_error_at: string | null
  last_error_message: string | null
  consecutive_errors: number
  created_at: string
  updated_at: string
}

export interface AdminCompany {
  id: string
  name: string
  url: string
  created_at: string
  scan_settings: ScanSettingsView | null
}

export interface AdminCompanyListResult {
  data: AdminCompany[]
  total: number
  total_active: number
  page: number
  limit: number
}

export interface ScanSettingsInput {
  adapter_type?: string
  adapter_config?: Record<string, unknown> | null
  scan_interval_override?: number | null
  active?: boolean
}

export interface AdminCompanyFilters {
  search?: string
  has_settings?: boolean
  active?: boolean
  adapterTypes?: string[]
  sortBy?: string
  sortDir?: 'asc' | 'desc'
}

export type AdapterTypeValue = (typeof ADAPTER_TYPES)[number]

export async function fetchAdminCompanies(
  filters: AdminCompanyFilters = {},
  page = 1,
  limit = 50
): Promise<AdminCompanyListResult> {
  const p = new URLSearchParams()
  p.set('page', String(page))
  p.set('limit', String(limit))
  if (filters.search) p.set('search', filters.search)
  if (filters.has_settings !== undefined) p.set('has_settings', String(filters.has_settings))
  if (filters.active !== undefined) p.set('active', String(filters.active))
  filters.adapterTypes?.forEach((t) => p.append('adapter_types', t))
  if (filters.sortBy) p.set('sort_by', filters.sortBy)
  if (filters.sortDir) p.set('sort_dir', filters.sortDir)
  return apiClient.get<AdminCompanyListResult>(`/admin/companies?${p}`)
}

export async function createAdminCompany(data: {
  id: string
  name: string
  url: string
  scan_settings?: { adapter_type: string; active: boolean }
}): Promise<AdminCompany> {
  return apiClient.post<AdminCompany>('/admin/companies', data)
}

export async function updateAdminCompany(
  id: string,
  fields: Partial<Pick<AdminCompany, 'name' | 'url'>>
): Promise<void> {
  await apiClient.patch<void>(`/admin/companies/${encodeURIComponent(id)}`, fields)
}

export async function deleteAdminCompany(id: string): Promise<void> {
  await apiClient.delete<void>(`/admin/companies/${encodeURIComponent(id)}`)
}

export async function updateAdminCompanyScanSettings(
  id: string,
  input: ScanSettingsInput
): Promise<void> {
  await apiClient.patch<void>(`/admin/companies/${encodeURIComponent(id)}/scan-settings`, input)
}

export async function setAllAdminCompaniesActive(active: boolean): Promise<void> {
  await apiClient.patch<void>('/admin/companies/active', { active })
}
