import { useEffect, useState } from 'react'
import {
  Sparkles, Bookmark, Send, ScanLine, ExternalLink,
} from 'lucide-react'
import type { DashboardStats, WeeklyDataPoint } from '@/types'
import { fetchDashboardStats, fetchDashboardActivity } from '@/api/dashboard'
import { cn } from '@/lib/utils'

const funnelSteps = [
  { label: 'Scanned',  key: 'scanned_today'  as const, color: 'bg-slate-500' },
  { label: 'Relevant', key: 'relevant_today' as const, color: 'bg-violet-500' },
  { label: 'Saved',    key: 'saved'           as const, color: 'bg-violet-400' },
  { label: 'Applied',  key: 'applied'         as const, color: 'bg-emerald-500' },
]

export function DashboardPage() {
  const [stats, setStats] = useState<DashboardStats | null>(null)
  const [weekly, setWeekly] = useState<WeeklyDataPoint[]>([])
  const [loading, setLoading] = useState(true)

  useEffect(() => {
    Promise.all([fetchDashboardStats(), fetchDashboardActivity(7)])
      .then(([s, w]) => {
        setStats(s)
        setWeekly(w)
      })
      .catch(() => {/* leave defaults — empty state renders below */})
      .finally(() => setLoading(false))
  }, [])

  const maxScanned = Math.max(...weekly.map(d => d.scanned), 1)

  const statCards = [
    {
      label: 'Scanned today',
      value: stats?.scanned_today ?? 0,
      icon: <ScanLine size={16} className="text-slate-400" />,
      color: 'text-white',
    },
    {
      label: 'Relevant today',
      value: stats?.relevant_today ?? 0,
      icon: <Sparkles size={16} className="text-violet-400" />,
      color: 'text-violet-300',
    },
    {
      label: 'Jobs saved',
      value: stats?.saved ?? 0,
      icon: <Bookmark size={16} className="text-amber-400" />,
      color: 'text-amber-300',
    },
    {
      label: 'Applied',
      value: stats?.applied ?? 0,
      icon: <Send size={16} className="text-emerald-400" />,
      color: 'text-emerald-300',
    },
  ]

  return (
    <div className="flex-1 flex flex-col min-h-0 overflow-auto">
      {/* Header */}
      <div className="px-6 py-5 border-b border-white/[0.07]">
        <h1 className="text-xl font-semibold text-white">Dashboard</h1>
        <p className="text-sm text-slate-400 mt-0.5">Pipeline overview and recent activity</p>
      </div>

      <div className="flex-1 px-6 py-6 space-y-6">
        {loading ? (
          <div className="flex items-center justify-center h-48">
            <div className="w-6 h-6 border-2 border-violet-500 border-t-transparent rounded-full animate-spin" />
          </div>
        ) : (
          <>
            {/* Stats bar */}
            <div className="grid grid-cols-2 lg:grid-cols-4 gap-4">
              {statCards.map(card => (
                <div
                  key={card.label}
                  className="bg-white/[0.04] border border-white/[0.07] rounded-xl px-5 py-4"
                >
                  <div className="flex items-center justify-between mb-3">
                    <span className="text-xs text-slate-500 uppercase tracking-wider font-medium">
                      {card.label}
                    </span>
                    {card.icon}
                  </div>
                  <p className={cn('text-3xl font-semibold tabular-nums', card.color)}>
                    {card.value}
                  </p>
                </div>
              ))}
            </div>

            {/* Charts row */}
            <div className="grid grid-cols-1 lg:grid-cols-2 gap-4">
              {/* Pipeline funnel */}
              <div className="bg-white/[0.04] border border-white/[0.07] rounded-xl px-5 py-4">
                <h2 className="text-sm font-medium text-white mb-4">Pipeline funnel (today)</h2>
                {stats && (
                  <div className="space-y-3">
                    {funnelSteps.map(step => {
                      const val = stats[step.key]
                      const max = stats.scanned_today || 1
                      const pct = Math.round((val / max) * 100)
                      return (
                        <div key={step.key}>
                          <div className="flex items-center justify-between mb-1.5">
                            <span className="text-xs text-slate-400">{step.label}</span>
                            <span className="text-xs font-medium text-white tabular-nums">
                              {val}
                              <span className="text-slate-500 ml-1.5">({pct}%)</span>
                            </span>
                          </div>
                          <div className="h-2 bg-white/[0.06] rounded-full overflow-hidden">
                            <div
                              className={cn('h-full rounded-full transition-all', step.color)}
                              style={{ width: `${pct}%` }}
                            />
                          </div>
                        </div>
                      )
                    })}
                  </div>
                )}
              </div>

              {/* Weekly activity chart */}
              <div className="bg-white/[0.04] border border-white/[0.07] rounded-xl px-5 py-4">
                <div className="flex items-center justify-between mb-4">
                  <h2 className="text-sm font-medium text-white">Weekly activity</h2>
                  <div className="flex items-center gap-4 text-xs text-slate-500">
                    <span className="flex items-center gap-1.5">
                      <span className="w-2.5 h-2.5 rounded-sm bg-slate-500 inline-block" />
                      Scanned
                    </span>
                    <span className="flex items-center gap-1.5">
                      <span className="w-2.5 h-2.5 rounded-sm bg-violet-500 inline-block" />
                      Relevant
                    </span>
                  </div>
                </div>

                {/* Grouped bar chart */}
                <div className="flex items-end gap-3" style={{ height: '120px' }}>
                  {weekly.map(day => (
                    <div key={day.day} className="flex-1 flex flex-col items-center gap-1">
                      {/* Bars group */}
                      <div className="flex items-end gap-0.5 w-full" style={{ height: '100px' }}>
                        <div
                          className="flex-1 bg-slate-600/50 rounded-t-sm transition-all"
                          style={{ height: `${Math.max(2, (day.scanned / maxScanned) * 100)}px` }}
                          title={`Scanned: ${day.scanned}`}
                        />
                        <div
                          className="flex-1 bg-violet-500/80 rounded-t-sm transition-all"
                          style={{ height: `${Math.max(2, (day.relevant / maxScanned) * 100)}px` }}
                          title={`Relevant: ${day.relevant}`}
                        />
                      </div>
                      <span className="text-xs text-slate-500">{day.day}</span>
                    </div>
                  ))}
                </div>
              </div>
            </div>

            {/* Recent activity feed — populated once scan events are persisted */}
            <div className="bg-white/[0.04] border border-white/[0.07] rounded-xl">
              <div className="px-5 py-4 border-b border-white/[0.07]">
                <h2 className="text-sm font-medium text-white">Recent activity</h2>
              </div>
              <div className="px-5 py-8 text-center text-sm text-slate-500">
                No recent activity — run a scan to populate this feed
              </div>
            </div>

            {/* Weekly numbers table */}
            <div className="bg-white/[0.04] border border-white/[0.07] rounded-xl">
              <div className="px-5 py-4 border-b border-white/[0.07]">
                <h2 className="text-sm font-medium text-white">This week breakdown</h2>
              </div>
              <div className="overflow-x-auto">
                <table className="w-full">
                  <thead>
                    <tr>
                      {weekly.map(d => (
                        <th key={d.day} className="py-3 px-4 text-xs font-medium text-slate-400 uppercase tracking-wider text-center">
                          {d.day}
                        </th>
                      ))}
                    </tr>
                  </thead>
                  <tbody>
                    <tr className="border-t border-white/[0.04]">
                      {weekly.map(d => (
                        <td key={d.day} className="py-2 px-4 text-center">
                          <span className="text-sm font-medium text-white">{d.scanned}</span>
                          <p className="text-xs text-slate-500">scanned</p>
                        </td>
                      ))}
                    </tr>
                    <tr className="border-t border-white/[0.04]">
                      {weekly.map(d => (
                        <td key={d.day} className="py-2 px-4 text-center">
                          <span className="text-sm font-medium text-violet-300">{d.relevant}</span>
                          <p className="text-xs text-slate-500">relevant</p>
                        </td>
                      ))}
                    </tr>
                  </tbody>
                </table>
              </div>
            </div>
          </>
        )}
      </div>

      {/* Footer note */}
      <div className="px-6 py-3 border-t border-white/[0.07]">
        <p className="text-xs text-slate-600 flex items-center gap-1">
          <ExternalLink size={10} />
          Live data from /dashboard/stats and /dashboard/activity
        </p>
      </div>
    </div>
  )
}
