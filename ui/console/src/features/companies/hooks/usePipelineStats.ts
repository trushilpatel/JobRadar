import { useQuery } from '@tanstack/react-query'
import { fetchPipelineStats } from '@/features/dashboard/api/dashboard.api'
import { companyQueryKeys } from '@/features/companies/queryKeys'

export function usePipelineStats() {
  return useQuery({
    queryKey: companyQueryKeys.pipelineStats(),
    queryFn: fetchPipelineStats,
    refetchInterval: 30_000,
    staleTime: 30_000,
  })
}
