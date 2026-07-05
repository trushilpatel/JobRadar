import { useQuery } from '@tanstack/react-query'
import { resumeQueryKeys } from '../queryKeys'
import { fetchResumes } from '../api/resumes.api'
import { CACHE_TTL } from '@/lib/core/cacheConfig'

export function useResumes() {
  return useQuery({
    queryKey: resumeQueryKeys.list(),
    queryFn: () => fetchResumes(),
    staleTime: CACHE_TTL.resumes.staleTime,
    gcTime: CACHE_TTL.resumes.gcTime,
  })
}
