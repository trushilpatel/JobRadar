// Centralised query-key factory for the resumes feature. Same pattern as
// features/jobs/queryKeys.ts — never inline arrays in hooks.

export const resumeQueryKeys = {
  list: () => ['resumes', 'list'] as const,
}
