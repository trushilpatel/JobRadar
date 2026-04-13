import type { Company, ScanStats, QueueEntry } from '@/types'

const BASE = '/api'

export async function fetchCompanies(): Promise<Company[]> {
  const res = await fetch(`${BASE}/companies`)
  if (!res.ok) throw new Error('Failed to fetch companies')
  return res.json()
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
    const err = await res.json().catch(() => ({}))
    throw new Error(err.error ?? 'Failed to create company')
  }
  return res.json()
}

export async function updateCompany(
  id: string,
  fields: Partial<Pick<Company, 'name' | 'url' | 'adapter_type' | 'active'>>
): Promise<void> {
  const res = await fetch(`${BASE}/companies/${id}`, {
    method: 'PATCH',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(fields),
  })
  if (!res.ok) throw new Error('Failed to update company')
}

export async function deleteCompany(id: string): Promise<void> {
  const res = await fetch(`${BASE}/companies/${id}`, { method: 'DELETE' })
  if (!res.ok) throw new Error('Failed to delete company')
}

export async function fetchScanStats(id: string): Promise<ScanStats> {
  const res = await fetch(`${BASE}/companies/${id}/scan-stats`)
  if (!res.ok) throw new Error('Failed to fetch scan stats')
  return res.json()
}

export async function triggerScan(id: string): Promise<void> {
  const res = await fetch(`${BASE}/companies/${id}/scan`, { method: 'POST' })
  if (!res.ok) {
    const body = await res.json().catch(() => ({}))
    throw new Error(body.error ?? 'Failed to trigger scan')
  }
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
  return res.json()
}

export async function removeFromQueue(entryId: string): Promise<void> {
  const res = await fetch(`${BASE}/companies/queue/${entryId}`, { method: 'DELETE' })
  if (!res.ok) throw new Error('Failed to remove queue entry')
}

export async function moveQueueItemUp(entryId: string): Promise<void> {
  const res = await fetch(`${BASE}/companies/queue/${entryId}/move-up`, { method: 'POST' })
  if (!res.ok) throw new Error('Failed to move queue entry')
}

export async function moveQueueItemDown(entryId: string): Promise<void> {
  const res = await fetch(`${BASE}/companies/queue/${entryId}/move-down`, { method: 'POST' })
  if (!res.ok) throw new Error('Failed to move queue entry')
}
