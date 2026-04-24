import { useEffect, useState } from 'react'
import { X, Trash2, RefreshCw, Pencil, ChevronDown, ChevronUp } from 'lucide-react'
import type { Company, ScanStats } from '@/types'
import { fetchScanStats } from '@/api/companies'
import { getCompanyScanConfig } from '@/api/adapterConfigs'
import type { EffectiveScanConfig } from '@/api/adapterConfigs'
import { CompanyAvatar } from '@/components/shared/CompanyAvatar'
import { StatusPill, type StatusKind } from '@/components/ui/status-pill'
import { ScanHistoryBar, type HistoryPoint } from '@/components/ui/scan-history-bar'
import { cn, formatRelativeTime } from '@/lib/utils'
import { ScanIntervalOverrideModal } from '@/features/settings/components/ScanIntervalOverrideModal'
import { useScanIntervalOverride } from '@/features/settings/hooks/useScanIntervalOverride'

interface CompanyDetailDrawerProps {
  company: Company
  queueStatus?: 'queued' | 'processing'
  onClose: () => void
  onScan: (c: Company) => void
  onEdit: (c: Company) => void
  onDelete: (id: string) => void
}

export function CompanyDetailDrawer({
  company,
  queueStatus,
  onClose,
  onScan,
  onEdit,
  onDelete,
}: CompanyDetailDrawerProps) {
  const [stats, setStats] = useState<ScanStats | null>(null)
  const [loading, setLoading] = useState(false)
  const [expandedErrors, setExpandedErrors] = useState<Set<number>>(new Set())
  const [scanConfig, setScanConfig] = useState<EffectiveScanConfig | null>(null)
  const [overrideModalOpen, setOverrideModalOpen] = useState(false)
  const { setOverride } = useScanIntervalOverride(company.id)

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
    // eslint-disable-next-line react-hooks/set-state-in-effect
    setScanConfig(null)
    getCompanyScanConfig(company.id)
      .then(setScanConfig)
      .catch(() => setScanConfig(null))
  }, [company.id])

  useEffect(() => {
    function onKey(e: KeyboardEvent) {
      if (e.key === 'Escape') onClose()
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [onClose])

  const kind: StatusKind =
    queueStatus === 'processing'
      ? 'running'
      : queueStatus === 'queued'
        ? 'queued'
        : company.last_scan_status === 'ok'
          ? 'ok'
          : company.last_scan_status === 'error'
            ? 'error'
            : 'pending'

  const history: HistoryPoint[] = (stats?.recent_runs ?? [])
    .slice(0, 10)
    .reverse()
    .map((r) => (r.error ? 'err' : 'ok'))

  const successCount = history.filter((p) => p === 'ok').length
  const errorCount = history.filter((p) => p === 'err').length
  const successRate = history.length ? Math.round((successCount / history.length) * 100) : 0

  return (
    <div className="flex flex-col h-full bg-[#0d0d14]">
      {/* Header */}
      <div className="relative px-5 pt-4 pb-3 border-b border-white/[0.07]">
        <div className="absolute top-3 right-3 flex items-center gap-1">
          <button
            type="button"
            onClick={() => onDelete(company.id)}
            title="Delete"
            className="p-1.5 rounded-lg text-slate-400 hover:text-red-400 hover:bg-red-500/10 transition-colors"
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
        <div className="flex items-start gap-3 pr-16">
          <CompanyAvatar name={company.name} size="md" />
          <div className="min-w-0 flex-1">
            <p className="text-base font-semibold text-white leading-tight">{company.name}</p>
            <p className="text-sm text-slate-400 mt-0.5 truncate">
              {safeHost(company.url)} · {company.adapter_type}
            </p>
          </div>
        </div>
        <div className="flex flex-wrap gap-1.5 mt-3">
          <StatusPill status={kind} />
          <PillBadge>
            Last: {company.last_scan_at ? formatRelativeTime(company.last_scan_at) : '—'}
          </PillBadge>
          <PillBadge>Next: {formatNext(company.next_scan_at, queueStatus)}</PillBadge>
        </div>
        <div className="flex items-center gap-2 mt-3">
          <button
            type="button"
            onClick={() => onScan(company)}
            className="flex items-center gap-1.5 text-xs text-white bg-violet-600 hover:bg-violet-500 px-2.5 py-1.5 rounded-lg transition-colors"
          >
            <RefreshCw size={12} /> Scan now
          </button>
          <button
            type="button"
            onClick={() => onEdit(company)}
            className="flex items-center gap-1.5 text-xs text-slate-300 bg-white/5 border border-white/10 hover:bg-white/10 px-2.5 py-1.5 rounded-lg transition-colors"
          >
            <Pencil size={12} /> Edit
          </button>
        </div>
      </div>

      {/* Body */}
      <div className="flex-1 overflow-y-auto px-5 py-4 space-y-5">
        <Section title="Last 10 scans">
          {loading ? (
            <div className="h-4 w-24 bg-white/5 rounded animate-pulse" />
          ) : history.length ? (
            <div className="flex items-center gap-2">
              <ScanHistoryBar history={history} />
              <span className="text-xs text-slate-400">
                {successRate}% success{errorCount ? ` · ${errorCount} errors` : ''}
              </span>
            </div>
          ) : (
            <p className="text-xs text-slate-500">No scans yet</p>
          )}
        </Section>

        <Section title="Scan Interval">
          {scanConfig ? (
            <div className="space-y-1.5">
              <div className="flex items-center justify-between text-xs">
                <span className="text-slate-400">Adapter default</span>
                <span className="text-white/70">{scanConfig.effective_scan_interval_minutes} min</span>
              </div>
              <div className="flex items-center justify-between text-xs">
                <span className="text-slate-400">Custom override</span>
                {scanConfig.company_override !== null ? (
                  <div className="flex items-center gap-2">
                    <span className="text-violet-300 font-medium">{scanConfig.company_override} min</span>
                    <button
                      type="button"
                      onClick={() => void setOverride(null)}
                      title="Clear override"
                      className="text-slate-500 hover:text-red-400 transition-colors"
                    >
                      <X size={11} />
                    </button>
                  </div>
                ) : (
                  <div className="flex items-center gap-2">
                    <span className="text-slate-500">—</span>
                    <button
                      type="button"
                      onClick={() => setOverrideModalOpen(true)}
                      className="text-xs text-violet-400 hover:text-violet-300 transition-colors"
                    >
                      Set override
                    </button>
                  </div>
                )}
              </div>
            </div>
          ) : (
            <div className="h-8 bg-white/5 rounded animate-pulse" />
          )}
        </Section>

        <Section title="Activity">
          <div className="grid grid-cols-2 gap-2">
            <StatTile value={stats?.new_today ?? '—'} accent="new" label="Last 24 hours" />
            <StatTile value={stats?.new_this_month ?? '—'} accent="new" label="This month" />
          </div>
          {stats && (
            <div className="grid grid-cols-3 gap-2 mt-2">
              <StatTile value={stats.total_scans} label="Total scans" />
              <StatTile value={stats.error_count} label="Errors" />
              <StatTile
                value={stats.avg_duration_ms != null ? formatMs(stats.avg_duration_ms) : '—'}
                label="Avg time"
              />
            </div>
          )}
        </Section>

        <Section title="Recent runs">
          {loading ? (
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
          )}
        </Section>
      </div>

      {scanConfig && (
        <ScanIntervalOverrideModal
          open={overrideModalOpen}
          onClose={() => setOverrideModalOpen(false)}
          companyId={company.id}
          companyName={company.name}
          adapterDefault={scanConfig.effective_scan_interval_minutes}
          currentOverride={scanConfig.company_override}
        />
      )}
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

function StatTile({
  value,
  label,
  accent,
}: {
  value: string | number
  label: string
  accent?: 'new'
}) {
  return (
    <div className="border border-white/[0.07] rounded-lg px-3 py-2.5">
      <div className="flex items-baseline gap-1.5">
        <div className="text-lg font-semibold text-white">{value}</div>
        {accent === 'new' && <div className="text-[10px] text-emerald-400">new</div>}
      </div>
      <div className="text-[10px] uppercase tracking-wider text-slate-500 mt-0.5">{label}</div>
    </div>
  )
}

function PillBadge({ children }: { children: React.ReactNode }) {
  return (
    <span className="text-xs text-slate-400 bg-white/[0.05] border border-white/[0.07] px-2 py-0.5 rounded-md">
      {children}
    </span>
  )
}

function safeHost(url: string) {
  try {
    return new URL(url).hostname
  } catch {
    return url
  }
}

function formatNext(next: string | null, queueStatus?: 'queued' | 'processing') {
  if (queueStatus === 'processing') return 'now'
  if (queueStatus === 'queued') return 'up next'
  if (!next) return '—'
  const ms = new Date(next).getTime() - Date.now()
  if (ms <= 0) return '—'
  const mins = Math.floor(ms / 60000)
  if (mins < 60) return `in ${mins}m`
  const hrs = Math.floor(mins / 60)
  return `in ${hrs}h`
}

function formatMs(ms: number) {
  if (ms < 1000) return `${Math.round(ms)}ms`
  return `${(ms / 1000).toFixed(1)}s`
}
