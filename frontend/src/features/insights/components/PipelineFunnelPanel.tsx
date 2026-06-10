import type { CSSProperties } from 'react'
import type { FunnelStep } from '@/types'
import { cn } from '@/lib/utils/utils'
import { InsightCard, InsightEmpty, InsightError, InsightSkeleton } from './InsightCard'
import { useFunnel } from '../hooks/useInsights'

const COLORS: Record<string, string> = {
  enriched: 'bg-slate-500',
  saved: 'bg-amber-500',
  applied: 'bg-emerald-500',
  dismissed: 'bg-red-500/60',
}
const LABELS: Record<string, string> = {
  enriched: 'Enriched (total)',
  saved: 'Saved',
  applied: 'Applied',
  dismissed: 'Dismissed',
}
const ORDER = ['enriched', 'saved', 'applied', 'dismissed']

function FunnelContent({ data }: { data: FunnelStep[] }) {
  if (data.length === 0) return <InsightEmpty />
  const map = Object.fromEntries(data.map((d) => [d.stage, d.count]))
  const total = map['enriched'] ?? 1
  return (
    <div className="space-y-3">
      {ORDER.map((stage) => {
        const count = map[stage] ?? 0
        const pct = Math.round((count / total) * 100)
        return (
          <div key={stage}>
            <div className="flex items-center justify-between mb-1.5">
              <span className="text-xs text-muted">{LABELS[stage]}</span>
              <span className="text-xs font-medium text-primary tabular-nums">
                {count}
                <span className="text-muted ml-1.5">({pct}%)</span>
              </span>
            </div>
            <div className="h-1.5 bg-subtle rounded-full overflow-hidden">
              <div
                style={{ '--bar-pct': `${Math.max(pct, count > 0 ? 2 : 0)}%` } as CSSProperties}
                className={cn(
                  'h-full rounded-full transition-all [width:var(--bar-pct)]',
                  COLORS[stage]
                )}
              />
            </div>
          </div>
        )
      })}
    </div>
  )
}

export function PipelineFunnelPanel() {
  const { data, isLoading, isError } = useFunnel()
  return (
    <InsightCard title="Pipeline Funnel" subtitle="Enriched → saved → applied (all time)">
      {isLoading ? (
        <InsightSkeleton />
      ) : isError || !data ? (
        <InsightError />
      ) : (
        <FunnelContent data={data} />
      )}
    </InsightCard>
  )
}
