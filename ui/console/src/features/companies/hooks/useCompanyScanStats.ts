import { useQuery } from '@tanstack/react-query'
import type { ScanStats } from '@/types'
import { fetchScanStats } from '@/api/companies'
import { companyQueryKeys } from '@/features/companies/queryKeys'

export function useCompanyScanStats(companyId: string) {
  return useQuery<ScanStats>({
    queryKey: companyQueryKeys.companyScanStats(companyId),
    queryFn: () => fetchScanStats(companyId),
    enabled: !!companyId,
  })
}
