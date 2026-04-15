import type { ColumnDef } from '@tanstack/react-table'
import { RefreshCw, Pencil, Trash2, ExternalLink } from 'lucide-react'
import type { Company } from '@/types'
import { CompanyAvatar } from '@/components/shared/CompanyAvatar'
import { Badge } from '@/components/ui/badge'
import { StatusPill, type StatusKind } from '@/components/ui/status-pill'
import { ScanHistoryBar, type HistoryPoint } from '@/components/ui/scan-history-bar'
import { cn } from '@/lib/utils'

export interface CompanyColumnActions {
  onScan: (c: Company) => void
  onEdit: (c: Company) => void
  onDelete: (id: string) => void
  onToggleActive: (c: Company) => void
  scanning: Set<string>
  queueMap: Map<string, 'queued' | 'processing'>
}

export function buildCompanyColumns(actions: CompanyColumnActions): ColumnDef<Company, unknown>[] {
  return [
    {
      accessorKey: 'name',
      size: 260,
      header: () => <span className="text-slate-500">Company</span>,
      cell: ({ row }) => {
        const c = row.original
        return (
          <div className="flex items-center gap-2.5 min-w-0">
            <CompanyAvatar name={c.name} />
            <div className="min-w-0">
              <p className="text-sm font-medium text-white truncate">{c.name}</p>
              <a
                href={c.url}
                target="_blank"
                rel="noreferrer"
                onClick={e => e.stopPropagation()}
                className="flex items-center gap-1 text-[11px] text-slate-500 hover:text-slate-300 transition-colors mt-0.5 w-fit"
              >
                {safeHost(c.url)}
                <ExternalLink size={9} />
              </a>
            </div>
          </div>
        )
      },
    },

    {
      accessorKey: 'adapter_type',
      size: 110,
      header: () => <span className="text-slate-500">Adapter</span>,
      cell: ({ getValue }) => <Badge variant="mono">{getValue() as string}</Badge>,
    },

    {
      id: 'status',
      size: 110,
      enableSorting: false,
      header: () => <span className="text-slate-500">Status</span>,
      cell: ({ row }) => {
        const c = row.original
        const q = actions.queueMap.get(c.id)
        const kind: StatusKind =
          q === 'processing' ? 'running'
          : q === 'queued'   ? 'queued'
          : c.last_scan_status === 'ok'    ? 'ok'
          : c.last_scan_status === 'error' ? 'error'
          : 'pending'
        return <StatusPill status={kind} />
      },
    },

    {
      id: 'history',
      size: 100,
      enableSorting: false,
      header: () => <span className="text-slate-500">Last 10 scans</span>,
      // TODO(backend): list endpoint doesn't return per-company history.
      // For now we render a single bar derived from last_scan_status.
      cell: ({ row }) => {
        const c = row.original
        const q = actions.queueMap.get(c.id)
        let history: HistoryPoint[] = []
        if (q === 'processing') history = ['run']
        else if (c.last_scan_status === 'ok')    history = ['ok']
        else if (c.last_scan_status === 'error') history = ['err']
        return <ScanHistoryBar history={history} />
      },
    },

    {
      accessorKey: 'last_scan_at',
      size: 110,
      header: () => <span className="text-slate-500">Last</span>,
      cell: ({ getValue }) => {
        const v = getValue() as string | null
        return <span className="text-xs text-slate-400">{v ? relativeTime(new Date(v)) : '—'}</span>
      },
    },

    {
      accessorKey: 'next_scan_at',
      size: 110,
      enableSorting: false,
      header: () => <span className="text-slate-500">Next</span>,
      cell: ({ row }) => {
        const c = row.original
        const q = actions.queueMap.get(c.id)
        if (q === 'processing') return <span className="text-xs text-blue-400">now</span>
        if (q === 'queued')     return <span className="text-xs text-amber-400">up next</span>
        if (!c.next_scan_at)    return <span className="text-xs text-slate-600">—</span>
        const ms = new Date(c.next_scan_at).getTime() - Date.now()
        if (ms <= 0) return <span className="text-xs text-slate-600">—</span>
        return <span className="text-xs text-slate-400">in {duration(ms)}</span>
      },
    },

    {
      id: 'active',
      size: 72,
      enableSorting: false,
      header: () => <span className="text-slate-500">Active</span>,
      cell: ({ row }) => {
        const c = row.original
        return (
          <button
            type="button"
            onClick={e => { e.stopPropagation(); actions.onToggleActive(c) }}
            className={cn(
              'relative inline-flex w-9 h-5 rounded-full transition-colors shrink-0',
              c.active ? 'bg-violet-600' : 'bg-white/15',
            )}
            aria-label={c.active ? 'Deactivate' : 'Activate'}
          >
            <span className={cn(
              'absolute top-0.5 w-4 h-4 bg-white rounded-full transition-transform',
              c.active ? 'translate-x-4' : 'translate-x-0.5',
            )} />
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
          <div className="flex items-center gap-0.5 justify-end" onClick={e => e.stopPropagation()}>
            <button
              type="button"
              onClick={() => actions.onScan(c)}
              disabled={isScanning}
              title={isScanning ? 'Scanning…' : 'Scan now'}
              className={cn(
                'p-1.5 rounded-md transition-colors',
                isScanning
                  ? 'text-violet-400 bg-violet-500/10'
                  : 'text-slate-500 hover:text-violet-300 hover:bg-violet-500/10 opacity-0 group-hover:opacity-100',
              )}
            >
              <RefreshCw size={14} className={isScanning ? 'animate-spin' : ''} />
            </button>
            <button
              type="button"
              onClick={() => actions.onEdit(c)}
              title="Edit"
              className="p-1.5 rounded-md text-slate-500 hover:text-white hover:bg-white/[0.06] transition-colors opacity-0 group-hover:opacity-100"
            >
              <Pencil size={14} />
            </button>
            <button
              type="button"
              onClick={() => actions.onDelete(c.id)}
              title="Delete"
              className="p-1.5 rounded-md text-slate-500 hover:text-red-400 hover:bg-red-500/10 transition-colors opacity-0 group-hover:opacity-100"
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

function safeHost(url: string) {
  try { return new URL(url).hostname } catch { return url }
}

function relativeTime(d: Date) {
  const s = Math.floor((Date.now() - d.getTime()) / 1000)
  if (s < 60)    return 'Just now'
  if (s < 3600)  return `${Math.floor(s / 60)}m ago`
  if (s < 86400) return `${Math.floor(s / 3600)}h ago`
  return `${Math.floor(s / 86400)}d ago`
}

function duration(ms: number) {
  const mins = Math.floor(ms / 60000)
  if (mins < 60) return `${mins}m`
  const hrs = Math.floor(mins / 60)
  const rem = mins % 60
  return rem > 0 ? `${hrs}h ${rem}m` : `${hrs}h`
}
