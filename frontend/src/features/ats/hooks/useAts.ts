import { useQuery } from '@tanstack/react-query'
import { getAts } from '../api/ats.api'
import { atsQueryKeys } from '../queryKeys'

export function useAts() {
  return useQuery({
    queryKey: atsQueryKeys.list(),
    queryFn: getAts,
    staleTime: 10 * 60 * 1000,
  })
}
