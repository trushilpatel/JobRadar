import { useQuery } from '@tanstack/react-query'
import { resumeQueryKeys } from '../queryKeys'
import { MOCK_RESUMES } from '../mockData'
import type { Resume } from '../types'

// Mockup hook — returns stub list. Will be replaced with `fetchResumes()`
// against /admin/resumes once the backend slice lands.
export function useResumes() {
  return useQuery<Resume[]>({
    queryKey: resumeQueryKeys.list(),
    queryFn: () => Promise.resolve(MOCK_RESUMES.filter((r) => !r.archived_at)),
    staleTime: 30_000,
  })
}
