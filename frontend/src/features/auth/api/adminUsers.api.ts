import { z } from 'zod'
import { apiClient } from '@/lib/core/api-client'
import type { components } from '@/lib/api/schema.gen'

const AdminUserSchema = z.object({
  id: z.string(),
  email: z.string(),
  display_name: z.string().nullable(),
  status: z.enum(['active', 'suspended']),
  created_at: z.string(),
  roles: z.array(z.string()),
})

const AdminUserListResultSchema = z.object({
  data: z.array(AdminUserSchema),
  total: z.number(),
  page: z.number(),
  limit: z.number(),
})

export type AdminUser = z.infer<typeof AdminUserSchema>
export type AdminUserListResult = z.infer<typeof AdminUserListResultSchema>

export interface AdminUserFilter {
  status?: 'active' | 'suspended' | ''
  email?: string
}

export async function fetchAdminUsers(
  filter: AdminUserFilter,
  page = 1,
  limit = 25
): Promise<AdminUserListResult> {
  const params = new URLSearchParams()
  params.set('page', String(page))
  params.set('limit', String(limit))
  if (filter.status) params.set('status', filter.status)
  if (filter.email) params.set('email', filter.email)
  const raw = await apiClient.get<unknown>(`/admin/users?${params}`)
  return AdminUserListResultSchema.parse(raw)
}

export async function setUserRoles(userId: string, roleIds: string[]): Promise<void> {
  // `satisfies` cross-checks this request body against the backend's
  // generated OpenAPI contract at compile time — a renamed/removed field on
  // either side fails the build instead of silently drifting.
  const body = { role_ids: roleIds } satisfies components['schemas']['SetRolesInputBody']
  await apiClient.patch(`/admin/users/${userId}/roles`, body)
}

export async function setUserStatus(userId: string, status: 'active' | 'suspended'): Promise<void> {
  const body = { status } satisfies components['schemas']['SetStatusInputBody']
  await apiClient.patch(`/admin/users/${userId}/status`, body)
}

export async function deleteAdminUser(userId: string): Promise<void> {
  await apiClient.delete(`/admin/users/${userId}`)
}
