import { cn } from '@/lib/utils'
import type { PipelineStats } from '@/types'
import { usePipelineStats } from '../hooks/usePipelineStats'

function Stage({
  label,
  value,
  valueClass,
}: {
  label: string
  value: number
  valueClass?: string
}) {
  return (
    <div className="flex flex-col items-center min-w-[80px]">
      <span
        className={cn(
          'text-[22px] font-semibold leading-tight tabular-nums',
          valueClass ?? 'text-white'
        )}
      >
        {value}
      </span>
      <span className="text-[10px] uppercase tracking-[0.08em] text-slate-500 mt-0.5 text-center whitespace-nowrap">
        {label}
      </span>
    </div>
  )
}

function Arrow() {
  return <span className="text-slate-600 text-sm px-2 select-none">→</span>
}

function Skeleton() {
  return (
    <div className="flex items-center gap-2 animate-pulse">
      {[...Array(7)].map((_, i) => (
        <div key={i} className="flex items-center gap-2">
          <div className="w-16 h-9 bg-white/[0.04] rounded" />
          {i < 6 && <div className="w-4 h-3 bg-white/[0.02] rounded" />}
        </div>
      ))}
    </div>
  )
}

interface PipelineContentProps {
  stats: PipelineStats
  errorCount: number
  errorFilterActive: boolean
  onErrorToggle: () => void
}

function PipelineContent({
  stats,
  errorCount,
  errorFilterActive,
  onErrorToggle,
}: PipelineContentProps) {
  return (
    <div className="flex items-center w-full">
      <Stage label="Total" value={stats.total_companies} />
      <Arrow />
      <Stage label="Active" value={stats.active_companies} valueClass="text-emerald-400" />
      <Arrow />
      <Stage label="Queued" value={stats.queued} valueClass="text-amber-400" />
      <Arrow />
      <Stage label="Crawling" value={stats.crawling} valueClass="text-blue-400" />
      <Arrow />
      <Stage label="Awaiting Filter" value={stats.awaiting_filter} valueClass="text-violet-400" />
      <Arrow />
      <Stage
        label="Awaiting Enrichment"
        value={stats.awaiting_enrichment}
        valueClass="text-violet-400"
      />
      <Arrow />
      <Stage label="New Jobs (24h)" value={stats.new_jobs_24h} valueClass="text-emerald-400" />

      {errorCount > 0 && (
        <button
          type="button"
          onClick={onErrorToggle}
          className={cn(
            'ml-auto text-[11px] font-medium rounded-full px-2.5 py-1 whitespace-nowrap border transition-colors',
            errorFilterActive
              ? 'text-red-300 bg-red-500/20 border-red-500/40'
              : 'text-red-400 bg-red-500/10 border-red-500/20 hover:bg-red-500/20'
          )}
        >
          ⚠ {errorCount} {errorCount === 1 ? 'scan error' : 'scan errors'}
        </button>
      )}
    </div>
  )
}

interface PipelineStripProps {
  errorCount: number
  errorFilterActive: boolean
  onErrorToggle: () => void
}

export function PipelineStrip({
  errorCount,
  errorFilterActive,
  onErrorToggle,
}: PipelineStripProps) {
  const { data, isLoading } = usePipelineStats()

  return (
    <div className="px-6 py-3 border-b border-white/[0.07] overflow-x-auto">
      {isLoading || !data ? (
        <Skeleton />
      ) : (
        <PipelineContent
          stats={data}
          errorCount={errorCount}
          errorFilterActive={errorFilterActive}
          onErrorToggle={onErrorToggle}
        />
      )}
    </div>
  )
}
