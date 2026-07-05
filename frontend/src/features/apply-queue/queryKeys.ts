import type { ApplyQueueFilter } from './constants'

// Centralised query-key factory for the apply-queue feature. Always import
// from here — never inline `['apply-queue', ...]` arrays in hooks or
// components (see features/jobs/queryKeys.ts for the rationale).
export const applyQueueQueryKeys = {
  all: () => ['apply-queue'] as const,
  list: (filter: ApplyQueueFilter, limit: number, cursor = '') =>
    ['apply-queue', 'list', filter, limit, cursor] as const,
}
