// Job status constants — mirrors backend JobServiceConstants.go.
//
// The model is "sparse":
//   - A row in user_job_state exists ONLY when the user has acted on the job.
//   - `status` is NULLABLE — it stores a WORKFLOW stage only.
//   - "new" is synthetic: it means status IS NULL (no workflow stage).
//   - `saved_at IS NOT NULL` is the source of truth for "is this saved" —
//     ORTHOGONAL to workflow status. A job can be saved AND applied AND new.
//
// Never write 'saved' as a status. Use the dedicated save endpoint
// (setJobSaved) instead.
export const JOB_STATUS = {
  /** Synthetic — backend never stores 'new'; status IS NULL means new. */
  NEW: 'new',
  /**
   * UI tab/filter key only — corresponds to "rows where saved_at IS NOT NULL".
   * Never sent to the SetStatus endpoint; bookmarking uses setJobSaved(true/false).
   */
  SAVED: 'saved',
  APPLIED: 'applied',
  INTERVIEWING: 'interviewing',
  OFFER: 'offer',
  DISMISSED: 'dismissed',
  REJECTED: 'rejected',
} as const

export type JobStatus = (typeof JOB_STATUS)[keyof typeof JOB_STATUS]

/**
 * Stored workflow statuses — the only values the backend will ever WRITE to
 * `user_job_state.status`. Excludes:
 *  - 'saved' (orthogonal flag — uses setJobSaved)
 *  - 'new' (synthetic — represented as status IS NULL)
 *  - 'dismissed' (uses dedicated dismiss endpoint)
 */
export const WORKFLOW_STATUSES = [
  JOB_STATUS.APPLIED,
  JOB_STATUS.INTERVIEWING,
  JOB_STATUS.OFFER,
  JOB_STATUS.REJECTED,
] as const

export type WorkflowStatus = (typeof WORKFLOW_STATUSES)[number]

/**
 * Status values writable via PATCH /jobs/:id/status. Includes:
 *  - 'new' — backend interprets as "clear workflow status" (sets status NULL).
 *    Used when reverting a job out of applied/interviewing/etc back to new.
 *  - Workflow statuses (applied/interviewing/offer/rejected).
 *
 * Does NOT include 'saved' (orthogonal flag, uses setJobSaved) or
 * 'dismissed' (uses the dismiss endpoint).
 */
export const PATCHABLE_STATUSES = [JOB_STATUS.NEW, ...WORKFLOW_STATUSES] as const
export type PatchableStatus = (typeof PATCHABLE_STATUSES)[number]

// Tab keys — UI navigation. Includes the synthetic 'new' and the
// orthogonal-flag-tab 'saved'. Each tab maps to a different query on
// the backend (status IS NULL for new, saved_at IS NOT NULL for saved,
// status = X for the rest).
export const TAB_KEYS = [
  JOB_STATUS.NEW,
  JOB_STATUS.SAVED,
  JOB_STATUS.APPLIED,
  JOB_STATUS.INTERVIEWING,
  JOB_STATUS.OFFER,
  JOB_STATUS.REJECTED,
] as const

export type TabKey = (typeof TAB_KEYS)[number]

/**
 * Status select dropdown options — shown in the JobDetailPanel.
 * Workflow statuses only plus 'new' (revert path). Save/unsave is the
 * dedicated bookmark icon next to the dropdown, not a status option.
 */
export const SELECT_STATUSES = [JOB_STATUS.NEW, ...WORKFLOW_STATUSES] as const
export type SelectStatus = (typeof SELECT_STATUSES)[number]
