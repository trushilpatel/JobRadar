import type { AdminCompanyFilters } from '@/api/adminCompanies'

export const companyQueryKeys = {
  adminCompaniesBase: () => ['admin-companies'] as const,
  adminCompanies: (filters: AdminCompanyFilters, cursor: string, limit: number) =>
    ['admin-companies', filters, cursor, limit] as const,
  pipelineStats: () => ['pipeline-stats'] as const,
  companyAnalytics: (id: string) => ['company-analytics', id] as const,
  companyScanStats: (id: string) => ['company-scan-stats', id] as const,
  scanQueue: () => ['scan-queue'] as const,
}
