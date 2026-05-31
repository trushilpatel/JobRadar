import { toast } from 'sonner'
import { useQueryClient } from '@tanstack/react-query'
import { updateAdapterConfig } from '../api/adapterConfigs.api'
import type { AdapterConfig, UpdateAdapterConfigInput } from '../api/adapterConfigs.api'
import { settingsQueryKeys } from '../queryKeys'

export function useAdapterConfigActions() {
  const qc = useQueryClient()

  async function updateConfig(adapterType: string, data: UpdateAdapterConfigInput): Promise<void> {
    const previous = qc.getQueryData<AdapterConfig[]>(settingsQueryKeys.adapterConfigs())

    // Optimistic update
    qc.setQueryData<AdapterConfig[]>(
      settingsQueryKeys.adapterConfigs(),
      (old) =>
        old?.map((cfg) => (cfg.adapter_type === adapterType ? { ...cfg, ...data } : cfg)) ?? []
    )

    try {
      const updated = await updateAdapterConfig(adapterType, data)
      // Replace with server response
      qc.setQueryData<AdapterConfig[]>(
        settingsQueryKeys.adapterConfigs(),
        (old) => old?.map((cfg) => (cfg.adapter_type === adapterType ? updated : cfg)) ?? []
      )
      toast.success(`${adapterType} config updated`)
    } catch (err) {
      // Rollback
      if (previous !== undefined) {
        qc.setQueryData<AdapterConfig[]>(settingsQueryKeys.adapterConfigs(), previous)
      }
      toast.error(err instanceof Error ? err.message : 'Failed to update config')
      throw err
    }
  }

  return { updateConfig }
}
