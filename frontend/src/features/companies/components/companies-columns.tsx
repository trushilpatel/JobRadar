import type { ColumnDef } from '@tanstack/react-table'
import { RefreshCw, Pencil, Trash2, ExternalLink } from 'lucide-react'
import * as Tooltip from '@radix-ui/react-tooltip'
import type { Company, ATS } from '@/types'
import { QUEUE_STATUS, SCAN_STATUS } from '../constants'
import { CompanyAvatar } from '@/components/shared/CompanyAvatar'
import { Badge } from '@/components/ui/badge'
import { StatusPill, type StatusKind } from '@/components/ui/status-pill'
import { ScanHistoryBar, type HistoryPoint } from '@/components/ui/scan-history-bar'
import { DataTableColumnHeader } from '@/components/ui/data-table'
import { cn } from '@/lib/utils/utils'

const HISTORY_SLOTS = 10

export interface CompanyColumnActions {
  onScan: (c: Company) => void
  onEdit: (c: Company) => void
  onDelete: (id: string) => void
  onToggleActive: (c: Company) => void
  scanning: Set<string>
  // Per-company in-flight set for the active/inactive toggle. Optional so the
  // table can render without wiring it; absence is treated as "not toggling".
  togglingActive?: Set<string>
  // Optional ATS display name map — keyed by adapter name. When provided,
  // the adapter_type cell renders the display name. Falls back to raw string.
  atsMap?: Map<string, ATS>
}

