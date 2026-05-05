import { useCallback, useEffect, useState } from 'react'
import { useQueryClient } from '@tanstack/react-query'
import { RefreshCw, Timer } from 'lucide-react'
import { cn } from '@/lib/utils'
import { PipelineStrip } from './PipelineStrip'

const AUTO_REFRESH_KEY = 'companies-auto-refresh'
const AUTO_REFRESH_INTERVAL_MS = 30_000

interface CompaniesHeaderProps {
  errorCount: number
  errorFilterActive: boolean
  onErrorToggle: () => void
}

export function CompaniesHeader({
  errorCount,
  errorFilterActive,
  onErrorToggle,
}: CompaniesHeaderProps) {
  const queryClient = useQueryClient()
  const [autoRefresh, setAutoRefresh] = useState(
    () => localStorage.getItem(AUTO_REFRESH_KEY) === 'true'
  )

  const refresh = useCallback(() => {
    queryClient.invalidateQueries({ queryKey: ['admin-companies'] })
    queryClient.invalidateQueries({ queryKey: ['scan-queue'] })
    queryClient.invalidateQueries({ queryKey: ['pipeline-stats'] })
  }, [queryClient])

  useEffect(() => {
    if (!autoRefresh) return
    const id = setInterval(refresh, AUTO_REFRESH_INTERVAL_MS)
    return () => clearInterval(id)
  }, [autoRefresh, refresh])

  function toggleAutoRefresh() {
    setAutoRefresh((prev) => {
      const next = !prev
      localStorage.setItem(AUTO_REFRESH_KEY, String(next))
      return next
    })
  }

  return (
    <div className="flex items-center px-6 py-3 border-b border-white/[0.07] gap-6">
      <h1 className="text-lg font-semibold text-white shrink-0">Companies</h1>

      <div className="flex items-center flex-1 overflow-x-auto">
        <PipelineStrip />
      </div>

      <div className="flex items-center gap-1.5 shrink-0">
        {errorCount > 0 && (
          <button
            type="button"
            onClick={onErrorToggle}
            className={cn(
              'text-[11px] font-medium rounded-full px-2.5 py-1 whitespace-nowrap border transition-colors',
              errorFilterActive
                ? 'text-red-300 bg-red-500/20 border-red-500/40'
                : 'text-red-400 bg-red-500/10 border-red-500/20 hover:bg-red-500/20'
            )}
          >
            ⚠ {errorCount} {errorCount === 1 ? 'scan error' : 'scan errors'}
          </button>
        )}

        <button
          type="button"
          onClick={toggleAutoRefresh}
          aria-label={autoRefresh ? 'Auto-refresh: on' : 'Auto-refresh: off'}
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
          <span>{autoRefresh ? 'Live' : 'Auto'}</span>
        </button>

        <button
          type="button"
          onClick={refresh}
          aria-label="Refresh data"
          className="flex items-center gap-1 px-2 py-1.5 rounded-lg border text-xs font-medium transition-colors bg-white/5 border-white/10 text-slate-400 hover:text-white"
        >
          <RefreshCw size={12} />
          <span>Refresh</span>
        </button>
      </div>
    </div>
  )
}
