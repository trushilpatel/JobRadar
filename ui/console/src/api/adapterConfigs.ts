import type { AdapterConfig, Company, EffectiveScanConfig } from '@/types'
import { apiClient } from '@/lib/api-client'

export type { AdapterConfig, EffectiveScanConfig }

export interface UpdateAdapterConfigInput {
  scan_interval_minutes: number
  inter_company_delay_s: number
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
