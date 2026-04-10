/**
 * Column definitions for the Companies table.
 * Kept separate from CompaniesPage so the page component stays focused on
 * state/data-fetching, not rendering details.
 */
import type { ColumnDef } from '@tanstack/react-table'
import { RefreshCw, CheckCircle2, XCircle, Clock, Minus, ExternalLink, Pencil, Trash2 } from 'lucide-react'
import type { Company } from '@/types'
import { DataTableColumnHeader } from '@/components/ui/data-table'
import { Badge } from '@/components/ui/badge'
import { cn } from '@/lib/utils'

// ── Action callbacks injected by CompaniesPage ───────────────────────────────

export interface CompanyTableActions {
  onScan:   (company: Company) => void
  onEdit:   (company: Company) => void
  onDelete: (id: string) => void
  onToggleActive: (company: Company) => void
  scanning: Set<string>
  queueMap: Map<string, 'queued' | 'processing'>
}

// ── Column definitions ───────────────────────────────────────────────────────

export function buildCompanyColumns(actions: CompanyTableActions): ColumnDef<Company, unknown>[] {
  return [
    // Company name + URL
    {
      accessorKey: 'name',
      size: 260,
      header: ({ column }) => <DataTableColumnHeader column={column} title="Company" />,
      cell: ({ row }) => {
        const c = row.original
        return (
          <div>
            <p className="text-sm font-medium text-white group-hover:text-violet-300 transition-colors">
              {c.name}
            </p>
            <a
              href={c.url}
              target="_blank"
              rel="noreferrer"
              onClick={e => e.stopPropagation()}
              className="flex items-center gap-1 text-xs text-slate-500 hover:text-slate-300 transition-colors mt-0.5 w-fit"
            >
              {safeHostname(c.url)}
              <ExternalLink size={10} />
            </a>
          </div>
        )
      },
    },

    // Adapter type
    {
      accessorKey: 'adapter_type',
      size: 120,
      header: ({ column }) => <DataTableColumnHeader column={column} title="Adapter" />,
      cell: ({ getValue }) => (
        <Badge variant="mono">{getValue() as string}</Badge>
      ),
    },

    // Queue-aware status
    {
      id: 'status',
      accessorKey: 'last_scan_status',
      size: 110,
      header: ({ column }) => <DataTableColumnHeader column={column} title="Status" />,
      cell: ({ row }) => {
        const queueStatus = actions.queueMap.get(row.original.id)
        return <QueueAwareStatus scanStatus={row.original.last_scan_status} queueStatus={queueStatus} />
      },
    },

    // Last scan
    {
      accessorKey: 'last_scan_at',
      size: 120,
      header: ({ column }) => <DataTableColumnHeader column={column} title="Last Scan" />,
      cell: ({ getValue }) => {
        const v = getValue() as string | null
        return (
          <span className="text-sm text-slate-400">{v ? relativeTime(new Date(v)) : 'Never'}</span>
        )
      },
    },

    // Next scan
    {
      accessorKey: 'next_scan_at',
      size: 130,
      enableSorting: false,
      header: () => <span className="text-slate-400">Next Scan</span>,
      cell: ({ row }) => {
        const queueStatus = actions.queueMap.get(row.original.id)
        return <NextScanCell nextScanAt={row.original.next_scan_at} queueStatus={queueStatus} />
      },
    },

    // Active toggle
    {
      id: 'active',
      size: 72,
      enableSorting: false,
      header: () => <span className="text-slate-400">Active</span>,
      cell: ({ row }) => {
        const c = row.original
        return (
          <button
            onClick={e => { e.stopPropagation(); actions.onToggleActive(c) }}
            className={cn(
              'relative inline-flex w-10 h-6 rounded-full transition-colors duration-200 focus:outline-none shrink-0',
              c.active ? 'bg-violet-600' : 'bg-white/15'
            )}
          >
            <span className={cn(
              'absolute top-1 w-4 h-4 bg-white rounded-full shadow transition-transform duration-200',
              c.active ? 'translate-x-5' : 'translate-x-1'
            )} />
          </button>
        )
      },
    },

    // Row actions
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
            className="flex items-center gap-1 justify-end"
            onClick={e => e.stopPropagation()}
          >
            <button
              onClick={() => actions.onScan(c)}
              disabled={isScanning}
              title={isScanning ? 'Scanning…' : 'Scan now'}
              className={cn(
                'p-1.5 rounded-md transition-colors',
                isScanning
                  ? 'text-violet-400 bg-violet-500/10'
                  : 'text-slate-600 hover:text-violet-300 hover:bg-violet-500/10 opacity-0 group-hover:opacity-100'
              )}
            >
              <RefreshCw size={14} className={isScanning ? 'animate-spin' : ''} />
            </button>
            <button
              onClick={() => actions.onEdit(c)}
              title="Edit"
              className="p-1.5 text-slate-600 hover:text-white hover:bg-white/[0.06] rounded-md transition-colors opacity-0 group-hover:opacity-100"
            >
              <Pencil size={14} />
            </button>
            <button
              onClick={() => actions.onDelete(c.id)}
              title="Delete"
              className="p-1.5 text-slate-600 hover:text-red-400 hover:bg-red-500/10 rounded-md transition-colors opacity-0 group-hover:opacity-100"
            >
              <Trash2 size={14} />
            </button>
          </div>
        )
      },
    },
  ]
}

