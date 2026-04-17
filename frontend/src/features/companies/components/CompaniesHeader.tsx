import { Plus, RefreshCw } from 'lucide-react'
import { cn } from '@/lib/utils'

interface CompaniesHeaderProps {
  total: number
  totalActive: number
  loading: boolean
  onRefresh: () => void
  onAddClick: () => void
}

export function CompaniesHeader({
  total,
  totalActive,
  loading,
  onRefresh,
  onAddClick,
}: CompaniesHeaderProps) {
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
          onClick={onRefresh}
          title="Refresh"
          className="p-1.5 rounded-lg bg-white/5 border border-white/10 text-slate-400 hover:text-white transition-colors"
        >
          <RefreshCw size={14} className={cn(loading && 'animate-spin')} />
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
