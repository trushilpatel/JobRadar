export const NAV_TOKENS = {
  'nav.dashboard': { copy: 'Dashboard' },
  'nav.jobs': { copy: 'Jobs' },
  'nav.companies': { copy: 'Companies' },
  'nav.users': { copy: 'Users' },
  'nav.resumes': { copy: 'Resumes' },
  'nav.apply-queue': { copy: 'Apply Queue' },
  'nav.audit-log': { copy: 'Audit Log' },
  'nav.insights': { copy: 'Insights' },
  'nav.settings': { copy: 'Settings' },
  'nav.account': { copy: 'Account' },
} as const satisfies Record<string, { copy: string; shortcut?: string }>
