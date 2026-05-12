import type { AdminCompanyFilters } from '@/api/adminCompanies'

export const companyQueryKeys = {
  adminCompaniesBase: () => ['admin-companies'] as const,
  adminCompanies: (filters: AdminCompanyFilters, page: number, limit: number) =>
    ['admin-companies', filters, page, limit] as const,
  pipelineStats: () => ['pipeline-stats'] as const,
  companyAnalytics: (id: string) => ['company-analytics', id] as const,
  companyScanStats: (id: string) => ['company-scan-stats', id] as const,
  scanQueue: () => ['scan-queue'] as const,
}
