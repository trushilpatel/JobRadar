// Centralised query-key factory for the resumes feature. Same pattern as
// features/jobs/queryKeys.ts — never inline arrays in hooks.

export const resumeQueryKeys = {
  all: () => ['resumes'] as const,
  list: () => ['resumes', 'list'] as const,
  detail: (id: string) => ['resumes', 'detail', id] as const,
  versions: (id: string) => ['resumes', 'versions', id] as const,
  attachedJobs: (id: string) => ['resumes', 'attached', id] as const,
  applyQueue: (state?: string) => ['apply-queue', state ?? 'all'] as const,
}
