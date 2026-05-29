import { toast } from 'sonner'
import { useQueryClient } from '@tanstack/react-query'
import { updateAdapterConfig } from '../api/adapterConfigs.api'
import type { AdapterConfig, UpdateAdapterConfigInput } from '../api/adapterConfigs.api'

export function useAdapterConfigActions() {
  const qc = useQueryClient()

  async function updateConfig(adapterType: string, data: UpdateAdapterConfigInput): Promise<void> {
    const previous = qc.getQueryData<AdapterConfig[]>(['adapter-configs'])

    // Optimistic update
    qc.setQueryData<AdapterConfig[]>(
      ['adapter-configs'],
      (old) =>
        old?.map((cfg) => (cfg.adapter_type === adapterType ? { ...cfg, ...data } : cfg)) ?? []
    )

    try {
      const updated = await updateAdapterConfig(adapterType, data)
      // Replace with server response
      qc.setQueryData<AdapterConfig[]>(
        ['adapter-configs'],
        (old) => old?.map((cfg) => (cfg.adapter_type === adapterType ? updated : cfg)) ?? []
      )
      toast.success(`${adapterType} config updated`)
    } catch (err) {
      // Rollback
      if (previous !== undefined) {
        qc.setQueryData<AdapterConfig[]>(['adapter-configs'], previous)
      }
      toast.error(err instanceof Error ? err.message : 'Failed to update config')
      throw err
    }
  }

  return { updateConfig }
}
