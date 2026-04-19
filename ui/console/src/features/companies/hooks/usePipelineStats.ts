import { useQuery } from '@tanstack/react-query'
import { fetchPipelineStats } from '@/api/dashboard'

export function usePipelineStats() {
  return useQuery({
    queryKey: ['pipeline-stats'],
    queryFn: fetchPipelineStats,
    refetchInterval: 30_000,
    staleTime: 30_000,
  })
}
