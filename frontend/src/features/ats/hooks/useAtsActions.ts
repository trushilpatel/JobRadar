import { useMutation, useQueryClient } from '@tanstack/react-query'
import { toast } from 'sonner'
import { updateAts } from '../api/ats.api'
import { atsQueryKeys } from '../queryKeys'
import type { ATS } from '@/types'

interface UpdateAtsArgs {
  name: string
  data: Partial<
    Pick<ATS, 'scan_interval_min' | 'inter_company_delay_s' | 'is_active' | 'is_admin_creatable'>
  >
}

export function useAtsActions() {
  const queryClient = useQueryClient()

  return useMutation({
    mutationFn: ({ name, data }: UpdateAtsArgs) => updateAts(name, data),
    onSuccess: (_result, { name }) => {
      void queryClient.invalidateQueries({ queryKey: atsQueryKeys.list() })
      void queryClient.invalidateQueries({ queryKey: atsQueryKeys.detail(name) })
      toast.success(`${name} config updated`)
    },
    onError: (err) => {
      toast.error(err instanceof Error ? err.message : 'Failed to update ATS config')
    },
  })
}
