import type { JobFilters } from '@/api/jobs'

/**
 * Centralised query-key factory for the jobs feature. Always import from here
 * — never inline `['jobs', ...]` arrays in hooks or components. Drift between
 * a writer and a reader of the cache silently breaks lookups and invalidation
 * (the original `lookupStatus` bug). The eslint guide in
 * docs/superpowers/specs has more.
 */
export const jobQueryKeys = {
  all: () => ['jobs'] as const,
  list: (filters: JobFilters, limit: number) => ['jobs', 'list', filters, limit] as const,
  counts: () => ['jobs', 'counts'] as const,
  companies: () => ['jobs', 'companies'] as const,
  detail: (id: string) => ['jobs', 'detail', id] as const,
}

/**
 * True when a TanStack query key belongs to the jobs LIST cache (excludes
 * counts / companies / detail / any other future namespaced sub-key). Used
 * by mutation `onMutate` snapshots and by undo's `lookupStatus`.
 */
export function isJobListKey(queryKey: readonly unknown[]): boolean {
  if (queryKey[0] !== 'jobs') return false
  const second = queryKey[1]
  // Anything other than the list segment (or a legacy raw-filters shape) is
  // a non-list cache and should be ignored by list scans.
  return second === 'list' || (typeof second === 'object' && second !== null)
}
