// Job status constants — mirrors backend JobServiceConstants.go.
// 'new' is synthetic: it means no row exists in user_job_state.
// Never write 'new' directly to the API; use setJobSaved(false) to delete the row.
export const JOB_STATUS = {
  NEW: 'new',
  SAVED: 'saved',
  APPLIED: 'applied',
  INTERVIEWING: 'interviewing',
  OFFER: 'offer',
  DISMISSED: 'dismissed',
  REJECTED: 'rejected',
} as const

export type JobStatus = (typeof JOB_STATUS)[keyof typeof JOB_STATUS]

// Statuses writable via PATCH /jobs/:id/status.
// Excludes 'new' (synthetic — use setJobSaved(false)) and 'dismissed' (use DELETE /jobs/:id).
export const PATCHABLE_STATUSES = [
  JOB_STATUS.SAVED,
  JOB_STATUS.APPLIED,
  JOB_STATUS.INTERVIEWING,
  JOB_STATUS.OFFER,
  JOB_STATUS.REJECTED,
] as const

export type PatchableStatus = (typeof PATCHABLE_STATUSES)[number]

// Tab keys — all statuses shown as navigation tabs (excludes 'dismissed').
export const TAB_KEYS = [
  JOB_STATUS.NEW,
  JOB_STATUS.SAVED,
  JOB_STATUS.APPLIED,
  JOB_STATUS.INTERVIEWING,
  JOB_STATUS.OFFER,
  JOB_STATUS.REJECTED,
] as const

export type TabKey = (typeof TAB_KEYS)[number]

// Status select dropdown options — same as tabs; 'new' selection routes to setJobSaved(false).
export const SELECT_STATUSES = TAB_KEYS
export type SelectStatus = TabKey
