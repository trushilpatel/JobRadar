import { useQuery } from '@tanstack/react-query'
import type { CompanyAnalytics } from '@/types'
import { fetchCompanyAnalytics } from '../api/companies.api'
import { companyQueryKeys } from '../queryKeys'

export function useCompanyAnalytics(companyId: string) {
  return useQuery<CompanyAnalytics>({
    queryKey: companyQueryKeys.companyAnalytics(companyId),
    queryFn: () => fetchCompanyAnalytics(companyId),
    enabled: !!companyId,
  })
}
