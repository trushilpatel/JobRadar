import type { JobFilters } from '@/api/jobs'

export const jobQueryKeys = {
  all: () => ['jobs'] as const,
  list: (filters: JobFilters, limit: number) => ['jobs', 'list', filters, limit] as const,
  counts: () => ['jobs', 'counts'] as const,
  companies: () => ['jobs', 'companies'] as const,
  detail: (id: string) => ['jobs', 'detail', id] as const,
}
