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
  website: string
  created_at: string
  scan_settings: ScanSettingsView | null
  queue_status?: string
  scan_history?: boolean[]
}

export interface AdminCompanyListResult {
  data: AdminCompany[]
  total: number
  total_active: number
  limit: number
  next_cursor: string | null
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
  scanStatus?: string
}

export type AdapterTypeValue = (typeof ADAPTER_TYPES)[number]

export async function fetchAdminCompanies(
  filters: AdminCompanyFilters = {},
  cursor = '',
  limit = 50
): Promise<AdminCompanyListResult> {
  const p = new URLSearchParams()
  if (cursor) p.set('cursor', cursor)
  p.set('limit', String(limit))
  if (filters.search) p.set('search', filters.search)
  if (filters.has_settings !== undefined) p.set('has_settings', String(filters.has_settings))
  if (filters.active !== undefined) p.set('active', String(filters.active))
  filters.adapterTypes?.forEach((t) => p.append('adapter_types', t))
  if (filters.sortBy) p.set('sort_by', filters.sortBy)
  if (filters.sortDir) p.set('sort_dir', filters.sortDir)
  if (filters.scanStatus) p.set('scan_status', filters.scanStatus)
  return apiClient.get<AdminCompanyListResult>(`/admin/companies?${p}`)
}

export async function createAdminCompany(data: {
  id: string
  name: string
  website: string
  scan_settings?: { adapter_type: string; active: boolean }
}): Promise<AdminCompany> {
  // Backend requires slug on create; id and slug are 1:1 for admin-created
  // companies (the id IS the slug in `companies`), so default it from id
  // unless an explicit slug was supplied via .slug — keeps the modal payload
  // contract minimal while satisfying CompanyPublicInput validation.
  const payload = { slug: data.id, ...data }
  return apiClient.post<AdminCompany>('/admin/companies', payload)
}

export async function updateAdminCompany(
  id: string,
  fields: Partial<Pick<AdminCompany, 'name' | 'website'>>
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
