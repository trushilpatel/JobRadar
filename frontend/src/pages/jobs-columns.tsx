/**
 * Column definitions for the Jobs table.
 * Separated from JobsPage to keep the page component focused on state/data-fetching.
 */
import { useRef, useEffect } from 'react'
import type { ColumnDef } from '@tanstack/react-table'
import { MapPin, Wifi } from 'lucide-react'
import type { Job, SalaryRange } from '@/types'
import { DataTableColumnHeader } from '@/components/ui/data-table'
import { formatRelativeTime, cn } from '@/lib/utils'

// ── Action callbacks injected by JobsPage ────────────────────────────────────

export interface JobTableActions {
  selectedIds: Set<string>
  filteredIds: string[]
  onToggleSelect: (id: string, checked: boolean) => void
  onSelectAll: (checked: boolean) => void
}

// ── Checkbox with indeterminate support ──────────────────────────────────────

function IndeterminateCheckbox({
  checked,
  indeterminate,
  onChange,
}: {
  checked: boolean
  indeterminate: boolean
  onChange: (checked: boolean) => void
}) {
  const ref = useRef<HTMLInputElement>(null)
  useEffect(() => {
    if (ref.current) ref.current.indeterminate = indeterminate
  }, [indeterminate])
  return (
    <input
      ref={ref}
      type="checkbox"
      checked={checked}
      onChange={e => onChange(e.target.checked)}
      className="accent-violet-500 cursor-pointer"
    />
  )
}

// ── Column definitions ───────────────────────────────────────────────────────

export function buildJobColumns(actions: JobTableActions): ColumnDef<Job, unknown>[] {
  const { selectedIds, filteredIds, onToggleSelect, onSelectAll } = actions
  const allSelected = filteredIds.length > 0 && filteredIds.every(id => selectedIds.has(id))
  const someSelected = filteredIds.some(id => selectedIds.has(id))

  return [
    // Select checkbox
    {
      id: 'select',
      size: 40,
      enableSorting: false,
      header: () => (
        <IndeterminateCheckbox
          checked={allSelected}
          indeterminate={someSelected && !allSelected}
          onChange={onSelectAll}
        />
      ),
      cell: ({ row }) => (
        <div onClick={e => e.stopPropagation()}>
          <input
            type="checkbox"
            checked={selectedIds.has(row.original.id)}
            onChange={e => onToggleSelect(row.original.id, e.target.checked)}
            className="accent-violet-500 cursor-pointer"
          />
        </div>
      ),
    },

    // Company + avatar
    {
      id: 'company',
      size: 180,
      accessorFn: row => row.company?.name ?? row.company_id ?? '',
      header: ({ column }) => <DataTableColumnHeader column={column} title="Company" />,
      cell: ({ row }) => {
        const name = row.original.company?.name ?? row.original.company_id ?? ''
        return (
          <div className="flex items-center gap-2">
            <CompanyAvatar name={name} />
            <span className="text-xs text-slate-300 truncate max-w-[120px]">{name}</span>
          </div>
        )
      },
    },

    // Title
    {
      accessorKey: 'title',
      header: ({ column }) => <DataTableColumnHeader column={column} title="Title" />,
      cell: ({ getValue }) => (
        <span className="font-medium text-white text-sm">{getValue() as string}</span>
      ),
    },

    // Location
    {
      id: 'location',
      size: 180,
      enableSorting: false,
      header: () => <span className="text-slate-400">Location</span>,
      cell: ({ row }) => {
        const j = row.original
        if (j.remote) return (
          <span className="inline-flex items-center gap-1 text-xs text-emerald-400 bg-emerald-500/10 border border-emerald-500/20 px-2 py-0.5 rounded-md">
            <Wifi size={10} /> Remote
          </span>
        )
        if (j.location) return (
          <span className="inline-flex items-center gap-1 text-xs text-slate-400">
            <MapPin size={10} /> {j.location}
          </span>
        )
        return <span className="text-xs text-slate-600">—</span>
      },
    },

    // Salary
    {
      id: 'salary',
      size: 140,
      enableSorting: false,
      header: () => <span className="text-slate-400">Salary</span>,
      cell: ({ row }) => {
        const salary = formatSalary(row.original.salary_range)
        return salary
          ? <span className="text-xs text-amber-300">{salary}</span>
          : <span className="text-xs text-slate-600">—</span>
      },
    },

    // Added
    {
      accessorKey: 'added_at',
      size: 100,
      header: ({ column }) => <DataTableColumnHeader column={column} title="Added" />,
      cell: ({ getValue }) => (
        <span className="text-xs text-slate-500">{formatRelativeTime(getValue() as string)}</span>
      ),
    },
  ]
}

// ── Cell sub-components ──────────────────────────────────────────────────────

const AVATAR_COLORS = [
  'bg-violet-600', 'bg-blue-600', 'bg-emerald-600',
  'bg-amber-600', 'bg-rose-600', 'bg-cyan-600',
  'bg-indigo-600', 'bg-teal-600',
]

function avatarColor(name: string): string {
  if (!name) return AVATAR_COLORS[0]
  let hash = 0
  for (let i = 0; i < name.length; i++) hash = name.charCodeAt(i) + ((hash << 5) - hash)
  return AVATAR_COLORS[Math.abs(hash) % AVATAR_COLORS.length]
}

function CompanyAvatar({ name }: { name: string }) {
  return (
    <div className={cn('w-7 h-7 rounded-md flex items-center justify-center text-xs font-semibold text-white shrink-0', avatarColor(name))}>
      {name ? name[0].toUpperCase() : '?'}
    </div>
  )
}

function formatSalary(salary: SalaryRange | null | undefined): string | null {
  if (!salary) return null
  const currency = salary.currency ?? '$'
  const fmt = (n: number) => n >= 1000 ? `${currency}${Math.round(n / 1000)}k` : `${currency}${n}`
  if (salary.min && salary.max) return `${fmt(salary.min)} – ${fmt(salary.max)}`
  if (salary.min) return `${fmt(salary.min)}+`
  if (salary.max) return `up to ${fmt(salary.max)}`
  return null
}