// ── Cell sub-components ──────────────────────────────────────────────────────

function QueueAwareStatus({
  scanStatus,
  queueStatus,
}: {
  scanStatus: Company['last_scan_status']
  queueStatus: 'queued' | 'processing' | undefined
}) {
  if (queueStatus === 'processing') return <Badge variant="info"><RefreshCw size={10} className="animate-spin" /> Running</Badge>
  if (queueStatus === 'queued')     return <Badge variant="warning">· Queued</Badge>

  if (!scanStatus) return <Badge variant="ghost"><Minus size={10} /> Never</Badge>

  const map = {
    ok:      { icon: <CheckCircle2 size={10} />, label: 'OK',      variant: 'success'  },
    error:   { icon: <XCircle size={10} />,      label: 'Error',   variant: 'error'    },
    pending: { icon: <Clock size={10} />,         label: 'Pending', variant: 'warning'  },
  } as const

  const { icon, label, variant } = map[scanStatus]
  return <Badge variant={variant}>{icon} {label}</Badge>
}

function NextScanCell({
  nextScanAt,
  queueStatus,
}: {
  nextScanAt: string | null
  queueStatus: 'queued' | 'processing' | undefined
}) {
  if (queueStatus === 'processing') return <span className="text-xs text-blue-400">Now</span>
  if (queueStatus === 'queued')     return <span className="text-xs text-amber-400">Up next</span>

  if (!nextScanAt) return <span className="text-xs text-slate-600">—</span>

  const diffMs = new Date(nextScanAt).getTime() - Date.now()
  if (diffMs <= 0) return <span className="text-xs text-slate-600">—</span>

  return <span className="text-xs text-slate-400">in {durationFromMs(diffMs)}</span>
}

// ── Helpers ──────────────────────────────────────────────────────────────────

function safeHostname(url: string) {
  try { return new URL(url).hostname } catch { return url }
}

function relativeTime(date: Date): string {
  const secs = Math.floor((Date.now() - date.getTime()) / 1000)
  if (secs < 60)    return 'Just now'
  if (secs < 3600)  return `${Math.floor(secs / 60)}m ago`
  if (secs < 86400) return `${Math.floor(secs / 3600)}h ago`
  return `${Math.floor(secs / 86400)}d ago`
}

function durationFromMs(ms: number): string {
  const mins = Math.floor(ms / 60000)
  if (mins < 60) return `${mins}m`
  const hrs = Math.floor(mins / 60)
  const rem = mins % 60
  return rem > 0 ? `${hrs}h ${rem}m` : `${hrs}h`
}
