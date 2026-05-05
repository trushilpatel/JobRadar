import type { Identity } from '@/types'
import { apiClient } from '@/lib/api-client'

export async function fetchMe(): Promise<Identity> {
  return apiClient.get<Identity>('/me')
}

export async function updateMe(displayName: string): Promise<void> {
  await apiClient.patch('/me', { display_name: displayName })
}

export async function deleteMe(): Promise<void> {
  await apiClient.delete('/me')
}
