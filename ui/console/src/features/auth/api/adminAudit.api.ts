import { z } from 'zod'
import { apiClient } from '@/lib/core/api-client'

const AuditEntrySchema = z.object({
  id: z.string(),
  actor_user_id: z.string().nullable(),
  actor_email: z.string().nullable(),
  action: z.string(),
  target_type: z.string().nullable(),
  target_id: z.string().nullable(),
  payload: z.unknown(),
  created_at: z.string(),
})

const AuditListResultSchema = z.object({
  data: z.array(AuditEntrySchema),
  total: z.number(),
  page: z.number(),
  limit: z.number(),
})

export type AuditEntry = z.infer<typeof AuditEntrySchema>
export type AuditListResult = z.infer<typeof AuditListResultSchema>

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
  const raw = await apiClient.get<unknown>(`/admin/audit-log?${params}`)
  return AuditListResultSchema.parse(raw)
}
