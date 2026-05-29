import { z } from 'zod'
import { apiClient } from '@/lib/api-client'
import type { ADAPTER_TYPES } from '@/types'

const ScanSettingsViewSchema = z
  .object({
    company_id: z.string(),
    adapter_type: z.string(),
    adapter_config: z.record(z.string(), z.unknown()).nullable(),
    scan_interval_override: z.number().nullable(),
    active: z.boolean(),
    last_scan_at: z.string().nullable(),
    last_success_at: z.string().nullable(),
    last_error_at: z.string().nullable(),
    last_error_message: z.string().nullable(),
    consecutive_errors: z.number(),
    created_at: z.string(),
    updated_at: z.string(),
  })
  .passthrough()

const AdminCompanySchema = z.object({
  id: z.string(),
  name: z.string(),
  website: z.string(),
  created_at: z.string(),
  scan_settings: ScanSettingsViewSchema.nullable(),
  queue_status: z.string().optional(),
  scan_history: z.array(z.boolean()).optional(),
})

const AdminCompanyListResultSchema = z.object({
  data: z.array(AdminCompanySchema),
  total: z.number(),
  total_active: z.number(),
  limit: z.number(),
  next_cursor: z.string().nullable(),
})

export type ScanSettingsView = z.infer<typeof ScanSettingsViewSchema>
export type AdminCompany = z.infer<typeof AdminCompanySchema>
export type AdminCompanyListResult = z.infer<typeof AdminCompanyListResultSchema>

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
  const raw = await apiClient.get<unknown>(`/admin/companies?${p}`)
  return AdminCompanyListResultSchema.parse(raw)
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
  const raw = await apiClient.post<unknown>('/admin/companies', payload)
  return AdminCompanySchema.parse(raw)
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
