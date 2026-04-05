import { useEffect, useState } from 'react'
import { X, CheckCircle2, XCircle, Clock, TrendingUp, Zap } from 'lucide-react'
import type { Company, ScanStats } from '@/types'
import { fetchScanStats } from '@/api/companies'
import { formatRelativeTime } from '@/lib/utils'

interface Props {
  company: Company | null
  onClose: () => void
}

export function ScanStatsDrawer({ company, onClose }: Props) {
  const [stats, setStats] = useState<ScanStats | null>(null)
  const [loading, setLoading] = useState(false)

  useEffect(() => {
    if (!company) return
    setStats(null)
    setLoading(true)
    fetchScanStats(company.id)
      .then(setStats)
      .catch(console.error)
      .finally(() => setLoading(false))
  }, [company?.id])

  if (!company) return null

  return (
    <>
      <div className="fixed inset-0 z-40" onClick={onClose} />
      <div className="fixed right-0 top-0 h-full w-full max-w-md bg-[#13131a] border-l border-white/10 z-50 flex flex-col shadow-2xl">
        {/* Header */}
        <div className="flex items-center justify-between p-5 border-b border-white/10">
          <div>
            <h2 className="text-base font-semibold text-white">{company.name}</h2>
            <a href={company.url} target="_blank" rel="noreferrer" className="text-xs text-slate-400 hover:text-violet-400 transition-colors truncate block max-w-xs">
              {company.url}
            </a>
          </div>
          <button onClick={onClose} className="text-slate-400 hover:text-white transition-colors">
            <X size={18} />
          </button>
        </div>

        <div className="flex-1 overflow-y-auto p-5 space-y-6">
          {loading && (
            <div className="flex items-center justify-center h-32">
              <div className="w-6 h-6 border-2 border-violet-500 border-t-transparent rounded-full animate-spin" />
            </div>
          )}

          {stats && (
            <>
              {/* Summary stat cards */}
              <div className="grid grid-cols-2 gap-3">
                <StatCard icon={<TrendingUp size={14} />} label="Total Scans" value={stats.total_scans} />
                <StatCard icon={<Zap size={14} />} label="New Jobs Found" value={stats.total_new} accent />
                <StatCard icon={<CheckCircle2 size={14} />} label="Total Fetched" value={stats.total_fetched} />
                <StatCard
                  icon={<Clock size={14} />}
                  label="Avg Duration"
                  value={stats.avg_duration_ms ? `${Math.round(stats.avg_duration_ms)}ms` : '—'}
                />
              </div>

              {stats.error_count > 0 && (
                <div className="flex items-center gap-2 bg-red-500/10 border border-red-500/20 rounded-lg px-3 py-2">
                  <XCircle size={14} className="text-red-400 shrink-0" />
                  <span className="text-sm text-red-300">{stats.error_count} scan{stats.error_count !== 1 ? 's' : ''} failed</span>
                </div>
              )}

              {/* Recent runs */}
              <div>
                <h3 className="text-xs font-medium text-slate-400 uppercase tracking-wider mb-3">Recent Scans</h3>
                {stats.recent_runs.length === 0 ? (
                  <p className="text-sm text-slate-500">No scans yet</p>
                ) : (
                  <div className="space-y-2">
                    {stats.recent_runs.map(run => (
                      <div key={run.id} className="flex items-center gap-3 bg-white/[0.03] border border-white/[0.06] rounded-lg px-3 py-2.5">
                        <div className="shrink-0">
                          {run.error
                            ? <XCircle size={14} className="text-red-400" />
                            : <CheckCircle2 size={14} className="text-emerald-400" />
                          }
                        </div>
                        <div className="flex-1 min-w-0">
                          {run.error
                            ? <p className="text-xs text-red-300 truncate">{run.error}</p>
                            : <p className="text-xs text-slate-300">
                                <span className="text-white font-medium">{run.jobs_new}</span> new
                                <span className="text-slate-500"> / {run.jobs_fetched} fetched</span>
                              </p>
                          }
                          <p className="text-xs text-slate-500 mt-0.5">{formatRelativeTime(run.scanned_at)}</p>
                        </div>
                        {run.duration_ms && (
                          <span className="text-xs text-slate-500 shrink-0">{run.duration_ms}ms</span>
                        )}
                      </div>
                    ))}
                  </div>
                )}
              </div>
            </>
          )}
        </div>
      </div>
    </>
  )
}

function StatCard({ icon, label, value, accent }: {
  icon: React.ReactNode
  label: string
  value: string | number
  accent?: boolean
}) {
  return (
    <div className="bg-white/[0.03] border border-white/[0.06] rounded-lg p-3">
      <div className={`flex items-center gap-1.5 mb-1.5 ${accent ? 'text-violet-400' : 'text-slate-400'}`}>
        {icon}
        <span className="text-xs">{label}</span>
      </div>
      <p className={`text-xl font-semibold ${accent ? 'text-violet-300' : 'text-white'}`}>{value}</p>
    </div>
  )
}
