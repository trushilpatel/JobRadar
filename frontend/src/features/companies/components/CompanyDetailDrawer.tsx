import { useState } from 'react'
import * as Dialog from '@radix-ui/react-dialog'
import { X, Trash2, RefreshCw, Pencil, ChevronDown, ChevronUp, AlertTriangle } from 'lucide-react'
import {
  AreaChart,
  Area,
  XAxis,
  YAxis,
  Tooltip as RechartTooltip,
  ResponsiveContainer,
} from 'recharts'
import type { Company, ATS } from '@/types'
import { CompanyAvatar } from '@/components/shared/CompanyAvatar'
import { ScanHistoryBar, type HistoryPoint } from '@/components/ui/scan-history-bar'
import { cn, formatRelativeTime } from '@/lib/utils/utils'
import { useCompanyScanStats } from '../hooks/useCompanyScanStats'
import { useCompanyAnalytics } from '../hooks/useCompanyAnalytics'

interface CompanyDetailDrawerProps {
  company: Company
  onClose: () => void
  onScan: (c: Company) => void
  onEdit: (c: Company) => void
  onDelete: (id: string) => void
  onViewJobs?: (companyName: string) => void
  /** Optional ATS entry for this company — used to display human-readable adapter name. */
  atsEntry?: ATS
}

