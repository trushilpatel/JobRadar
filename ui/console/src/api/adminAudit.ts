import { apiClient } from '@/lib/api-client'

export interface AuditEntry {
  id: string
  actor_user_id: string | null
  action: string
  target_type: string | null
  target_id: string | null
  payload: unknown
  created_at: string
}

export interface AuditListResult {
  data: AuditEntry[]
  total: number
  page: number
  limit: number
}

export interface AuditFilter {
  actor_user_id?: string
  action?: string
}

export async function fetchAuditLog(
  filter: AuditFilter,
  page = 1,
  limit = 50
): Promise<AuditListResult> {
  const params = new URLSearchParams()
  params.set('page', String(page))
  params.set('limit', String(limit))
  if (filter.actor_user_id) params.set('actor_user_id', filter.actor_user_id)
  if (filter.action) params.set('action', filter.action)
  return apiClient.get<AuditListResult>(`/admin/audit-log?${params}`)
}
