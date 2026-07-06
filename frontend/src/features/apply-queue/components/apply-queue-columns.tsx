import type { ColumnDef } from '@tanstack/react-table'
import { DataTableColumnHeader } from '@/components/ui/data-table'
import { JobLocationCell } from '@/components/shared/JobLocationCell'
import { formatRelativeTime, formatExactDateTime, formatSalary, cn } from '@/lib/utils/utils'
import type { ApplyQueueItem } from '../api/apply-queue.api'
import { ApplyQueueStatusPill } from './ApplyQueueStatusPill'

export interface EffectiveResume {
  id: string
  label: string
}

// One resume's match detail against a specific job's keywords, for the
// resume-picker dialog's table (sorted best-match-first by the hook that
// builds this). Defined here (not in pages/apply-queue/useApplyQueueResumeMatch.ts)
// because features/ may not import from pages/ — the hook and the dialog
// component both import this shape from the feature barrel instead.
export interface ResumeMatchOption {
  id: string
  label: string
  matched: number
  total: number
  matchedKeywords: string[]
  missingKeywords: string[]
  /** False when the resume predates extracted_text backfill or extraction otherwise produced nothing — distinct from a genuine 0/N poor match. */
  hasExtractedText: boolean
}

export interface ApplyQueueColumnActions {
  /** True while a resume-attach mutation for this job is in flight — disables its picker trigger. */
  isResumeUpdatePending?: (jobId: string) => boolean
  /**
   * Resolves the resume actually shown as selected for a row — the user's
   * explicit attachment when one exists, otherwise the best client-side
   * auto-pick, otherwise null. Computed at the page layer (see
   * pages/apply-queue/useApplyQueueResumeMatch.ts) since it needs both this
   * feature's items and the resumes feature's data.
   */
  getEffectiveResume: (jobId: string) => EffectiveResume | null
  /** One-line "N/M matched" summary for the row's effective resume, or null when none is selected or the job has no keywords. */
  getMatchSummary: (jobId: string) => string | null
  /** Opens the resume-picker dialog for this job — the dialog itself is a single shared instance owned by ApplyQueueTable, not per-row. */
  onOpenResumePicker: (jobId: string) => void
}

export function buildApplyQueueColumns(
  actions: ApplyQueueColumnActions
): ColumnDef<ApplyQueueItem, unknown>[] {
  const { isResumeUpdatePending, getEffectiveResume, getMatchSummary, onOpenResumePicker } = actions

  return [
    // ── 1. Job (title, linked to posting + company) ────────────────────────
    {
      id: 'job',
      size: 260,
      header: () => <span className="text-muted">Job</span>,
      cell: ({ row }) => {
        const item = row.original
        return (
          <div className="flex flex-col min-w-0 gap-0.5">
            <a
              href={item.job_url}
              target="_blank"
              rel="noopener noreferrer"
              onClick={(e) => e.stopPropagation()}
              title={item.job_title}
              className="font-semibold text-primary text-sm truncate leading-tight hover:text-violet hover:underline"
            >
              {item.job_title}
            </a>
            <span className="text-[13px] text-secondary truncate">{item.company}</span>
          </div>
        )
      },
    },

    // ── 2. Location ──────────────────────────────────────────────────────────
    // Shares JobLocationCell with the Jobs table (components/shared/) so both
    // tables render remote/on-site jobs identically without duplicating JSX.
    {
      id: 'location',
      size: 180,
      enableSorting: false,
      header: () => <span className="text-muted">Location</span>,
      cell: ({ row }) => (
        <JobLocationCell remote={row.original.remote} location={row.original.location} />
      ),
    },

    // ── 3. Salary ──────────────────────────────────────────────────────────
    {
      id: 'salary',
      size: 140,
      enableSorting: false,
      header: () => <span className="text-muted">Salary</span>,
      cell: ({ row }) => {
        const salary = formatSalary(row.original.salary_range)
        return salary ? (
          <span className="font-mono text-[13px] font-semibold text-salary">{salary}</span>
        ) : (
          <span className="text-[13px] text-placeholder">Not listed</span>
        )
      },
    },

    // ── 4. Status ────────────────────────────────────────────────────────────
    {
      id: 'status',
      size: 130,
      enableSorting: false,
      header: () => <span className="text-muted">Status</span>,
      cell: ({ row }) => <ApplyQueueStatusPill status={row.original.status} />,
    },

    // ── 5. Posted (added) ────────────────────────────────────────────────────
    {
      accessorKey: 'added_at',
      id: 'added_at',
      size: 100,
      header: ({ column }) => <DataTableColumnHeader column={column} title="Posted" />,
      cell: ({ getValue }) => {
        const addedAt = getValue() as string
        return (
          <span
            className="text-[12.5px] text-muted"
            title={`Added ${formatExactDateTime(addedAt)}`}
          >
            {formatRelativeTime(addedAt)}
          </span>
        )
      },
    },

    // ── 6. Resume + match — opens the shared resume-picker dialog ──────────
    {
      id: 'resume',
      size: 240,
      enableSorting: false,
      header: () => <span className="text-muted">Resume</span>,
      cell: ({ row }) => {
        const item = row.original
        const effective = getEffectiveResume(item.job_id)
        const summary = getMatchSummary(item.job_id)
        const pending = isResumeUpdatePending?.(item.job_id) ?? false
        return (
          <button
            type="button"
            disabled={pending}
            onClick={(e) => {
              e.stopPropagation()
              onOpenResumePicker(item.job_id)
            }}
            aria-label={`Select resume for ${item.job_id}`}
            className={cn(
              'flex h-7 w-full max-w-[240px] items-center gap-2 rounded-md border border-[var(--border)] bg-panel px-2 text-xs hover:bg-subtle disabled:opacity-60'
            )}
          >
            <span className="truncate text-primary">{effective?.label ?? 'Select resume'}</span>
            {summary && (
              <span className="ml-auto shrink-0 font-mono text-[11px] text-secondary">
                {summary.replace(' matched', '')}
              </span>
            )}
          </button>
        )
      },
    },
  ]
}
