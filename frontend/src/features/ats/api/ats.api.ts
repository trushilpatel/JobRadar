import { z } from 'zod'
import { apiClient } from '@/lib/core/api-client'
import type { ATS } from '@/types'

export const ATSSchema = z.object({
  name: z.string(),
  display_name: z.string(),
  base_url: z.string(),
  is_active: z.boolean(),
  is_admin_creatable: z.boolean(),
  scan_interval_min: z.number(),
  inter_company_delay_s: z.number(),
})

export async function getAts(): Promise<ATS[]> {
  const raw = await apiClient.get<unknown>('/admin/ats')
  if (raw == null) return []
  return z.array(ATSSchema).parse(raw)
}

export async function updateAts(
  name: string,
  data: Partial<
    Pick<ATS, 'scan_interval_min' | 'inter_company_delay_s' | 'is_active' | 'is_admin_creatable'>
  >
): Promise<ATS> {
  const raw = await apiClient.patch<unknown>('/admin/ats/' + name, data)
  return ATSSchema.parse(raw)
}
