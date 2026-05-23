import type { ColumnDef } from '@tanstack/react-table'
import { MapPin, Wifi, Bookmark, X, Paperclip } from 'lucide-react'
import type { Job } from '@/types'
import { DataTableColumnHeader } from '@/components/ui/data-table'
import { formatRelativeTime, cn } from '@/lib/utils'
import { CompanyAvatar } from '@/components/shared/CompanyAvatar'
import { formatSalary } from '@/services/jobs.service'
import { getMockAttachmentForJob } from '@/features/resumes/mockData'

export interface JobTableActions {
  onToggleSave: (job: Job) => void
  onDismiss: (job: Job) => void
}

export function buildJobColumns(actions: JobTableActions): ColumnDef<Job, unknown>[] {
  const { onToggleSave, onDismiss } = actions

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
              'focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-violet-400',
              saved
                ? 'text-amber-400 hover:bg-amber-500/10'
                : 'text-slate-500 hover:text-amber-300 hover:bg-white/[0.06]'
            )}
          >
            <Bookmark size={15} fill={saved ? 'currentColor' : 'none'} />
            {/* Lucide accepts fill/stroke — same icon, different state. */}
          </button>
        )
      },
    },

    // ── 2. Company ─────────────────────────────────────────────────────────
    {
      id: 'company',
      size: 180,
      accessorFn: (row) => row.company?.name ?? row.company_id ?? '',
      header: ({ column }) => <DataTableColumnHeader column={column} title="Company" />,
      cell: ({ row }) => {
        const name = row.original.company?.name ?? row.original.company_id ?? ''
        return (
          <div className="flex items-center gap-2.5">
            <CompanyAvatar name={name} size="sm" />
            <span className="text-xs text-slate-300 truncate max-w-[120px]">{name}</span>
          </div>
        )
      },
    },

    // ── 3. Title ───────────────────────────────────────────────────────────
    {
      accessorKey: 'title',
      header: ({ column }) => <DataTableColumnHeader column={column} title="Title" />,
      cell: ({ row, getValue }) => {
        // Mockup paperclip indicator — shown when a resume is attached to
        // this job (any state). Real impl will read from user_job_state /
        // resume_job_attachments via a join, not from mock data.
        const attachment = getMockAttachmentForJob(row.original.id)
        return (
          <div className="flex items-center gap-1.5 min-w-0">
            {attachment && (
              <Paperclip
                size={11}
                className="text-violet-400 shrink-0"
                aria-label={`Resume attached (${attachment.state})`}
              />
            )}
            <span className="font-medium text-white text-sm truncate">{getValue() as string}</span>
          </div>
        )
      },
    },

    // ── 4. Location ────────────────────────────────────────────────────────
    {
      id: 'location',
      size: 180,
      enableSorting: false,
      header: () => <span className="text-slate-400">Location</span>,
      cell: ({ row }) => {
        const j = row.original
        if (j.remote)
          return (
            <span className="inline-flex items-center gap-1 text-xs text-emerald-400 bg-emerald-500/10 border border-emerald-500/20 px-2 py-0.5 rounded-md">
              <Wifi size={10} /> Remote
            </span>
          )
        if (j.location)
          return (
            <span className="inline-flex items-center gap-1 text-xs text-slate-400">
              <MapPin size={10} /> {j.location}
            </span>
          )
        // Empty cells render blank rather than an em-dash — per UX direction:
        // absence of data should mean field disappears, not show a placeholder.
        return null
      },
    },

    // ── 5. Salary ──────────────────────────────────────────────────────────
    {
      id: 'salary',
      size: 140,
      enableSorting: false,
      header: () => <span className="text-slate-400">Salary</span>,
      cell: ({ row }) => {
        const salary = formatSalary(row.original.salary_range)
        return salary ? <span className="text-xs text-amber-300">{salary}</span> : null
      },
    },

    // ── 6. Added ───────────────────────────────────────────────────────────
    {
      accessorKey: 'added_at',
      size: 100,
      header: ({ column }) => <DataTableColumnHeader column={column} title="Added" />,
      cell: ({ getValue }) => (
        <span className="text-xs text-slate-500">{formatRelativeTime(getValue() as string)}</span>
      ),
    },

    // ── 7. Dismiss (hover-revealed) ────────────────────────────────────────
    // Anchored to the row's right edge. Hidden by default, shown on row hover
    // or when the row is focused via keyboard. Keyboard users hit `D` on the
    // focused row instead.
    {
      id: 'dismiss',
      size: 48,
      enableSorting: false,
      header: () => null,
      cell: ({ row }) => (
        <button
          type="button"
          onClick={(e) => {
            e.stopPropagation()
            onDismiss(row.original)
          }}
          aria-label={`Dismiss ${row.original.title}`}
          title="Dismiss"
          className={cn(
            'p-1.5 rounded-md transition-all text-slate-500 hover:text-red-400 hover:bg-red-500/10',
            'opacity-0 group-hover/row:opacity-100 group-focus-within/row:opacity-100',
            'focus-visible:opacity-100 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-violet-400'
          )}
        >
          <X size={14} />
        </button>
      ),
    },
  ]
}
