import type { ColumnDef } from '@tanstack/react-table'
import { Bookmark, Paperclip, ArrowUpRight } from 'lucide-react'
import type { Job } from '@/types'
import { DataTableColumnHeader } from '@/components/ui/data-table'
import { formatRelativeTime, formatExactDateTime, cn, formatSalary } from '@/lib/utils/utils'
import { CompanyAvatar } from '@/components/shared/CompanyAvatar'
import { JobLocationCell } from '@/components/shared/JobLocationCell'
import { JobStatusPill } from './JobStatusPill'

export interface JobTableActions {
  onToggleSave: (job: Job) => void
  /** Opens the posting URL and auto-advances status to "applied" — matches
   *  the approved mockup's row-level Apply action. */
  onApply: (job: Job) => void
  /** Optional callback used by the pages layer to inject cross-feature data
   *  (e.g. resume attachment state) without creating a cross-feature import
   *  inside the jobs feature. Returns the attachment state string or null. */
  getAttachmentState?: (jobId: string) => string | null
}

export function buildJobColumns(actions: JobTableActions): ColumnDef<Job, unknown>[] {
  const { onToggleSave, onApply, getAttachmentState } = actions

  return [
    // ── 1. Bookmark / save column ───────────────────────────────────────────
    // Always-visible, one-click save toggle at the row's left edge. Replaces
    // the old checkbox column; multi-select via checkbox is no longer offered.
    {
      id: 'bookmark',
      size: 48,
      enableSorting: false,
      header: () => null,
      cell: ({ row }) => {
        const job = row.original
        // Source of truth: saved_at IS NOT NULL — orthogonal to status.
        // A saved job stays in whatever workflow stage it's in (typically
        // none/new) and visually shows the filled bookmark in every tab
        // where it appears.
        const saved = job.user_state?.saved_at != null
        return (
          <button
            type="button"
            onClick={(e) => {
              e.stopPropagation()
              onToggleSave(job)
            }}
            aria-label={saved ? `Remove ${job.title} from saved` : `Save ${job.title}`}
            aria-pressed={saved}
            title={saved ? 'Remove from saved' : 'Save'}
            className={cn(
              'p-1.5 rounded-md transition-colors',
              'focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-violet',
              saved ? 'text-salary hover:bg-hover' : 'text-muted hover:text-salary hover:bg-hover'
            )}
          >
            <Bookmark size={15} fill={saved ? 'currentColor' : 'none'} />
            {/* Lucide accepts fill/stroke — same icon, different state. */}
          </button>
        )
      },
    },

    // ── 2. Title (role) ────────────────────────────────────────────────────
    // Bounded width (unlike the other flexible columns, this one has no
    // `size` upstream default other than TanStack's 150 fallback) — without
    // an explicit size the browser hands it all the table's leftover space,
    // stretching short titles across most of the row. Long titles that still
    // overflow the bound get an ellipsis plus a native tooltip so the full
    // text stays reachable, matching the tooltip pattern already used by the
    // Posted column below.
    {
      accessorKey: 'title',
      size: 120,
      header: ({ column }) => <DataTableColumnHeader column={column} title="Role" />,
      cell: ({ row, getValue }) => {
        // Mockup paperclip indicator — shown when a resume is attached to
        // this job (any state). Real impl will read from user_job_state /
        // resume_job_attachments via a join. Attachment state is injected
        // via getAttachmentState by the pages layer to avoid a cross-feature
        // import inside features/jobs.
        const attachmentState = getAttachmentState?.(row.original.id) ?? null
        const title = getValue() as string
        return (
          <div className="flex items-center gap-1.5 min-w-0">
            {attachmentState && (
              <Paperclip
                size={11}
                className="text-violet shrink-0"
                aria-label={`Resume attached (${attachmentState})`}
              />
            )}
            <span
              title={title}
              className="font-semibold text-primary text-sm truncate leading-tight"
            >
              {title}
            </span>
          </div>
        )
      },
    },

    // ── 3. Company ─────────────────────────────────────────────────────────
    {
      id: 'company',
      size: 180,
      accessorFn: (row) => row.company?.name ?? row.company_id ?? '',
      header: ({ column }) => <DataTableColumnHeader column={column} title="Company" />,
      cell: ({ row }) => {
        const name = row.original.company?.name ?? row.original.company_id ?? ''
        return (
          <div className="flex items-center gap-2.5 min-w-0">
            <CompanyAvatar name={name} size="sm" />
            <span className="text-[13px] text-secondary font-medium truncate max-w-[120px]">
              {name}
            </span>
          </div>
        )
      },
    },

    // ── 4. Status ──────────────────────────────────────────────────────────
    {
      id: 'status',
      size: 120,
      enableSorting: false,
      header: () => <span className="text-muted">Status</span>,
      cell: ({ row }) => <JobStatusPill status={row.original.status} />,
    },

    // ── 5. Location ────────────────────────────────────────────────────────
    // Mockup direction: location cell shows ONLY the location text (or a
    // Remote indicator). Visa/sponsorship lives in the drawer meta, never here.
    {
      id: 'location',
      size: 180,
      enableSorting: false,
      header: () => <span className="text-muted">Location</span>,
      cell: ({ row }) => (
        <JobLocationCell remote={row.original.remote} location={row.original.location} />
      ),
    },

    // ── 6. Salary ──────────────────────────────────────────────────────────
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

    // ── 7. Posted (added) ──────────────────────────────────────────────────
    {
      accessorKey: 'added_at',
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

    // ── 8. Apply (always visible) ──────────────────────────────────────────
    // Opens the posting URL in a new tab and auto-advances status to
    // "applied" (no-op on status if already applied/interviewing/offer/
    // rejected) — the coupled behavior the approved mockup's Apply action has.
    {
      id: 'apply',
      size: 40,
      enableSorting: false,
      header: () => null,
      cell: ({ row }) => (
        <button
          type="button"
          onClick={(e) => {
            e.stopPropagation()
            onApply(row.original)
          }}
          aria-label={`Apply to ${row.original.title}`}
          title="Apply"
          className={cn(
            'p-1.5 rounded-md transition-colors text-muted hover:text-violet hover:bg-hover',
            'focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-violet'
          )}
        >
          <ArrowUpRight size={14} />
        </button>
      ),
    },
  ]
}
