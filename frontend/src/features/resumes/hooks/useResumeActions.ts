import { useMutation, useQueryClient } from '@tanstack/react-query'
import { toast } from 'sonner'
import { uploadResume, downloadResume, renameResume, deleteResume } from '../api/resumes.api'
import { resumeQueryKeys } from '../queryKeys'

export function useResumeActions() {
  const queryClient = useQueryClient()
  const invalidate = () => void queryClient.invalidateQueries({ queryKey: resumeQueryKeys.list() })

  const upload = useMutation({
    mutationFn: ({ file, label }: { file: File; label: string }) => uploadResume(file, label),
    onSuccess: () => {
      invalidate()
      toast.success('Resume uploaded')
    },
    onError: (err) => toast.error(err instanceof Error ? err.message : 'Failed to upload resume'),
  })

  const download = useMutation({
    mutationFn: (id: string) => downloadResume(id),
    onError: (err) => toast.error(err instanceof Error ? err.message : 'Failed to download resume'),
  })

  const rename = useMutation({
    mutationFn: ({ id, label }: { id: string; label: string }) => renameResume(id, label),
    onSuccess: () => {
      invalidate()
      toast.success('Resume renamed')
    },
    onError: (err) => toast.error(err instanceof Error ? err.message : 'Failed to rename resume'),
  })

  const remove = useMutation({
    mutationFn: (id: string) => deleteResume(id),
    onSuccess: () => {
      invalidate()
      toast.success('Resume deleted')
    },
    onError: (err) => toast.error(err instanceof Error ? err.message : 'Failed to delete resume'),
  })

  return { upload, download, rename, remove }
}
