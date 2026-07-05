import type { ColumnDef } from '@tanstack/react-table'
import { DataTableColumnHeader } from '@/components/ui/data-table'
import { JobLocationCell } from '@/components/shared/JobLocationCell'
import { formatRelativeTime, formatExactDateTime, formatSalary } from '@/lib/utils/utils'
import type { ApplyQueueItem } from '../api/apply-queue.api'
import { ApplyQueueStatusPill } from './ApplyQueueStatusPill'
import { ResumePickerCell, type ApplyQueueResumeOption } from './ResumePickerCell'

export type { ApplyQueueResumeOption }

export interface ApplyQueueColumnActions {
  resumeOptions: ApplyQueueResumeOption[]
  onResumeChange: (jobId: string, resumeId: string | null) => void
  /** True while a resume-attach mutation for this job is in flight — disables its picker. */
  isResumeUpdatePending?: (jobId: string) => boolean
}

export function buildApplyQueueColumns(
  actions: ApplyQueueColumnActions
): ColumnDef<ApplyQueueItem, unknown>[] {
  const { resumeOptions, onResumeChange, isResumeUpdatePending } = actions

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
      size: 240,
      enableSorting: false,
      header: () => <span className="text-muted">Resume</span>,
      cell: ({ row }) => {
        const item = row.original
        // Marks the option matching this row's keyword-match recommendation
        // (issue #264) — computed per row since the recommendation differs
        // per job, unlike the shared resumeOptions list itself.
        const options = resumeOptions.map((opt) => ({
          ...opt,
          recommended: opt.id === item.recommended_resume_id,
        }))
        return (
          <ResumePickerCell
            jobId={item.job_id}
            resumeId={item.resume_id}
            resumeLabel={item.resume_label}
            options={options}
            onChange={onResumeChange}
            disabled={isResumeUpdatePending?.(item.job_id) ?? false}
          />
        )
      },
    },
  ]
}
