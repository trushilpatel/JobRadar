import { useQuery } from '@tanstack/react-query'
import { fetchPipelineStats } from '@/lib/api/pipeline'
import { companyQueryKeys } from '../queryKeys'

export function usePipelineStats() {
  return useQuery({
    queryKey: companyQueryKeys.pipelineStats(),
    queryFn: fetchPipelineStats,
    refetchInterval: 30_000,
    staleTime: 30_000,
  })
}
