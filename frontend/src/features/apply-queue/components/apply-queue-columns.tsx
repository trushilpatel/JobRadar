import type { ColumnDef } from '@tanstack/react-table'
import { DataTableColumnHeader } from '@/components/ui/data-table'
import { JobLocationCell } from '@/components/shared/JobLocationCell'
import { formatRelativeTime, formatExactDateTime, formatSalary } from '@/lib/utils/utils'
import type { ApplyQueueItem } from '../api/apply-queue.api'
import { ApplyQueueStatusPill } from './ApplyQueueStatusPill'
import { ResumePickerCell, type ApplyQueueResumeOption } from './ResumePickerCell'

export type { ApplyQueueResumeOption }

export interface EffectiveResume {
  id: string
  label: string
}

export interface ApplyQueueColumnActions {
  resumeOptions: ApplyQueueResumeOption[]
  onResumeChange: (jobId: string, resumeId: string | null) => void
  /** True while a resume-attach mutation for this job is in flight — disables its picker. */
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
}

export function buildApplyQueueColumns(
  actions: ApplyQueueColumnActions
): ColumnDef<ApplyQueueItem, unknown>[] {
  const {
    resumeOptions,
    onResumeChange,
    isResumeUpdatePending,
    getEffectiveResume,
    getMatchSummary,
  } = actions

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

    // ── 6. Resume picker ───────────────────────────────────────────────────
    {
      id: 'resume',
      size: 220,
      enableSorting: false,
      header: () => <span className="text-muted">Resume</span>,
      cell: ({ row }) => {
        const item = row.original
        const effective = getEffectiveResume(item.job_id)
        return (
          <ResumePickerCell
            jobId={item.job_id}
            resumeId={effective?.id ?? null}
            resumeLabel={effective?.label ?? null}
            options={resumeOptions}
            onChange={onResumeChange}
            disabled={isResumeUpdatePending?.(item.job_id) ?? false}
          />
        )
      },
    },

    // ── 7. Match ─────────────────────────────────────────────────────────────
    // One-line "N/M matched" summary against the effective resume — only
    // meaningful once a resume is selected (auto-picked or user-chosen).
    {
      id: 'match',
      size: 110,
      enableSorting: false,
      header: () => <span className="text-muted">Match</span>,
      cell: ({ row }) => {
        const summary = getMatchSummary(row.original.job_id)
        return summary ? (
          <span className="text-[12.5px] font-mono text-secondary">{summary}</span>
        ) : (
          <span className="text-[12.5px] text-placeholder">—</span>
        )
      },
    },
  ]
}
