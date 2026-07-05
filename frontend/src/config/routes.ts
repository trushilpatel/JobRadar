// Centralised URL builders. NEVER hard-code paths or query strings in
// components / hooks / nav configs — import from here.
//
// This module is for OUTGOING URLs only (Link to=..., navigate(...)). Route
// patterns with :id placeholders (`/admin/resumes/:id`) live in App.tsx as
// React Router Route definitions, not here.
//
// Each builder is an arrow function returning a fully-formed URL string,
// composing query params via a single helper. Extend by adding a new key
// to the `routes` object — no other file needs to change.

interface JobsParams {
  job?: string
  tab?: string
  since?: '24h' | '7d' | '30d' | 'all'
}

// `withQuery` accepts any plain object whose values are stringable. The
// loose typing keeps the call sites readable (interface props can be passed
// directly without a cast). At runtime only string/number entries become
// query params; undefined / null / '' are dropped.
function withQuery(path: string, params?: object): string {
  if (!params) return path
  const sp = new URLSearchParams()
  for (const [k, v] of Object.entries(params)) {
    if (v !== undefined && v !== null && v !== '') sp.set(k, String(v))
  }
  const q = sp.toString()
  return q ? `${path}?${q}` : path
}

export const routes = {
  // ─── Top-level user routes ────────────────────────────────────────────────
  account: () => '/account',
  login: () => '/login',

  // ─── Jobs feed ────────────────────────────────────────────────────────────
  jobs: (params?: JobsParams) => withQuery('/jobs', params),
  // Opens the Jobs feed with a specific job's detail panel pre-opened
  // (?job=:id). Matches the URL pattern JobsPage already reads.
  jobOpen: (jobId: string, params?: Omit<JobsParams, 'job'>) =>
    withQuery('/jobs', { ...params, job: jobId }),
  jobMobileRoute: (jobId: string) => `/jobs/${jobId}`,

  // ─── Resumes ──────────────────────────────────────────────────────────────
  resumeLibrary: () => '/resumes',

  // ─── Admin ────────────────────────────────────────────────────────────────
  adminCompanies: () => '/admin/companies',
  adminUsers: () => '/admin/users',
  adminAudit: () => '/admin/audit',
  adminPipeline: () => '/admin/pipeline',
} as const

export type RouteBuilders = typeof routes
