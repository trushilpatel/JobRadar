import type { Company } from '@/types'
import { apiClient } from '@/lib/api-client'

export interface AdapterConfig {
  adapter_type: string
  scan_interval_minutes: number
  inter_company_delay_s: number
  last_scheduled_at: string | null
  updated_at: string
}

export interface UpdateAdapterConfigInput {
  scan_interval_minutes: number
  inter_company_delay_s: number
}

export interface EffectiveScanConfig {
  adapter_type: string
  effective_scan_interval_minutes: number
  inter_company_delay_s: number
  company_override: number | null
}

export async function getAdapterConfigs(): Promise<AdapterConfig[]> {
  const result = await apiClient.get<AdapterConfig[] | null>('/admin/adapter-configs')
  return result ?? []
}

export async function updateAdapterConfig(
  adapterType: string,
  data: UpdateAdapterConfigInput
): Promise<AdapterConfig> {
  return apiClient.patch<AdapterConfig>(
    `/admin/adapter-configs/${encodeURIComponent(adapterType)}`,
    data
  )
}

export async function updateScanIntervalOverride(
  companyId: string,
  override: number | null
): Promise<Company> {
  return apiClient.patch<Company>(
    `/admin/companies/${encodeURIComponent(companyId)}/scan-interval-override`,
    { scan_interval_minutes_override: override }
  )
}

export async function getCompanyScanConfig(companyId: string): Promise<EffectiveScanConfig> {
  return apiClient.get<EffectiveScanConfig>(
    `/admin/companies/${encodeURIComponent(companyId)}/scan-config`
  )
}
