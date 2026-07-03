/**
 * Pure display strings that are NOT commands — sidebar/nav labels, job tab
 * titles, and the search placeholder. Layer 0: imports nothing. Job commands
 * (label + keys) live in `features/jobs/commands/`, not here.
 */
export const COPY = {
  // Navigation (sidebar)
  'nav.jobs': 'Jobs',
  'nav.companies': 'Companies',
  'nav.users': 'Users',
  'nav.resumes': 'Resumes',
  'nav.apply-queue': 'Apply Queue',
  'nav.audit-log': 'Audit Log',
  'nav.insights': 'Insights',
  'nav.pipeline': 'Pipeline',
  'nav.account': 'Account',

  // Job tab titles
  'jobs.tab.new': 'New',
  'jobs.tab.saved': 'Saved',
  'jobs.tab.applied': 'Applied',
  'jobs.tab.interviewing': 'Interviewing',
  'jobs.tab.offer': 'Offer',
  'jobs.tab.rejected': 'Rejected',

  // Search
  'jobs.search.placeholder': 'Search jobs…',
} as const satisfies Record<string, string>

export type CopyKey = keyof typeof COPY
