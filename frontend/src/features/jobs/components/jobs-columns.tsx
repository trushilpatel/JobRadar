import { useRef, useEffect } from 'react'
import type { ColumnDef } from '@tanstack/react-table'
import { MapPin, Wifi } from 'lucide-react'
import type { Job } from '@/types'
import { DataTableColumnHeader } from '@/components/ui/data-table'
import { formatRelativeTime } from '@/lib/utils'
import { CompanyAvatar } from '@/components/shared/CompanyAvatar'
import { formatSalary } from '@/services/jobs.service'

export interface JobTableActions {
  selectedIds: Set<string>
  filteredIds: string[]
  onToggleSelect: (id: string, checked: boolean) => void
  onSelectAll: (checked: boolean) => void
}

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

export function buildJobColumns(actions: JobTableActions): ColumnDef<Job, unknown>[] {
  const { selectedIds, filteredIds, onToggleSelect, onSelectAll } = actions
  const allSelected = filteredIds.length > 0 && filteredIds.every(id => selectedIds.has(id))
  const someSelected = filteredIds.some(id => selectedIds.has(id))

  return [
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
    {
      id: 'company',
      size: 180,
      accessorFn: row => row.company?.name ?? row.company_id ?? '',
      header: ({ column }) => <DataTableColumnHeader column={column} title="Company" />,
      cell: ({ row }) => {
        const name = row.original.company?.name ?? row.original.company_id ?? ''
        return (
          <div className="flex items-center gap-2">
            <CompanyAvatar name={name} size="sm" />
            <span className="text-xs text-slate-300 truncate max-w-[120px]">{name}</span>
          </div>
        )
      },
    },
    {
      accessorKey: 'title',
      header: ({ column }) => <DataTableColumnHeader column={column} title="Title" />,
      cell: ({ getValue }) => (
        <span className="font-medium text-white text-sm">{getValue() as string}</span>
      ),
    },
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
        return <span className="text-xs text-slate-600">—</span>
      },
    },
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
    {
      accessorKey: 'added_at',
      size: 100,
      header: ({ column }) => <DataTableColumnHeader column={column} title="Added" />,
      cell: ({ getValue }) => (
        <span className="text-xs text-slate-500">
          {formatRelativeTime(getValue() as string)}
        </span>
      ),
    },
  ]
}
