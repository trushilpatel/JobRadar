import { AlertCircle } from 'lucide-react'
import { cn } from '@/lib/utils'
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
    <div className="flex flex-col items-center min-w-[70px]">
      <span
        className={cn(
          'text-[18px] font-semibold leading-tight tabular-nums',
          valueClass ?? 'text-white'
        )}
      >
        {value}
      </span>
      <span className="text-[9px] uppercase tracking-[0.08em] text-slate-500 mt-0.5 text-center whitespace-nowrap">
        {label}
      </span>
    </div>
  )
}

function Arrow() {
  return <span className="text-slate-600 text-sm px-1.5 select-none">→</span>
}

function Skeleton() {
  return (
    <div className="flex items-center gap-2 animate-pulse">
      {[...Array(7)].map((_, i) => (
        <div key={i} className="flex items-center gap-1">
          <div className="w-14 h-8 bg-white/[0.04] rounded" />
          {i < 6 && <div className="w-4 h-3 bg-white/[0.02] rounded" />}
        </div>
      ))}
    </div>
  )
}

export function PipelineStrip() {
  const { data, isLoading, isError } = usePipelineStats()

  if (isLoading) return <Skeleton />
  if (isError || !data) {
    return (
      <div className="flex items-center gap-2 text-xs text-slate-500">
        <AlertCircle size={13} className="text-red-400/70" />
        Pipeline stats unavailable
      </div>
    )
  }

  return (
    <div className="flex items-center">
      <Stage label="Total" value={data.total_companies} />
      <Arrow />
      <Stage label="Active" value={data.active_companies} valueClass="text-emerald-400" />
      <Arrow />
      <Stage label="Queued" value={data.queued} valueClass="text-amber-400" />
      <Arrow />
      <Stage label="Crawling" value={data.crawling} valueClass="text-blue-400" />
      <Arrow />
      <Stage label="Awaiting Filter" value={data.awaiting_filter} valueClass="text-violet-400" />
      <Arrow />
      <Stage
        label="Awaiting Enrichment"
        value={data.awaiting_enrichment}
        valueClass="text-violet-400"
      />
      <Arrow />
      <Stage label="New Jobs (24h)" value={data.new_jobs_24h} valueClass="text-emerald-400" />
    </div>
  )
}
