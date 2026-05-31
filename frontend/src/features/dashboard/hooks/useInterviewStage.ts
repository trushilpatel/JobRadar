import { useMutation, useQueryClient } from '@tanstack/react-query'
import { toast } from 'sonner'
import { updateApplicationStage } from '../api/dashboard.api'
import { dashboardQueryKeys } from '../queryKeys'

/**
 * Mutation hook for updating an interview application's stage.
 * On success, invalidates the interviews and progress queries so the UI
 * reflects the stage change immediately.
 */
export function useInterviewStage() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: ({ id, stage }: { id: number; stage: string }) => updateApplicationStage(id, stage),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: dashboardQueryKeys.interviews() })
      queryClient.invalidateQueries({ queryKey: dashboardQueryKeys.progress() })
    },
    onError: () => toast.error('Failed to update stage'),
  })
}
