import { useMutation, useQueryClient } from '@tanstack/react-query'
import { toast } from 'sonner'
import { setApplyQueueResume } from '../api/apply-queue.api'
import { applyQueueQueryKeys } from '../queryKeys'

export function useSetApplyQueueResume() {
  const queryClient = useQueryClient()

  return useMutation({
    mutationFn: ({ jobId, resumeId }: { jobId: string; resumeId: string | null }) =>
      setApplyQueueResume(jobId, resumeId),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: applyQueueQueryKeys.all() })
      toast.success('Resume updated')
    },
    onError: (err) =>
      toast.error(err instanceof Error ? err.message : 'Failed to update resume selection'),
  })
}
