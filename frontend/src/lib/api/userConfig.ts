import { z } from 'zod'
import { apiClient } from '@/lib/core/api-client'

const UserConfigSchema = z.object({
  show_keyboard_shortcuts: z.boolean().default(false),
})

export type UserConfigResponse = z.infer<typeof UserConfigSchema>

export async function getUserConfig(): Promise<UserConfigResponse> {
  const raw = await apiClient.get<unknown>('/me/config')
  return UserConfigSchema.parse(raw)
}

export async function patchUserConfig(
  partial: Partial<UserConfigResponse>
): Promise<UserConfigResponse> {
  const raw = await apiClient.patch<unknown>('/me/config', partial)
  return UserConfigSchema.parse(raw)
}
