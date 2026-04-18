import { useMutation, useQueryClient } from '@tanstack/react-query'
import { toast } from 'sonner'
import { setJobStatus, dismissJob } from '@/api/jobs'
import type { JobStatus } from '@/types'

export function useJobActions() {
  const queryClient = useQueryClient()
  const invalidateJobs = () => queryClient.invalidateQueries({ queryKey: ['jobs'] })

  const setStatus = useMutation({
    mutationFn: ({ id, status }: { id: string; status: JobStatus }) => setJobStatus(id, status),
    onSuccess: (_, { status }) => {
      toast.success(`Moved to ${status}`)
      invalidateJobs()
    },
    onError: () => toast.error('Failed to update status'),
  })

  const dismiss = useMutation({
    mutationFn: ({ id, reason, note }: { id: string; reason: string; note?: string }) =>
      dismissJob(id, reason, note),
    onSuccess: () => {
      toast.success('Job dismissed')
      invalidateJobs()
    },
    onError: () => toast.error('Failed to dismiss job'),
  })

  const bulkDismiss = useMutation({
    mutationFn: (ids: string[]) =>
      Promise.all(ids.map((id) => dismissJob(id, 'user_not_relevant'))),
    onSuccess: (_, ids) => {
      toast.success(`${ids.length} job${ids.length !== 1 ? 's' : ''} dismissed`)
      invalidateJobs()
    },
    onError: () => toast.error('Failed to dismiss some jobs'),
  })

  return { setStatus, dismiss, bulkDismiss }
}
