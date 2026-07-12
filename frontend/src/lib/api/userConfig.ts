import { z } from 'zod'
import { apiClient } from '@/lib/core/api-client'

// theme is not part of the backend's UserConfig contract (see
// components['schemas']['UserConfig'] in src/lib/api/schema.gen.ts, which
// has only show_keyboard_shortcuts) — cross-checking against the generated
// schema surfaced this as client-only state that happens to share this
// schema's shape, not a synced backend field. Left as-is (out of scope for
// the type-generation rollout); Go's JSON decode ignores unknown fields, so
// sending it in a PATCH is harmless but never persisted server-side.
const UserConfigSchema = z.object({
  show_keyboard_shortcuts: z.boolean().default(false),
  theme: z.enum(['dark', 'light', 'system']).optional(),
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
