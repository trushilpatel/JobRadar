import { useMutation, useQueryClient } from '@tanstack/react-query'
import { toast } from 'sonner'
import { extractResumeKeywords } from '../api/resumes.api'
import { resumeQueryKeys } from '../queryKeys'

// Triggers/retries keyword extraction for a resume (issue #264). Kept as its
// own hook (rather than folded into useResumeActions) since it's invoked from
// a single, narrow surface (the ResumeCard "Extract keywords" action) and has
// a distinct success message.
export function useExtractResumeKeywords() {
  const queryClient = useQueryClient()

  return useMutation({
    mutationFn: (id: string) => extractResumeKeywords(id),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: resumeQueryKeys.list() })
      toast.success('Keyword extraction started')
    },
    onError: (err) =>
      toast.error(err instanceof Error ? err.message : 'Failed to extract keywords'),
  })
}
