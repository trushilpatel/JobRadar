import { useQuery } from '@tanstack/react-query'
import { getAdapterConfigs } from '../api/adapterConfigs.api'
import type { AdapterConfig } from '../api/adapterConfigs.api'
import { settingsQueryKeys } from '../queryKeys'

export function useAdapterConfigs() {
  return useQuery<AdapterConfig[]>({
    queryKey: settingsQueryKeys.adapterConfigs(),
    queryFn: getAdapterConfigs,
    refetchInterval: 60_000,
  })
}
