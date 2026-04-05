import { useEffect, useState } from 'react'
import {
  Briefcase, Sparkles, Bookmark, Send,
  CheckCircle2, ScanLine, BookmarkPlus, ExternalLink, XCircle,
} from 'lucide-react'
import type { DashboardStats, WeeklyDataPoint, ActivityEvent } from '@/types'
import { cn } from '@/lib/utils'

// TODO: replace with GET /api/stats when backend endpoint exists
const mockStats: DashboardStats = {
  scanned_today: 47,
  relevant_today: 12,
  saved: 8,
  applied: 3,
}

// TODO: replace with GET /api/stats/weekly when backend endpoint exists
const mockWeekly: WeeklyDataPoint[] = [
  { day: 'Mon', scanned: 38, relevant: 9 },
  { day: 'Tue', scanned: 52, relevant: 14 },
  { day: 'Wed', scanned: 29, relevant: 6 },
  { day: 'Thu', scanned: 61, relevant: 17 },
  { day: 'Fri', scanned: 44, relevant: 11 },
  { day: 'Sat', scanned: 15, relevant: 3 },
  { day: 'Sun', scanned: 47, relevant: 12 },
]

// TODO: replace with GET /api/activity when backend endpoint exists
const mockActivity: ActivityEvent[] = [
  { id: '1',  type: 'job_found',     description: 'New job found: Senior Engineer at Stripe',        time: new Date(Date.now() - 4 * 60 * 1000).toISOString() },
  { id: '2',  type: 'scan_complete', description: 'Scan completed for Stripe (12 jobs found)',        time: new Date(Date.now() - 6 * 60 * 1000).toISOString() },
  { id: '3',  type: 'job_found',     description: 'New job found: Staff Engineer at Linear',          time: new Date(Date.now() - 18 * 60 * 1000).toISOString() },
  { id: '4',  type: 'job_saved',     description: 'Job saved: Platform Engineer at Vercel',           time: new Date(Date.now() - 34 * 60 * 1000).toISOString() },
  { id: '5',  type: 'scan_complete', description: 'Scan completed for Notion (5 jobs found)',         time: new Date(Date.now() - 55 * 60 * 1000).toISOString() },
  { id: '6',  type: 'job_applied',   description: 'Applied to: Backend Engineer at Figma',            time: new Date(Date.now() - 2 * 3600 * 1000).toISOString() },
  { id: '7',  type: 'job_found',     description: 'New job found: Systems Engineer at Cloudflare',    time: new Date(Date.now() - 3 * 3600 * 1000).toISOString() },
  { id: '8',  type: 'job_dismissed', description: 'Job dismissed: Junior Developer at SomeCo',        time: new Date(Date.now() - 4 * 3600 * 1000).toISOString() },
  { id: '9',  type: 'scan_complete', description: 'Scan completed for GitHub (23 jobs found)',        time: new Date(Date.now() - 5 * 3600 * 1000).toISOString() },
  { id: '10', type: 'job_found',     description: 'New job found: Principal Engineer at Shopify',     time: new Date(Date.now() - 7 * 3600 * 1000).toISOString() },
]

const funnelSteps = [
  { label: 'Scanned',  key: 'scanned_today'  as const, color: 'bg-slate-500' },
  { label: 'Relevant', key: 'relevant_today' as const, color: 'bg-violet-500' },
  { label: 'Saved',    key: 'saved'           as const, color: 'bg-violet-400' },
  { label: 'Applied',  key: 'applied'         as const, color: 'bg-emerald-500' },
]

function formatRelative(iso: string): string {
  const sec = Math.floor((Date.now() - new Date(iso).getTime()) / 1000)
  if (sec < 60) return 'Just now'
  if (sec < 3600) return `${Math.floor(sec / 60)}m ago`
  if (sec < 86400) return `${Math.floor(sec / 3600)}h ago`
  return `${Math.floor(sec / 86400)}d ago`
}

const activityIcon: Record<ActivityEvent['type'], React.ReactNode> = {
  job_found:    <Briefcase size={14} className="text-violet-400" />,
  scan_complete: <ScanLine size={14} className="text-slate-400" />,
  job_saved:    <BookmarkPlus size={14} className="text-amber-400" />,
  job_applied:  <CheckCircle2 size={14} className="text-emerald-400" />,
  job_dismissed: <XCircle size={14} className="text-red-400" />,
}

export function DashboardPage() {
  const [stats, setStats] = useState<DashboardStats | null>(null)
  const [weekly, setWeekly] = useState<WeeklyDataPoint[]>([])
  const [activity, setActivity] = useState<ActivityEvent[]>([])
  const [loading, setLoading] = useState(true)

  useEffect(() => {
    // Simulate async load — swap out for real API calls when endpoints exist
    const t = setTimeout(() => {
      setStats(mockStats)
      setWeekly(mockWeekly)
      setActivity(mockActivity)
      setLoading(false)
    }, 300)
    return () => clearTimeout(t)
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

            {/* Recent activity feed */}
            <div className="bg-white/[0.04] border border-white/[0.07] rounded-xl">
              <div className="px-5 py-4 border-b border-white/[0.07]">
                <h2 className="text-sm font-medium text-white">Recent activity</h2>
              </div>
              <div className="divide-y divide-white/[0.04]">
                {activity.map(event => (
                  <div
                    key={event.id}
                    className="flex items-start gap-3 px-5 py-3 hover:bg-white/[0.02] transition-colors"
                  >
                    <div className="mt-0.5 shrink-0">
                      {activityIcon[event.type]}
                    </div>
                    <p className="flex-1 text-sm text-slate-300">{event.description}</p>
                    <span className="text-xs text-slate-500 shrink-0 mt-0.5">
                      {formatRelative(event.time)}
                    </span>
                  </div>
                ))}
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
          Dashboard stats use mock data — wire to a backend endpoint when available
        </p>
      </div>
    </div>
  )
}
