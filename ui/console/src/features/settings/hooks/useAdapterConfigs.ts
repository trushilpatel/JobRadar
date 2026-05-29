import { useQuery } from '@tanstack/react-query'
import { getAdapterConfigs } from '../api/adapterConfigs.api'
import type { AdapterConfig } from '../api/adapterConfigs.api'

export function useAdapterConfigs() {
  return useQuery<AdapterConfig[]>({
    queryKey: ['adapter-configs'],
    queryFn: getAdapterConfigs,
    refetchInterval: 60_000,
  })
}
