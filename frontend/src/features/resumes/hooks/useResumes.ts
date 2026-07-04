import { useQuery } from '@tanstack/react-query'
import { resumeQueryKeys } from '../queryKeys'
import { fetchResumes } from '../api/resumes.api'

export function useResumes() {
  return useQuery({
    queryKey: resumeQueryKeys.list(),
    queryFn: () => fetchResumes(),
    staleTime: 30_000,
  })
}
