import { z } from 'zod'
import { apiClient } from '@/lib/api-client'

const IdentitySchema = z.object({
  user_id: z.string(),
  email: z.string(),
  display_name: z.string(),
  status: z.enum(['active', 'suspended']),
  roles: z.array(z.string()),
})

export type Identity = z.infer<typeof IdentitySchema>

export async function fetchMe(): Promise<Identity> {
  const raw = await apiClient.get<unknown>('/me')
  return IdentitySchema.parse(raw)
}

export async function updateMe(displayName: string): Promise<void> {
  await apiClient.patch('/me', { display_name: displayName })
}

export async function deleteMe(): Promise<void> {
  await apiClient.delete('/me')
}
