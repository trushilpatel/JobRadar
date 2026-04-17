import { useEffect, useState } from 'react'
import { Plus, RefreshCw, Timer } from 'lucide-react'
import { cn } from '@/lib/utils'

const AUTO_REFRESH_KEY = 'companies-auto-refresh'
const AUTO_REFRESH_INTERVAL_MS = 30_000

interface CompaniesHeaderProps {
  total: number
  totalActive: number
  onAddClick: () => void
}

export function CompaniesHeader({ total, totalActive, onAddClick }: CompaniesHeaderProps) {
  const [autoRefresh, setAutoRefresh] = useState(
    () => localStorage.getItem(AUTO_REFRESH_KEY) === 'true'
  )

  useEffect(() => {
    if (!autoRefresh) return
    const id = setInterval(() => window.location.reload(), AUTO_REFRESH_INTERVAL_MS)
    return () => clearInterval(id)
  }, [autoRefresh])

  function toggleAutoRefresh() {
    setAutoRefresh((prev) => {
      const next = !prev
      localStorage.setItem(AUTO_REFRESH_KEY, String(next))
      return next
    })
  }

  return (
    <div className="flex items-center justify-between px-6 py-3 border-b border-white/[0.07]">
      <div className="flex items-baseline gap-3">
        <h1 className="text-lg font-semibold text-white">Companies</h1>
        <span className="text-xs text-slate-400">
          {total} tracked · <span className="text-emerald-400">{totalActive} active</span>
        </span>
      </div>

      <div className="flex items-center gap-1.5">
        <button
          type="button"
          onClick={toggleAutoRefresh}
          title={
            autoRefresh
              ? 'Auto-refresh on — click to disable'
              : 'Auto-refresh off — click to enable (30s)'
          }
          className={cn(
            'flex items-center gap-1 px-2 py-1.5 rounded-lg border text-xs font-medium transition-colors',
            autoRefresh
              ? 'bg-emerald-500/15 border-emerald-500/30 text-emerald-400'
              : 'bg-white/5 border-white/10 text-slate-500 hover:text-slate-300'
          )}
        >
          <Timer size={12} />
          <span>30s</span>
        </button>

        <button
          type="button"
          onClick={() => window.location.reload()}
          title="Refresh page"
          className="p-1.5 rounded-lg bg-white/5 border border-white/10 text-slate-400 hover:text-white transition-colors"
        >
          <RefreshCw size={14} />
        </button>

        <button
          type="button"
          onClick={onAddClick}
          className="flex items-center gap-1.5 bg-violet-600 hover:bg-violet-500 text-white text-xs font-medium px-3 py-1.5 rounded-lg transition-colors"
        >
          <Plus size={12} />
          Add Company
        </button>
      </div>
    </div>
  )
}
