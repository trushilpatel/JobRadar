import { useQuery } from '@tanstack/react-query'
import { getAdapterConfigs } from '@/api/adapterConfigs'
import type { AdapterConfig } from '@/api/adapterConfigs'

export function useAdapterConfigs() {
  return useQuery<AdapterConfig[]>({
    queryKey: ['adapter-configs'],
    queryFn: getAdapterConfigs,
    refetchInterval: 60_000,
  })
}
