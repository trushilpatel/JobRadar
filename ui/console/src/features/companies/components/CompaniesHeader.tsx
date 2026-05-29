import { useCallback } from 'react'
import { useQueryClient } from '@tanstack/react-query'
import { RefreshCw, Timer } from 'lucide-react'
import { cn } from '@/lib/utils/utils'
import { companyQueryKeys } from '@/features/companies/queryKeys'
import { PipelineStrip } from './PipelineStrip'

interface CompaniesHeaderProps {
  autoRefresh: boolean
  onToggleAutoRefresh: () => void
}

export function CompaniesHeader({ autoRefresh, onToggleAutoRefresh }: CompaniesHeaderProps) {
  const queryClient = useQueryClient()

  const refresh = useCallback(() => {
    queryClient.invalidateQueries({ queryKey: companyQueryKeys.adminCompaniesBase() })
    queryClient.invalidateQueries({ queryKey: companyQueryKeys.pipelineStats() })
  }, [queryClient])

  return (
    <div className="flex items-center px-6 py-3 border-b border-white/[0.07] gap-6">
      <h1 className="text-lg font-semibold text-white shrink-0">Companies</h1>

      <div className="flex items-center flex-1 overflow-x-auto">
        <PipelineStrip />
      </div>

      <div className="flex items-center gap-1.5 shrink-0">
        <button
          type="button"
          onClick={onToggleAutoRefresh}
          aria-label={autoRefresh ? 'Auto-refresh: on' : 'Auto-refresh: off'}
          title={
            autoRefresh
              ? 'Auto-refresh on — click to disable'
              : 'Auto-refresh off — click to enable (10s)'
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
