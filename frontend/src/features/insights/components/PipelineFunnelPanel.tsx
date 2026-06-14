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
const FUNNEL_STAGES = ['enriched', 'saved', 'applied'] as const

function funnelClipPath(topPct: number, botPct: number): string {
  const MIN = 4
  const tp = Math.max(topPct, MIN)
  const bp = Math.max(botPct, MIN)
  const lt = (100 - tp) / 2
  const rt = (100 + tp) / 2
  const lb = (100 - bp) / 2
  const rb = (100 + bp) / 2
  return `polygon(${lt}% 0%, ${rt}% 0%, ${rb}% 100%, ${lb}% 100%)`
}

function FunnelContent({ data }: { data: FunnelStep[] }) {
  if (data.length === 0) return <InsightEmpty />
  const map = Object.fromEntries(data.map((d) => [d.stage, d.count]))
  const total = Math.max(map['enriched'] ?? 1, 1)

  const pcts: Record<string, number> = {}
  for (const k of [...FUNNEL_STAGES, 'dismissed']) {
    pcts[k] = Math.round(((map[k] ?? 0) / total) * 100)
  }

  const dismissedCount = map['dismissed'] ?? 0

  return (
    <div className="space-y-3">
      {/* Tapered funnel: enriched → saved → applied */}
      <div className="space-y-0.5">
        {FUNNEL_STAGES.map((stage, i) => {
          const topPct = i === 0 ? 100 : (pcts[stage] ?? 0)
          const nextStage = FUNNEL_STAGES[i + 1]
          const botPct = nextStage ? (pcts[nextStage] ?? 0) : (pcts[stage] ?? 0)
          const count = map[stage] ?? 0
          return (
            <div key={stage}>
              <div className="flex items-center justify-between mb-1">
                <span className="text-xs text-muted">{LABELS[stage]}</span>
                <span className="text-xs font-medium text-primary tabular-nums">
                  {count}
                  <span className="text-muted ml-1">({pcts[stage]}%)</span>
                </span>
              </div>
              <div
                className={cn('h-8 w-full', COLORS[stage])}
                style={{ clipPath: funnelClipPath(topPct, botPct) }}
              />
            </div>
          )
        })}
      </div>

      {/* Dismissed — exit path, shown as standard bar below the funnel */}
      <div className="pt-3 border-t border-[var(--border)]">
        <div className="flex items-center justify-between mb-1.5">
          <span className="text-xs text-muted">{LABELS['dismissed']}</span>
          <span className="text-xs font-medium text-primary tabular-nums">
            {dismissedCount}
            <span className="text-muted ml-1">({pcts['dismissed']}%)</span>
          </span>
        </div>
        <div className="h-1.5 bg-subtle rounded-full overflow-hidden">
          <div
            className={cn('h-full rounded-full', COLORS['dismissed'])}
            style={{ width: dismissedCount > 0 ? `max(${pcts['dismissed']}%, 6px)` : '0' }}
          />
        </div>
      </div>
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
