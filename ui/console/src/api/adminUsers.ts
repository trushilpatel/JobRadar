import { apiClient } from '@/lib/api-client'

export interface AdminUser {
  id: string
  email: string
  display_name: string | null
  status: 'active' | 'suspended'
  created_at: string
  roles: string[]
}

export interface AdminUserListResult {
  data: AdminUser[]
  total: number
  page: number
  limit: number
}

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
  return apiClient.get<AdminUserListResult>(`/admin/users?${params}`)
}

export async function setUserRoles(userId: string, roleIds: string[]): Promise<void> {
  await apiClient.patch(`/admin/users/${userId}/roles`, { role_ids: roleIds })
}

export async function setUserStatus(userId: string, status: 'active' | 'suspended'): Promise<void> {
  await apiClient.patch(`/admin/users/${userId}/status`, { status })
}

export async function deleteAdminUser(userId: string): Promise<void> {
  await apiClient.delete(`/admin/users/${userId}`)
}
