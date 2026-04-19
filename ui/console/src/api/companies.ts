import type { Company, ScanStats, QueueEntry } from '@/types'
import { parseApiError } from '@/lib/api-error'

const BASE = '/api'

export interface CompanyFilters {
  active?: boolean // true = active only, false = inactive only, undefined = all
  adapterType?: string // e.g. 'greenhouse', 'lever'
  search?: string // ILIKE name match
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
  if (filters.adapterType) p.set('adapter_type', filters.adapterType)
  if (filters.search) p.set('search', filters.search)

  const res = await fetch(`${BASE}/companies?${p}`)
  if (!res.ok) throw new Error('Failed to fetch companies')
  return res.json().then((r: { data: CompaniesPage }) => r.data)
}

export async function createCompany(data: {
  id: string
  name: string
  url: string
  adapter_type: string
  active: boolean
}): Promise<Company> {
  const res = await fetch(`${BASE}/companies`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(data),
  })
  if (!res.ok) {
    throw await parseApiError(res, 'Failed to create company')
  }
  return res.json().then((r: { data: Company }) => r.data)
}

export async function updateCompany(
  id: string,
  fields: Partial<Pick<Company, 'name' | 'url' | 'adapter_type' | 'active'>>
): Promise<void> {
  const res = await fetch(`${BASE}/companies/${encodeURIComponent(id)}`, {
    method: 'PATCH',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(fields),
  })
  if (!res.ok) {
    throw await parseApiError(res, 'Failed to update company')
  }
}

export async function deleteCompany(id: string): Promise<void> {
  const res = await fetch(`${BASE}/companies/${encodeURIComponent(id)}`, { method: 'DELETE' })
  if (!res.ok) throw new Error('Failed to delete company')
}

export async function fetchScanStats(id: string): Promise<ScanStats> {
  const res = await fetch(`${BASE}/companies/${encodeURIComponent(id)}/scan-stats`)
  if (!res.ok) throw new Error('Failed to fetch scan stats')
  return res.json().then((r: { data: ScanStats }) => r.data)
}

export async function triggerScan(id: string): Promise<void> {
  const res = await fetch(`${BASE}/companies/${encodeURIComponent(id)}/scan`, { method: 'POST' })
  if (!res.ok) {
    const body = await res.json().catch(() => ({}))
    throw new Error(body.error?.message ?? 'Failed to trigger scan')
  }
}

export async function triggerScanAll(): Promise<{ queued: number }> {
  const res = await fetch(`${BASE}/companies/scan-all`, { method: 'POST' })
  if (!res.ok) throw new Error('Failed to trigger scan all')
  return res.json().then((r: { data: { queued: number } }) => r.data)
}

export async function setAllCompaniesActive(active: boolean): Promise<void> {
  const res = await fetch(`${BASE}/companies/active`, {
    method: 'PATCH',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ active }),
  })
  if (!res.ok) throw new Error('Failed to update companies')
}

export async function fetchQueue(): Promise<QueueEntry[]> {
  const res = await fetch(`${BASE}/companies/queue`)
  if (!res.ok) throw new Error('Failed to fetch queue')
  return res.json().then((r: { data: QueueEntry[] }) => r.data)
}

export async function removeFromQueue(entryId: string): Promise<void> {
  const res = await fetch(`${BASE}/companies/queue/${entryId}`, { method: 'DELETE' })
  if (!res.ok) throw new Error('Failed to remove queue entry')
}