export function buildCompanyColumns(actions: CompanyColumnActions): ColumnDef<Company, unknown>[] {
  return [
    {
      accessorKey: 'name',
      size: 260,
      header: ({ column }) => <DataTableColumnHeader column={column} title="Company" />,
      cell: ({ row }) => {
        const c = row.original
        return (
          <div className="flex items-center gap-2.5 min-w-0">
            <CompanyAvatar name={c.name} />
            <div className="min-w-0 flex items-center gap-1.5">
              <p className="text-sm font-medium text-white truncate">{c.name}</p>
              <a
                href={c.url}
                target="_blank"
                rel="noreferrer"
                onClick={(e) => e.stopPropagation()}
                className="text-slate-500 hover:text-slate-300 transition-colors opacity-0 group-hover:opacity-100 shrink-0"
              >
                <ExternalLink size={10} />
              </a>
            </div>
          </div>
        )
      },
    },

    {
      accessorKey: 'adapter_type',
      size: 110,
      header: ({ column }) => <DataTableColumnHeader column={column} title="Adapter" />,
      cell: ({ getValue }) => {
        const raw = getValue() as string
        const display = actions.atsMap?.get(raw)?.display_name
        return <Badge variant="mono">{display ?? raw}</Badge>
      },
    },

    {
      id: 'status',
      size: 110,
      enableSorting: false,
      header: () => <span className="text-slate-400">Status</span>,
      cell: ({ row }) => {
        const c = row.original
        const kind: StatusKind =
          c.last_scan_status === QUEUE_STATUS.RUNNING
            ? 'running'
            : c.last_scan_status === QUEUE_STATUS.QUEUED
              ? 'queued'
              : c.last_scan_status === SCAN_STATUS.OK
                ? 'ok'
                : c.last_scan_status === SCAN_STATUS.ERROR
                  ? 'error'
                  : 'pending'
        return <StatusPill status={kind} />
      },
    },

    {
      id: 'history',
      size: 100,
      enableSorting: false,
      header: () => (
        <Tooltip.Provider delayDuration={200}>
          <Tooltip.Root>
            <Tooltip.Trigger asChild>
              <span className="text-slate-400 cursor-help">Last 10 scans</span>
            </Tooltip.Trigger>
            <Tooltip.Portal>
              <Tooltip.Content
                className="z-50 bg-slate-800 border border-white/10 text-xs text-slate-300 px-2.5 py-1.5 rounded-lg shadow-lg"
                sideOffset={4}
              >
                green = ok · amber = running · red = error · grey = no data
                <Tooltip.Arrow className="fill-slate-800" />
              </Tooltip.Content>
            </Tooltip.Portal>
          </Tooltip.Root>
        </Tooltip.Provider>
      ),
      cell: ({ row }) => {
        const c = row.original

        // Real history from scan_history field (oldest → newest).
        const history: HistoryPoint[] = (c.scan_history ?? []).map((ok) => (ok ? 'ok' : 'err'))

        // Pad to HISTORY_SLOTS with idle on the left.
        const padded: HistoryPoint[] = Array<HistoryPoint>(
          Math.max(0, HISTORY_SLOTS - history.length)
        )
          .fill('idle')
          .concat(history)

        // Override the rightmost slot if currently running.
        if (c.last_scan_status === QUEUE_STATUS.RUNNING && padded.length > 0) {
          padded[padded.length - 1] = 'run'
        }

        return <ScanHistoryBar history={padded} />
      },
    },

    {
      accessorKey: 'last_scan_at',
      size: 110,
      header: ({ column }) => <DataTableColumnHeader column={column} title="Last scan" />,
      cell: ({ getValue }) => {
        const v = getValue() as string | null
        return <span className="text-xs text-slate-400">{v ? relativeTime(new Date(v)) : '—'}</span>
      },
    },

    {
      id: 'active',
      size: 72,
      enableSorting: false,
      header: () => <span className="text-slate-400">Active</span>,
      cell: ({ row }) => {
        const c = row.original
        const isToggling = actions.togglingActive?.has(c.id) ?? false
        return (
          <button
            type="button"
            role="switch"
            aria-checked={c.active}
            disabled={isToggling}
            onClick={(e) => {
              e.stopPropagation()
              actions.onToggleActive(c)
            }}
            className={cn(
              'relative inline-flex w-9 h-5 rounded-full transition-colors shrink-0',
              c.active ? 'bg-violet-600' : 'bg-white/15',
              isToggling && 'opacity-50 cursor-not-allowed'
            )}
            aria-label={c.active ? 'Deactivate' : 'Activate'}
          >
            <span
              className={cn(
                'absolute top-0.5 w-4 h-4 bg-white rounded-full transition-transform',
                c.active ? 'translate-x-4' : 'translate-x-0.5'
              )}
            />
          </button>
        )
      },
    },

    {
      id: 'actions',
      size: 100,
      enableSorting: false,
      header: () => null,
      cell: ({ row }) => {
        const c = row.original
        const isScanning = actions.scanning.has(c.id)
        return (
          <div
            className="flex items-center gap-0.5 justify-end"
            onClick={(e) => e.stopPropagation()}
          >
            <button
              type="button"
              onClick={() => actions.onScan(c)}
              disabled={isScanning}
              title={isScanning ? 'Scanning…' : 'Scan now'}
              aria-label={isScanning ? `Scanning ${c.name}` : `Scan ${c.name} now`}
              className={cn(
                'p-1.5 rounded-md transition-colors',
                isScanning
                  ? 'text-violet-400 bg-violet-500/10'
                  : 'text-slate-500 hover:text-violet-300 hover:bg-violet-500/10 opacity-0 group-hover:opacity-100 focus-within:opacity-100'
              )}
            >
              <RefreshCw size={14} className={isScanning ? 'animate-spin' : ''} />
            </button>
            <button
              type="button"
              onClick={() => actions.onEdit(c)}
              title="Edit"
              aria-label={`Edit ${c.name}`}
              className="p-1.5 rounded-md text-slate-500 hover:text-white hover:bg-white/[0.06] transition-colors opacity-0 group-hover:opacity-100 focus-within:opacity-100"
            >
              <Pencil size={14} />
            </button>
            <button
              type="button"
              onClick={() => actions.onDelete(c.id)}
              title="Delete"
              aria-label={`Delete ${c.name}`}
              className="p-1.5 rounded-md text-slate-500 hover:text-red-400 hover:bg-red-500/10 transition-colors opacity-0 group-hover:opacity-100 focus-within:opacity-100"
            >
              <Trash2 size={14} />
            </button>
          </div>
        )
      },
    },
  ]
}

// ── helpers ────────────────────────────────────────────────────────────────

function relativeTime(d: Date) {
  const s = Math.floor((Date.now() - d.getTime()) / 1000)
  if (s < 60) return 'Just now'
  if (s < 3600) return `${Math.floor(s / 60)}m ago`
  if (s < 86400) return `${Math.floor(s / 3600)}h ago`
  return `${Math.floor(s / 86400)}d ago`
}
