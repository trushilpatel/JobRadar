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

// Tab keys — UI navigation. Includes the synthetic 'new', the
// orthogonal-flag-tab 'saved', the synthetic 'tracking' tab (combined view
// over the 4 workflow stages below), AND the 4 individual workflow statuses
// — kept here ONLY so a pre-existing `?tab=applied`-style deep link still
// parses instead of silently falling back to the default tab. They are no
// longer rendered as top-level tabs (JobStatusTabs only renders
// New/Saved/Tracking) — useJobUrlState redirects any legacy single-stage
// value into `tracking` with that one stage pre-checked on mount.
// 'tracking' is NEVER sent to the backend as a literal `status=tracking`
// value — the frontend translates it into a `statuses` (plural) query param
// carrying the checked TRACKING_STAGES instead. See useJobs.ts / jobs.api.ts.
export const TAB_KEYS = [
  JOB_STATUS.NEW,
  JOB_STATUS.SAVED,
  JOB_STATUS.APPLIED,
  JOB_STATUS.INTERVIEWING,
  JOB_STATUS.OFFER,
  JOB_STATUS.REJECTED,
  'tracking',
] as const

export type TabKey = (typeof TAB_KEYS)[number]

/**
 * The 4 workflow stages selectable inside the Tracking tab's checkbox
 * sub-menu. All 4 are checked by default (combined view). Backed by
 * `JobFilter.Statuses` on the backend (migration-free — see
 * UserJobStateRepositoryService.GetJobsByStatus).
 */
export const TRACKING_STAGES = [...WORKFLOW_STATUSES] as const
export type TrackingStage = (typeof TRACKING_STAGES)[number]

/**
 * Status select dropdown options — shown in the JobDetailPanel.
 * Workflow statuses only plus 'new' (revert path). Save/unsave is the
 * dedicated bookmark icon next to the dropdown, not a status option.
 */
export const SELECT_STATUSES = [JOB_STATUS.NEW, ...WORKFLOW_STATUSES] as const
export type SelectStatus = (typeof SELECT_STATUSES)[number]