export function CompanyDetailDrawer({
  company,
  onClose,
  onScan,
  onEdit,
  onDelete,
  onViewJobs,
  atsEntry,
}: CompanyDetailDrawerProps) {
  const [expandedErrors, setExpandedErrors] = useState<Set<string>>(new Set())
  const [logsOpen, setLogsOpen] = useState(false)
  const [deleteConfirmOpen, setDeleteConfirmOpen] = useState(false)

  const { data: stats, isLoading } = useCompanyScanStats(company.id)
  const { data: analytics, isError: analyticsError } = useCompanyAnalytics(company.id)

  function toggleError(id: string) {
    setExpandedErrors((prev) => {
      const next = new Set(prev)
      if (next.has(id)) next.delete(id)
      else next.add(id)
      return next
    })
  }

  const history: HistoryPoint[] = (() => {
    const runs = (stats?.recent_runs ?? [])
      .slice(0, 10)
      .reverse()
      .map<HistoryPoint>((r) => (r.error ? 'err' : 'ok'))
    const padding: HistoryPoint[] = Array<HistoryPoint>(Math.max(0, 10 - runs.length)).fill('idle')
    return [...padding, ...runs]
  })()

  const successCount = history.filter((p) => p === 'ok').length
  const scannedCount = history.filter((p) => p !== 'idle').length
  const successRate = scannedCount ? Math.round((successCount / scannedCount) * 100) : 0

  // Persistent failure state from company_scan_settings — distinct from the
  // per-run scan logs below. Only surfaced while the company is actively failing
  // (consecutive_errors > 0); resets to 0 on the next successful scan.
  const settings = company.scan_settings
  const persistentError =
    settings && settings.consecutive_errors > 0
      ? {
          consecutive_errors: settings.consecutive_errors,
          last_error_message: settings.last_error_message,
          last_error_at: settings.last_error_at,
        }
      : null

  return (
    <div className="flex flex-col h-full bg-panel">
      {/* Header */}
      <div className="relative px-5 pt-4 pb-3 border-b border-[var(--border)]">
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
            className="p-1.5 rounded-lg text-muted hover:text-primary hover:bg-hover transition-colors"
          >
            <Pencil size={14} />
          </button>
          <button
            type="button"
            onClick={() => setDeleteConfirmOpen(true)}
            title="Delete"
            className="p-1.5 rounded-lg text-muted hover:text-red-400 hover:bg-hover transition-colors"
          >
            <Trash2 size={14} />
          </button>
          <button
            type="button"
            onClick={onClose}
            title="Close"
            aria-label="Close drawer"
            className="p-1.5 rounded-lg text-muted hover:text-primary hover:bg-hover transition-colors"
          >
            <X size={14} />
          </button>
        </div>
        <div className="flex items-start gap-3 pr-36">
          <CompanyAvatar name={company.name} size="md" />
          <div className="min-w-0 flex-1">
            <p className="text-base font-semibold text-primary leading-tight">{company.name}</p>
            <p className="text-sm text-muted mt-0.5 truncate">
              <a
                href={company.url}
                target="_blank"
                rel="noopener noreferrer"
                className="hover:text-primary transition-colors"
              >
                {safeHost(company.url)}
              </a>
              {' · '}
              {atsEntry?.display_name ?? company.adapter_type}
            </p>
          </div>
        </div>
      </div>

      {/* Body */}
      <div className="flex-1 overflow-y-auto px-5 py-4 space-y-5">
        {persistentError && (
          <div
            role="alert"
            className="rounded-lg border border-red-500/30 bg-red-950/20 px-3 py-2.5"
          >
            <div className="flex items-center gap-1.5 text-red-300">
              <AlertTriangle size={13} />
              <span className="text-xs font-semibold">
                Scan failing
                {persistentError.consecutive_errors > 1
                  ? ` · ${persistentError.consecutive_errors} consecutive errors`
                  : ''}
              </span>
              {persistentError.last_error_at && (
                <span className="ml-auto text-[10px] text-red-400/70">
                  {formatRelativeTime(persistentError.last_error_at)}
                </span>
              )}
            </div>
            {persistentError.last_error_message && (
              <p className="mt-1.5 text-[11px] text-red-300/90 font-mono break-all leading-relaxed">
                {persistentError.last_error_message}
              </p>
            )}
          </div>
        )}

        <Section title="Recent scans">
          <div className="flex gap-2">
            <div className="flex-1 border border-[var(--border)] rounded-lg px-3 py-2.5 flex flex-col justify-between gap-2">
              {isLoading ? (
                <div className="h-4 bg-white/5 rounded animate-pulse" />
              ) : (
                <>
                  <ScanHistoryBar history={history} className="w-full justify-between" />
                  <span className="text-[10px] uppercase tracking-wider text-muted">
                    {successRate}% success
                  </span>
                </>
              )}
            </div>
            <div className="border border-[var(--border)] rounded-lg px-3 py-2.5 flex flex-col justify-between shrink-0 w-24">
              <div className="text-sm font-semibold text-primary">
                {company.last_scan_at ? formatRelativeTime(company.last_scan_at) : '—'}
              </div>
              <div className="text-[10px] uppercase tracking-wider text-muted">Last scan</div>
            </div>
            <div className="border border-[var(--border)] rounded-lg px-3 py-2.5 flex flex-col justify-between shrink-0 w-20">
              <div className="text-sm font-semibold text-primary">
                {isLoading
                  ? '—'
                  : stats?.avg_duration_ms != null
                    ? formatMs(stats.avg_duration_ms)
                    : '—'}
              </div>
              <div className="text-[10px] uppercase tracking-wider text-muted">Avg time</div>
            </div>
          </div>
          <button
            type="button"
            onClick={() => setLogsOpen((v) => !v)}
            className="mt-2 w-full flex items-center justify-between px-3 py-2 border border-[var(--border)] rounded-lg text-xs text-muted hover:text-primary hover:border-[var(--border-strong)] transition-colors"
          >
            <span>Logs</span>
            {logsOpen ? <ChevronUp size={12} /> : <ChevronDown size={12} />}
          </button>
          {logsOpen &&
            (isLoading ? (
              <div className="mt-2 space-y-1">
                <div className="h-4 bg-white/5 rounded animate-pulse" />
                <div className="h-4 bg-white/5 rounded animate-pulse" />
              </div>
            ) : (stats?.recent_runs ?? []).length === 0 ? (
              <p className="mt-2 text-xs text-muted">No runs yet</p>
            ) : (
              <div className="mt-2 text-xs">
                {(stats?.recent_runs ?? []).slice(0, 10).map((r) => (
                  <div key={r.scanned_at} className="border-b border-[var(--border)]">
                    <div
                      className={cn(
                        'flex items-center justify-between py-1.5',
                        r.error && 'cursor-pointer'
                      )}
                      onClick={() => r.error && toggleError(r.scanned_at)}
                    >
                      <span className="text-secondary flex items-center gap-1.5">
                        {formatRelativeTime(r.scanned_at)}
                        {r.duration_ms != null && (
                          <span className="text-placeholder">{formatMs(r.duration_ms)}</span>
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
                            {expandedErrors.has(r.scanned_at) ? (
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
                    {r.error && expandedErrors.has(r.scanned_at) && (
                      <div className="pb-2">
                        <p className="text-[11px] text-red-300 font-mono bg-red-950/20 rounded p-2 break-all leading-relaxed">
                          {r.error}
                        </p>
                      </div>
                    )}
                  </div>
                ))}
              </div>
            ))}
        </Section>

        <Section title="New jobs">
          <div className="grid grid-cols-3 gap-2">
            <StatTile value={stats?.new_today ?? '—'} label="Today" />
            <StatTile value={stats?.new_this_month ?? '—'} label="30 days" />
            <StatTile value={stats?.total_new ?? '—'} label="All time" />
          </div>
          {stats && stats.total_new > 0 && (
            <button
              type="button"
              onClick={() => onViewJobs?.(company.name)}
              className="mt-2 w-full flex items-center justify-between px-3 py-2 border border-[var(--border)] rounded-lg text-xs text-secondary hover:text-primary hover:border-[var(--border-strong)] transition-colors"
            >
              <span>View {stats.total_new} new jobs</span>
              <span className="text-muted">→</span>
            </button>
          )}
        </Section>

        <Section
          title="Hiring Activity"
          aside={
            analytics
              ? velocityLabel(analytics.velocity.this_week, analytics.velocity.last_week)
              : undefined
          }
        >
          {analyticsError ? (
            <p className="text-xs text-muted">Failed to load hiring data</p>
          ) : !analytics ? (
            <div className="h-28 bg-white/5 rounded animate-pulse" />
          ) : analytics.monthly.every((p) => p.new === 0 && p.removed === 0) ? (
            <p className="text-xs text-muted">No hiring data yet</p>
          ) : (
            <div className="h-28">
              <ResponsiveContainer width="100%" height="100%">
                <AreaChart
                  data={analytics.monthly}
                  margin={{ top: 4, right: 4, left: 0, bottom: 0 }}
                >
                  <defs>
                    <linearGradient id="gradNew" x1="0" y1="0" x2="0" y2="1">
                      <stop offset="5%" stopColor="#10b981" stopOpacity={0.25} />
                      <stop offset="95%" stopColor="#10b981" stopOpacity={0} />
                    </linearGradient>
                    <linearGradient id="gradRemoved" x1="0" y1="0" x2="0" y2="1">
                      <stop offset="5%" stopColor="#f87171" stopOpacity={0.18} />
                      <stop offset="95%" stopColor="#f87171" stopOpacity={0} />
                    </linearGradient>
                  </defs>
                  <XAxis
                    dataKey="month"
                    tick={{ fontSize: 9, fill: '#64748b' }}
                    axisLine={false}
                    tickLine={false}
                    interval={1}
                  />
                  <YAxis hide />
                  <RechartTooltip
                    content={<HiringTooltip />}
                    cursor={{ stroke: 'rgba(255,255,255,0.08)', strokeWidth: 1 }}
                  />
                  <Area
                    type="monotone"
                    dataKey="new"
                    name="New"
                    stroke="#10b981"
                    strokeWidth={1.5}
                    fill="url(#gradNew)"
                    dot={false}
                    activeDot={{ r: 3, fill: '#10b981', strokeWidth: 0 }}
                  />
                  <Area
                    type="monotone"
                    dataKey="removed"
                    name="Removed"
                    stroke="#f87171"
                    strokeWidth={1.5}
                    strokeDasharray="3 3"
                    fill="url(#gradRemoved)"
                    dot={false}
                    activeDot={{ r: 3, fill: '#f87171', strokeWidth: 0 }}
                  />
                </AreaChart>
              </ResponsiveContainer>
            </div>
          )}
        </Section>
      </div>

      <Dialog.Root open={deleteConfirmOpen} onOpenChange={setDeleteConfirmOpen}>
        <Dialog.Portal>
          <Dialog.Overlay className="fixed inset-0 bg-black/60 z-50" />
          <Dialog.Content className="fixed left-1/2 top-1/2 -translate-x-1/2 -translate-y-1/2 z-50 w-full max-w-sm bg-panel border border-[var(--border-strong)] rounded-xl shadow-2xl p-6">
            <Dialog.Title className="text-base font-semibold text-primary mb-2">
              Delete company?
            </Dialog.Title>
            <Dialog.Description className="text-sm text-muted mb-5">
              This will permanently delete {company.name} and all its scan history. This cannot be
              undone.
            </Dialog.Description>
            <div className="flex gap-3">
              <button
                type="button"
                onClick={() => setDeleteConfirmOpen(false)}
                className="flex-1 px-4 py-2 rounded-lg text-sm font-medium bg-subtle hover:bg-hover text-secondary transition-colors"
              >
                Cancel
              </button>
              <button
                type="button"
                onClick={() => {
                  setDeleteConfirmOpen(false)
                  onDelete(company.id)
                }}
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
        <h3 className="text-[10px] font-medium text-muted uppercase tracking-wider">{title}</h3>
        {aside && <span className="text-[10px] text-muted">{aside}</span>}
      </div>
      {children}
    </div>
  )
}

function StatTile({ value, label }: { value: string | number; label: string }) {
  return (
    <div className="border border-[var(--border)] rounded-lg px-3 py-2.5">
      <div className="text-lg font-semibold text-white">{value}</div>
      <div className="text-[10px] uppercase tracking-wider text-muted mt-0.5">{label}</div>
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

function HiringTooltip({
  active,
  payload,
  label,
}: {
  active?: boolean
  payload?: { value: number; name: string }[]
  label?: string
}) {
  if (!active || !payload?.length) return null
  const newCount = payload.find((p) => p.name === 'New')?.value ?? 0
  const removed = payload.find((p) => p.name === 'Removed')?.value ?? 0
  const net = newCount - removed
  return (
    <div className="bg-panel border border-[var(--border-strong)] rounded-lg px-3 py-2 text-xs shadow-xl">
      <p className="text-muted mb-1.5 font-medium">{label}</p>
      <div className="flex flex-col gap-1">
        <span className="text-emerald-400">+{newCount} new</span>
        {removed > 0 && <span className="text-red-400">−{removed} removed</span>}
        <span className={cn('font-semibold mt-0.5', net >= 0 ? 'text-primary' : 'text-red-300')}>
          net {net >= 0 ? '+' : ''}
          {net}
        </span>
      </div>
    </div>
  )
}

function velocityLabel(thisWeek: number, lastWeek: number): string {
  const delta = thisWeek - lastWeek
  if (delta === 0) return `${thisWeek} new this week`
  const sign = delta > 0 ? '↑' : '↓'
  return `${sign} ${Math.abs(delta)} · ${thisWeek} this week`
}
