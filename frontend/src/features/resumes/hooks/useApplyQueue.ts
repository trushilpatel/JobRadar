import { useQuery } from '@tanstack/react-query'
import { resumeQueryKeys } from '../queryKeys'
import { buildApplyQueue } from '../mockData'
import type { ApplyQueueItem } from '../types'
import type { ApplyQueueFilter } from '@/config/routes'

// Mockup hook — would call GET /admin/apply-queue?filter=... once backend
// exists. Filter rules match the spec in Designs/resume-feature-spec.md §1:
//
//   - all           → every row (default)
//   - needs-resume  → saved jobs with no attachment
//   - draft         → attachment.state === 'draft'
//   - final         → attachment.state === 'final'
//   - submitted     → attachment.state === 'submitted'
//   - interviewing  → job.lifecycle_status === 'interviewing' AND has attachment
//
// (interviewing/applied without an attachment never appear here — they belong
// in the Jobs feed.)
function applyFilter(items: ApplyQueueItem[], filter: ApplyQueueFilter): ApplyQueueItem[] {
  switch (filter) {
    case 'all':
      return items
    case 'needs-resume':
      return items.filter((i) => i.attachment === null)
    case 'draft':
      return items.filter((i) => i.attachment?.state === 'draft')
    case 'final':
      return items.filter((i) => i.attachment?.state === 'final')
    case 'submitted':
      return items.filter((i) => i.attachment?.state === 'submitted')
    case 'interviewing':
      return items.filter((i) => i.job.lifecycle_status === 'interviewing' && i.attachment !== null)
  }
}

export function useApplyQueue(filter: ApplyQueueFilter = 'all') {
  return useQuery<ApplyQueueItem[]>({
    queryKey: resumeQueryKeys.applyQueue(filter),
    queryFn: () => Promise.resolve(applyFilter(buildApplyQueue(), filter)),
    staleTime: 30_000,
  })
}
