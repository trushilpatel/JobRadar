import { useMutation, useQueryClient } from '@tanstack/react-query'
import { toast } from 'sonner'
import { setJobSaved, setJobStatus, dismissJob } from '@/api/jobs'

export function useJobActions() {
  const queryClient = useQueryClient()
  const invalidateJobs = () => queryClient.invalidateQueries({ queryKey: ['jobs'] })

  const save = useMutation({
    mutationFn: ({ id, saved }: { id: string; saved: boolean }) => setJobSaved(id, saved),
    onSuccess: (_, { saved }) => {
      toast.success(saved ? 'Job saved' : 'Removed from saved')
      invalidateJobs()
    },
    onError: () => toast.error('Failed to update job'),
  })

  const apply = useMutation({
    mutationFn: (id: string) => setJobStatus(id, 'applied'),
    onSuccess: () => {
      toast.success('Marked as applied')
      invalidateJobs()
    },
    onError: () => toast.error('Failed to update job'),
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

  const markInterviewing = useMutation({
    mutationFn: (id: string) => setJobStatus(id, 'interviewing'),
    onSuccess: () => {
      toast.success('Marked as interviewing')
      invalidateJobs()
    },
    onError: () => toast.error('Failed to update job'),
  })

  const markOffer = useMutation({
    mutationFn: (id: string) => setJobStatus(id, 'offer'),
    onSuccess: () => {
      toast.success('Marked as offer')
      invalidateJobs()
    },
    onError: () => toast.error('Failed to update job'),
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

  return { save, apply, markInterviewing, markOffer, dismiss, bulkDismiss }
}
