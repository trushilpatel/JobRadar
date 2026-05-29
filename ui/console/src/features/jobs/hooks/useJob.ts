import { useQuery } from '@tanstack/react-query'
import { fetchJob } from '../api/jobs.api'
import { jobQueryKeys } from '../queryKeys'

export function useJob(id: string | undefined) {
  return useQuery({
    queryKey: jobQueryKeys.detail(id ?? ''),
    queryFn: () => fetchJob(id as string),
    enabled: !!id,
    staleTime: 30_000,
  })
}
