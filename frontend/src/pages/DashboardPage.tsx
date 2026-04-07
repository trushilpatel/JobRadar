import { useEffect, useState } from 'react'
import { Sparkles, Bookmark, Send, ScanLine, TrendingUp, AlertCircle, Loader2, TriangleAlert } from 'lucide-react'
import type { DashboardStats, WeeklyDataPoint } from '@/types'
import { fetchDashboardStats, fetchDashboardActivity } from '@/api/dashboard'
import { cn } from '@/lib/utils'

export function DashboardPage() {
  const [stats, setStats] = useState<DashboardStats | null>(null)
  const [weekly, setWeekly] = useState<WeeklyDataPoint[]>([])
  const [loading, setLoading] = useState(true)

  useEffect(() => {
    Promise.all([fetchDashboardStats(), fetchDashboardActivity(7)])
      .then(([s, w]) => { setStats(s); setWeekly(w) })
      .catch(() => {})
      .finally(() => setLoading(false))
  }, [])

  const statCards = stats ? [
    {
      label: 'New in feed',
      value: stats.jobs_new,
      sub: 'unreviewed jobs',
      icon: <Sparkles size={16} />,
      color: 'text-violet-300',
      iconColor: 'text-violet-400 bg-violet-500/10',
    },
    {
      label: 'Scanned today',
      value: stats.scanned_today,
      sub: 'job pages fetched',
      icon: <ScanLine size={16} />,
      color: 'text-white',
      iconColor: 'text-slate-400 bg-white/[0.06]',
    },
    {
      label: 'Saved',
      value: stats.saved,
      sub: 'bookmarked jobs',
      icon: <Bookmark size={16} />,
      color: 'text-amber-300',
      iconColor: 'text-amber-400 bg-amber-500/10',
    },
    {
      label: 'Applied',
      value: stats.applied,
      sub: 'applications tracked',
      icon: <Send size={16} />,
      color: 'text-emerald-300',
      iconColor: 'text-emerald-400 bg-emerald-500/10',
    },
  ] : []

  const totalRelevant = stats ? stats.jobs_new + stats.saved + stats.applied : 0

  const funnelSteps = stats ? [
    { label: 'Scanned today', value: stats.scanned_today,  max: stats.scanned_today || 1, color: 'bg-slate-500' },
    { label: 'In pipeline',   value: totalRelevant,        max: stats.scanned_today || 1, color: 'bg-violet-500' },
    { label: 'Saved',         value: stats.saved,          max: stats.scanned_today || 1, color: 'bg-amber-500' },
    { label: 'Applied',       value: stats.applied,        max: stats.scanned_today || 1, color: 'bg-emerald-500' },
  ] : []

  const maxScanned = Math.max(...weekly.map(d => d.scanned), 1)

  const totalScanned7d = weekly.reduce((s, d) => s + d.scanned, 0)
  const totalRelevant7d = weekly.reduce((s, d) => s + d.relevant, 0)

  return (
    <div className="flex-1 flex flex-col min-h-0 overflow-auto">
      {/* Header */}
      <div className="px-6 py-5 border-b border-white/[0.07]">
        <h1 className="text-xl font-semibold text-white">Dashboard</h1>
        <p className="text-sm text-slate-400 mt-0.5">Pipeline overview</p>
      </div>

      <div className="flex-1 px-6 py-6 space-y-5">
        {loading ? (
          <div className="flex items-center justify-center h-48">
            <div className="w-6 h-6 border-2 border-violet-500 border-t-transparent rounded-full animate-spin" />
          </div>
        ) : (
          <>
            {/* Stat cards */}
            <div className="grid grid-cols-2 lg:grid-cols-4 gap-4">
              {statCards.map(card => (
                <div key={card.label} className="bg-white/[0.04] border border-white/[0.07] rounded-xl px-5 py-4">
                  <div className="flex items-center justify-between mb-3">
                    <span className="text-xs text-slate-500 font-medium">{card.label}</span>
                    <span className={cn('p-1.5 rounded-lg', card.iconColor)}>{card.icon}</span>
                  </div>
                  <p className={cn('text-3xl font-semibold tabular-nums', card.color)}>{card.value}</p>
                  <p className="text-xs text-slate-600 mt-1">{card.sub}</p>
                </div>
              ))}
            </div>

            {/* Pipeline queue status */}
            {stats && (
              <div className="flex items-center gap-3 px-4 py-3 bg-white/[0.03] border border-white/[0.07] rounded-xl">
                <span className="text-xs text-slate-500 font-medium mr-1">Queue</span>
                <div className="flex items-center gap-1.5 px-3 py-1.5 rounded-lg bg-white/[0.04] border border-white/[0.06]">
                  <Loader2 size={13} className={cn('text-violet-400', stats.pipeline_pending > 0 && 'animate-spin')} />
                  <span className="text-xs font-medium text-white tabular-nums">{stats.pipeline_pending}</span>
                  <span className="text-xs text-slate-500">pending</span>
                </div>
                <div className={cn(
                  'flex items-center gap-1.5 px-3 py-1.5 rounded-lg border',
                  stats.pipeline_errors > 0
                    ? 'bg-red-500/10 border-red-500/20'
                    : 'bg-white/[0.04] border-white/[0.06]'
                )}>
                  <TriangleAlert size={13} className={stats.pipeline_errors > 0 ? 'text-red-400' : 'text-slate-600'} />
                  <span className={cn('text-xs font-medium tabular-nums', stats.pipeline_errors > 0 ? 'text-red-300' : 'text-slate-500')}>{stats.pipeline_errors}</span>
                  <span className={cn('text-xs', stats.pipeline_errors > 0 ? 'text-red-400/70' : 'text-slate-600')}>errors</span>
                </div>
                <span className="text-xs text-slate-600 ml-auto">jobs awaiting scorer / enricher</span>
              </div>
            )}

            {/* Charts row */}
            <div className="grid grid-cols-1 lg:grid-cols-2 gap-4">
              {/* Pipeline funnel */}
              <div className="bg-white/[0.04] border border-white/[0.07] rounded-xl px-5 py-4">
                <h2 className="text-sm font-medium text-white mb-1">Pipeline</h2>
                <p className="text-xs text-slate-500 mb-4">Today's scan → pipeline totals</p>
                <div className="space-y-3">
                  {funnelSteps.map(step => {
                    const pct = Math.min(100, Math.round((step.value / step.max) * 100))
                    return (
                      <div key={step.label}>
                        <div className="flex items-center justify-between mb-1.5">
                          <span className="text-xs text-slate-400">{step.label}</span>
                          <span className="text-xs font-medium text-white tabular-nums">
                            {step.value}
                            <span className="text-slate-500 ml-1.5">({pct}%)</span>
                          </span>
                        </div>
                        <div className="h-1.5 bg-white/[0.06] rounded-full overflow-hidden">
                          <div
                            className={cn('h-full rounded-full transition-all', step.color)}
                            style={{ width: `${Math.max(pct, step.value > 0 ? 2 : 0)}%` }}
                          />
                        </div>
                      </div>
                    )
                  })}
                </div>
              </div>

              {/* 7-day bar chart */}
              <div className="bg-white/[0.04] border border-white/[0.07] rounded-xl px-5 py-4">
                <div className="flex items-center justify-between mb-1">
                  <h2 className="text-sm font-medium text-white">Last 7 days</h2>
                  <div className="flex items-center gap-3 text-xs text-slate-500">
                    <span className="flex items-center gap-1.5">
                      <span className="w-2 h-2 rounded-sm bg-slate-500 inline-block" /> Scanned
                    </span>
                    <span className="flex items-center gap-1.5">
                      <span className="w-2 h-2 rounded-sm bg-violet-500 inline-block" /> Relevant
                    </span>
                  </div>
                </div>
                <p className="text-xs text-slate-500 mb-4">
                  {totalScanned7d} scanned · {totalRelevant7d} relevant
                </p>

                {weekly.length === 0 ? (
                  <div className="flex items-center justify-center h-28 text-slate-500 text-xs">
                    No scan data yet
                  </div>
                ) : (
                  <div className="flex items-end gap-2" style={{ height: '110px' }}>
                    {weekly.map(day => (
                      <div key={day.day} className="flex-1 flex flex-col items-center gap-1">
                        <div className="flex items-end gap-0.5 w-full" style={{ height: '90px' }}>
                          <div
                            className="flex-1 bg-slate-600/60 rounded-t-sm transition-all"
                            style={{ height: `${Math.max(2, (day.scanned / maxScanned) * 90)}px` }}
                            title={`Scanned: ${day.scanned}`}
                          />
                          <div
                            className="flex-1 bg-violet-500/80 rounded-t-sm transition-all"
                            style={{ height: `${Math.max(day.relevant > 0 ? 2 : 0, (day.relevant / maxScanned) * 90)}px` }}
                            title={`Relevant: ${day.relevant}`}
                          />
                        </div>
                        <span className="text-xs text-slate-500">{day.day}</span>
                      </div>
                    ))}
                  </div>
                )}
              </div>
            </div>

            {/* Recent scan activity */}
            <div className="bg-white/[0.04] border border-white/[0.07] rounded-xl">
              <div className="px-5 py-4 border-b border-white/[0.07] flex items-center justify-between">
                <h2 className="text-sm font-medium text-white">Recent scans</h2>
                <span className="text-xs text-slate-500">Last 7 days</span>
              </div>
              {weekly.length === 0 ? (
                <div className="px-5 py-8 text-center text-sm text-slate-500">
                  No scans yet — add companies and trigger a scan
                </div>
              ) : (
                <div className="divide-y divide-white/[0.04]">
                  {[...weekly].reverse().map((day, i) => (
                    <div key={i} className="flex items-center gap-4 px-5 py-3">
                      <div className={cn(
                        'w-1.5 h-1.5 rounded-full shrink-0',
                        day.relevant > 0 ? 'bg-violet-400' : day.scanned > 0 ? 'bg-slate-500' : 'bg-white/10'
                      )} />
                      <span className="text-xs text-slate-400 w-8 shrink-0">{day.day}</span>
                      <div className="flex-1 flex items-center gap-3">
                        {day.scanned > 0 ? (
                          <>
                            <span className="flex items-center gap-1.5 text-xs text-slate-300">
                              <TrendingUp size={11} className="text-slate-500" />
                              {day.scanned} scanned
                            </span>
                            {day.relevant > 0 && (
                              <span className="flex items-center gap-1.5 text-xs text-violet-300">
                                <Sparkles size={11} />
                                {day.relevant} relevant
                              </span>
                            )}
                          </>
                        ) : (
                          <span className="text-xs text-slate-600">No scans</span>
                        )}
                      </div>
                      {day.scanned > 0 && (
                        <span className="text-xs text-slate-600 shrink-0">
                          {day.relevant > 0
                            ? `${Math.round((day.relevant / day.scanned) * 100)}% relevant`
                            : '0% relevant'}
                        </span>
                      )}
                    </div>
                  ))}
                </div>
              )}
            </div>

            {/* This week breakdown table */}
            {weekly.length > 0 && (
              <div className="bg-white/[0.04] border border-white/[0.07] rounded-xl">
                <div className="px-5 py-4 border-b border-white/[0.07]">
                  <h2 className="text-sm font-medium text-white">7-day breakdown</h2>
                </div>
                <div className="overflow-x-auto">
                  <table className="w-full">
                    <thead>
                      <tr className="border-b border-white/[0.04]">
                        <td className="py-2.5 px-5 text-xs text-slate-500 w-24" />
                        {weekly.map(d => (
                          <th key={d.day} className="py-2.5 px-4 text-xs font-medium text-slate-400 uppercase tracking-wider text-center">
                            {d.day}
                          </th>
                        ))}
                        <th className="py-2.5 px-4 text-xs font-medium text-slate-400 uppercase tracking-wider text-center">
                          Total
                        </th>
                      </tr>
                    </thead>
                    <tbody>
                      <tr className="border-b border-white/[0.04]">
                        <td className="py-2.5 px-5 text-xs text-slate-500">Scanned</td>
                        {weekly.map(d => (
                          <td key={d.day} className="py-2.5 px-4 text-center text-sm font-medium text-white tabular-nums">
                            {d.scanned}
                          </td>
                        ))}
                        <td className="py-2.5 px-4 text-center text-sm font-semibold text-white tabular-nums">
                          {totalScanned7d}
                        </td>
                      </tr>
                      <tr>
                        <td className="py-2.5 px-5 text-xs text-slate-500">Relevant</td>
                        {weekly.map(d => (
                          <td key={d.day} className="py-2.5 px-4 text-center text-sm font-medium text-violet-300 tabular-nums">
                            {d.relevant}
                          </td>
                        ))}
                        <td className="py-2.5 px-4 text-center text-sm font-semibold text-violet-300 tabular-nums">
                          {totalRelevant7d}
                        </td>
                      </tr>
                    </tbody>
                  </table>
                </div>
              </div>
            )}

            {/* Empty state hint */}
            {stats && stats.scanned_today === 0 && stats.jobs_new === 0 && (
              <div className="flex items-start gap-3 bg-amber-500/[0.06] border border-amber-500/20 rounded-xl px-5 py-4">
                <AlertCircle size={15} className="text-amber-400 mt-0.5 shrink-0" />
                <div>
                  <p className="text-sm text-amber-300 font-medium">No data yet</p>
                  <p className="text-xs text-amber-400/70 mt-0.5">
                    Add companies on the Companies page, then trigger a scan to start populating the pipeline.
                  </p>
                </div>
              </div>
            )}
          </>
        )}
      </div>
    </div>
  )
}
