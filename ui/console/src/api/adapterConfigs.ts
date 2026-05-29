import { z } from 'zod'
import type { Company } from '@/types'
import { apiClient } from '@/lib/api-client'

const AdapterConfigSchema = z.object({
  adapter_type: z.string(),
  scan_interval_minutes: z.number(),
  inter_company_delay_s: z.number(),
  last_scheduled_at: z.string().nullable(),
  updated_at: z.string(),
})

const EffectiveScanConfigSchema = z.object({
  adapter_type: z.string(),
  effective_scan_interval_minutes: z.number(),
  inter_company_delay_s: z.number(),
  company_override: z.number().nullable(),
})

const CompanySchema = z
  .object({
    id: z.string(),
    name: z.string(),
    url: z.string(),
    adapter_type: z.string(),
    active: z.boolean(),
    created_at: z.string(),
    last_scan_at: z.string().nullable(),
    last_scan_status: z.enum(['ok', 'error', 'pending', 'running', 'queued']).nullable(),
    queue_status: z.string().optional(),
    scan_history: z.array(z.boolean()).optional(),
  })
  .passthrough()

export type AdapterConfig = z.infer<typeof AdapterConfigSchema>
export type EffectiveScanConfig = z.infer<typeof EffectiveScanConfigSchema>

export interface UpdateAdapterConfigInput {
  scan_interval_minutes: number
  inter_company_delay_s: number
}

export async function getAdapterConfigs(): Promise<AdapterConfig[]> {
  const raw = await apiClient.get<unknown>('/admin/adapter-configs')
  if (raw == null) return []
  return z.array(AdapterConfigSchema).parse(raw)
}

export async function updateAdapterConfig(
  adapterType: string,
  data: UpdateAdapterConfigInput
): Promise<AdapterConfig> {
  const raw = await apiClient.patch<unknown>(
    `/admin/adapter-configs/${encodeURIComponent(adapterType)}`,
    data
  )
  return AdapterConfigSchema.parse(raw)
}

export async function updateScanIntervalOverride(
  companyId: string,
  override: number | null
): Promise<Company> {
  const raw = await apiClient.patch<unknown>(
    `/admin/companies/${encodeURIComponent(companyId)}/scan-interval-override`,
    { scan_interval_minutes_override: override }
  )
  return CompanySchema.parse(raw) as Company
}

export async function getCompanyScanConfig(companyId: string): Promise<EffectiveScanConfig> {
  const raw = await apiClient.get<unknown>(
    `/admin/companies/${encodeURIComponent(companyId)}/scan-config`
  )
  return EffectiveScanConfigSchema.parse(raw)
}
