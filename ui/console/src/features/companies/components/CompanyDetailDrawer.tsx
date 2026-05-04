import { useEffect, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import * as Dialog from '@radix-ui/react-dialog'
import { X, Trash2, RefreshCw, Pencil, ChevronDown, ChevronUp } from 'lucide-react'
import type { Company, ScanStats } from '@/types'
import { fetchScanStats } from '@/api/companies'
import { CompanyAvatar } from '@/components/shared/CompanyAvatar'
import { ScanHistoryBar, type HistoryPoint } from '@/components/ui/scan-history-bar'
import { cn, formatRelativeTime } from '@/lib/utils'

interface CompanyDetailDrawerProps {
  company: Company
  onClose: () => void
  onScan: (c: Company) => void
  onEdit: (c: Company) => void
  onDelete: (id: string) => void
}

export function CompanyDetailDrawer({
  company,
  onClose,
  onScan,
  onEdit,
  onDelete,
}: CompanyDetailDrawerProps) {
  const [stats, setStats] = useState<ScanStats | null>(null)
  const [loading, setLoading] = useState(false)
  const [expandedErrors, setExpandedErrors] = useState<Set<number>>(new Set())
  const [logsOpen, setLogsOpen] = useState(false)
  const [deleteConfirmOpen, setDeleteConfirmOpen] = useState(false)
  const navigate = useNavigate()

  function toggleError(id: number) {
    setExpandedErrors((prev) => {
      const next = new Set(prev)
      if (next.has(id)) next.delete(id)
      else next.add(id)
      return next
    })
  }

  useEffect(() => {
    // eslint-disable-next-line react-hooks/set-state-in-effect
    setStats(null)
    setLoading(true)
    fetchScanStats(company.id)
      .then(setStats)
      .catch(() => setStats(null))
      .finally(() => setLoading(false))
  }, [company.id])

  useEffect(() => {
    function onKey(e: KeyboardEvent) {
      if (e.key === 'Escape') onClose()
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [onClose])

  const history: HistoryPoint[] = (stats?.recent_runs ?? [])
    .slice(0, 10)
    .reverse()
    .map((r) => (r.error ? 'err' : 'ok'))

  const successCount = history.filter((p) => p === 'ok').length
  const successRate = history.length ? Math.round((successCount / history.length) * 100) : 0

  return (
    <div className="flex flex-col h-full bg-[#0d0d14]">
      {/* Header */}
      <div className="relative px-5 pt-4 pb-3 border-b border-white/[0.07]">
        <div className="absolute top-3 right-3 flex items-center gap-1">
          <button
            type="button"
            onClick={() => onScan(company)}
            title="Scan now"
            className="flex items-center gap-1.5 text-xs text-white bg-violet-600 hover:bg-violet-500 px-2.5 py-1.5 rounded-lg transition-colors"
          >
            <RefreshCw size={12} /> Scan now
          </button>
          <button
            type="button"
            onClick={() => onEdit(company)}
            title="Edit"
            className="p-1.5 rounded-lg text-slate-400 hover:text-white hover:bg-white/[0.06] transition-colors"
          >
            <Pencil size={14} />
          </button>
          <button
            type="button"
            onClick={() => setDeleteConfirmOpen(true)}
            title="Delete"
            className="p-1.5 rounded-lg text-slate-400 hover:text-red-400 hover:bg-white/[0.06] transition-colors"
          >
            <Trash2 size={14} />
          </button>
          <button
            type="button"
            onClick={onClose}
            title="Close"
            className="p-1.5 rounded-lg text-slate-400 hover:text-white hover:bg-white/[0.06] transition-colors"
          >
            <X size={14} />
          </button>
        </div>
        <div className="flex items-start gap-3 pr-36">
          <CompanyAvatar name={company.name} size="md" />
          <div className="min-w-0 flex-1">
            <p className="text-base font-semibold text-white leading-tight">{company.name}</p>
            <p className="text-sm text-slate-400 mt-0.5 truncate">
              <a
                href={company.url}
                target="_blank"
                rel="noopener noreferrer"
                className="hover:text-white transition-colors"
              >
                {safeHost(company.url)}
              </a>
              {' · '}{company.adapter_type}
            </p>
          </div>
        </div>
      </div>

      {/* Body */}
      <div className="flex-1 overflow-y-auto px-5 py-4 space-y-5">
        {/* Scan status zone — tonal background */}
        <div className="bg-white/[0.03] rounded-lg px-3 py-3 space-y-2">
          {loading ? (
            <div className="h-10 bg-white/5 rounded animate-pulse" />
          ) : history.length ? (
            <>
              <span className="text-[10px] font-medium text-slate-500 uppercase tracking-wider">Last 10 scans</span>
              <div className="flex items-center gap-2 w-full">
                <ScanHistoryBar history={history} />
                <span className="text-xs text-slate-400 shrink-0">{successRate}%</span>
              </div>
              <div className="flex items-center gap-3 text-xs text-slate-500">
                <span>Last scan {company.last_scan_at ? formatRelativeTime(company.last_scan_at) : '—'}</span>
                {stats?.avg_duration_ms != null && (
                  <span>· avg {formatMs(stats.avg_duration_ms)}</span>
                )}
              </div>
            </>
          ) : (
            <span className="text-xs text-slate-500">No scans yet</span>
          )}
        </div>

        <Section title="New jobs">
          <div className="grid grid-cols-3 gap-2">
            <StatTile value={stats?.new_today ?? '—'} label="Today" />
            <StatTile value={stats?.new_this_month ?? '—'} label="30 days" />
            <StatTile value={stats?.total_new ?? '—'} label="All time" />
          </div>
          {stats && stats.total_new > 0 && (
            <button
              type="button"
              onClick={() => navigate('/jobs', { state: { companies: [company.name] } })}
              className="mt-2 w-full flex items-center justify-between px-3 py-2 border border-white/[0.07] rounded-lg text-xs text-slate-300 hover:text-white hover:border-white/20 transition-colors"
            >
              <span>View {stats.total_new} new jobs</span>
              <span className="text-slate-500">→</span>
            </button>
          )}
        </Section>

        {/* Recent run logs — collapsible */}
        <div>
          <button
            type="button"
            onClick={() => setLogsOpen((v) => !v)}
            className="flex items-center justify-between w-full mb-2 group"
          >
            <h3 className="text-[10px] font-medium text-slate-500 uppercase tracking-wider group-hover:text-slate-400 transition-colors">
              Recent run logs
            </h3>
            {logsOpen ? (
              <ChevronUp size={12} className="text-slate-500 group-hover:text-slate-400 transition-colors" />
            ) : (
              <ChevronDown size={12} className="text-slate-500 group-hover:text-slate-400 transition-colors" />
            )}
          </button>
          {logsOpen && (
            loading ? (
              <div className="space-y-1">
                <div className="h-4 bg-white/5 rounded animate-pulse" />
                <div className="h-4 bg-white/5 rounded animate-pulse" />
              </div>
            ) : (stats?.recent_runs ?? []).length === 0 ? (
              <p className="text-xs text-slate-500">No runs yet</p>
            ) : (
              <div className="text-xs">
                {(stats?.recent_runs ?? []).slice(0, 10).map((r) => (
                  <div key={r.id} className="border-b border-white/[0.04]">
                    <div
                      className={cn(
                        'flex items-center justify-between py-1.5',
                        r.error && 'cursor-pointer'
                      )}
                      onClick={() => r.error && toggleError(r.id)}
                    >
                      <span className="text-slate-300 flex items-center gap-1.5">
                        {formatRelativeTime(r.scanned_at)}
                        {r.duration_ms != null && (
                          <span className="text-slate-600">{formatMs(r.duration_ms)}</span>
                        )}
                      </span>
                      <span
                        className={cn(
                          'flex items-center gap-1',
                          r.error ? 'text-red-400' : 'text-emerald-400'
                        )}
                      >
                        {r.error ? (
                          <>
                            Error
                            {expandedErrors.has(r.id) ? (
                              <ChevronUp size={10} />
                            ) : (
                              <ChevronDown size={10} />
                            )}
                          </>
                        ) : (
                          `OK · ${r.jobs_fetched} fetched${r.jobs_new ? ` · ${r.jobs_new} new` : ''}`
                        )}
                      </span>
                    </div>
                    {r.error && expandedErrors.has(r.id) && (
                      <div className="pb-2">
                        <p className="text-[11px] text-red-300 font-mono bg-red-950/20 rounded p-2 break-all leading-relaxed">
                          {r.error}
                        </p>
                      </div>
                    )}
                  </div>
                ))}
              </div>
            )
          )}
        </div>
      </div>

      <Dialog.Root open={deleteConfirmOpen} onOpenChange={setDeleteConfirmOpen}>
        <Dialog.Portal>
          <Dialog.Overlay className="fixed inset-0 bg-black/60 z-50" />
          <Dialog.Content className="fixed left-1/2 top-1/2 -translate-x-1/2 -translate-y-1/2 z-50 w-full max-w-sm bg-[#13131a] border border-white/10 rounded-xl shadow-2xl p-6">
            <Dialog.Title className="text-base font-semibold text-white mb-2">Delete company?</Dialog.Title>
            <Dialog.Description className="text-sm text-slate-400 mb-5">
              This will permanently delete {company.name} and all its scan history. This cannot be undone.
            </Dialog.Description>
            <div className="flex gap-3">
              <button
                type="button"
                onClick={() => setDeleteConfirmOpen(false)}
                className="flex-1 px-4 py-2 rounded-lg text-sm font-medium bg-white/5 hover:bg-white/10 text-slate-300 transition-colors"
              >
                Cancel
              </button>
              <button
                type="button"
                onClick={() => { setDeleteConfirmOpen(false); onDelete(company.id) }}
                className="flex-1 px-4 py-2 rounded-lg text-sm font-medium bg-red-600 hover:bg-red-500 text-white transition-colors"
              >
                Delete
              </button>
            </div>
          </Dialog.Content>
        </Dialog.Portal>
      </Dialog.Root>

    </div>
  )
}

// ── helpers ────────────────────────────────────────────────────────────────

function Section({
  title,
  aside,
  children,
}: {
  title: string
  aside?: string
  children: React.ReactNode
}) {
  return (
    <div>
      <div className="flex items-center justify-between mb-2">
        <h3 className="text-[10px] font-medium text-slate-500 uppercase tracking-wider">{title}</h3>
        {aside && <span className="text-[10px] text-slate-500">{aside}</span>}
      </div>
      {children}
    </div>
  )
}

function StatTile({ value, label }: { value: string | number; label: string }) {
  return (
    <div className="border border-white/[0.07] rounded-lg px-3 py-2.5">
      <div className="text-lg font-semibold text-white">{value}</div>
      <div className="text-[10px] uppercase tracking-wider text-slate-500 mt-0.5">{label}</div>
    </div>
  )
}

function safeHost(url: string) {
  try {
    return new URL(url).hostname
  } catch {
    return url
  }
}


function formatMs(ms: number) {
  if (ms < 1000) return `${Math.round(ms)}ms`
  return `${(ms / 1000).toFixed(1)}s`
}
