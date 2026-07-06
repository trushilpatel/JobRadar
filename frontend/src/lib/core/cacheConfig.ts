// Central query-cache TTL config — scoped so far to queries known to be safe
// with a longer cache window (data that changes rarely and is already
// explicitly invalidated on the mutations that do change it). Queries that
// must stay "live" (e.g. useApplyQueue — job status/resume attachment must
// read as current on every navigation) are deliberately NOT added here; they
// keep their own inline staleTime/gcTime. Adding a new domain to this cache
// tier is one entry here plus pointing its hook at it — no change to the
// mechanism itself.
export const CACHE_TTL = {
  // Resumes rarely change — only on upload/delete, both of which already
  // invalidate resumeQueryKeys.list() explicitly (see useResumeActions.ts).
  resumes: { staleTime: 5 * 60_000, gcTime: 30 * 60_000 },
} as const
