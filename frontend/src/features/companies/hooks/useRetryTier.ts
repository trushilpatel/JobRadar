import { useMutation, useQueryClient } from '@tanstack/react-query'
import { toast } from 'sonner'
import { triggerRetry } from '@/lib/api/pipeline'
import { companyQueryKeys } from '../queryKeys'

export function useRetryTier() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({
      worker,
      tier,
    }: {
      worker: 'filter' | 'enricher'
      tier: '1m' | '10m' | '1h' | 'dlq'
    }) => triggerRetry(worker, tier),
    onSuccess: (data) => {
      qc.invalidateQueries({ queryKey: companyQueryKeys.pipelineStats() })
      toast.success(`Retrying ${data.drained} job${data.drained === 1 ? '' : 's'}`)
    },
    onError: () => {
      toast.error('Failed to trigger retry')
    },
  })
}
