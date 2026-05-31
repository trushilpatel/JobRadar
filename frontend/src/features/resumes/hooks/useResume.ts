import { useQuery } from '@tanstack/react-query'
import { resumeQueryKeys } from '../queryKeys'
import { MOCK_ATTACHMENTS, MOCK_RESUMES, MOCK_VERSIONS } from '../mockData'
import type { Resume, ResumeJobAttachment, ResumeVersion } from '../types'

interface ResumeDetailResult {
  resume: Resume | null
  versions: ResumeVersion[]
  attachments: ResumeJobAttachment[]
}

// Mockup hook — would call GET /admin/resumes/:id (returns resume + versions
// + attachments in one envelope) once backend is wired.
export function useResume(id: string | undefined) {
  return useQuery<ResumeDetailResult>({
    queryKey: resumeQueryKeys.detail(id ?? ''),
    enabled: !!id,
    queryFn: () => {
      const resume = MOCK_RESUMES.find((r) => r.id === id) ?? null
      const versions = id ? (MOCK_VERSIONS[id] ?? []) : []
      const attachments = id ? MOCK_ATTACHMENTS.filter((a) => a.resume_id === id) : []
      return Promise.resolve({ resume, versions, attachments })
    },
    staleTime: 30_000,
  })
}
