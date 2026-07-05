import type { ColumnDef } from '@tanstack/react-table'
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

    // ── 2. Status ────────────────────────────────────────────────────────────
    {
      id: 'status',
      size: 130,
      enableSorting: false,
      header: () => <span className="text-muted">Status</span>,
      cell: ({ row }) => <ApplyQueueStatusPill status={row.original.status} />,
    },

    // ── 3. Resume picker ───────────────────────────────────────────────────
    {
      id: 'resume',
      size: 240,
      enableSorting: false,
      header: () => <span className="text-muted">Resume</span>,
      cell: ({ row }) => {
        const item = row.original
        return (
          <ResumePickerCell
            jobId={item.job_id}
            resumeId={item.resume_id}
            resumeLabel={item.resume_label}
            options={resumeOptions}
            onChange={onResumeChange}
            disabled={isResumeUpdatePending?.(item.job_id) ?? false}
          />
        )
      },
    },
  ]
}
