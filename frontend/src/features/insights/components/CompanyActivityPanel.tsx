import { useMemo, useState } from 'react'
import type { CSSProperties } from 'react'
import type { CompanyWeekCount } from '@/types'
import { InsightCard, InsightEmpty, InsightError, InsightSkeleton } from './InsightCard'
import { useCompanyActivity } from '../hooks/useInsights'

const CAP = 30

function ActivityContent({ data }: { data: CompanyWeekCount[] }) {
  const [showAll, setShowAll] = useState(false)

  const stats = useMemo(() => {
    const map: Record<string, number> = {}
    for (const row of data) {
      map[row.company_name] = (map[row.company_name] ?? 0) + row.count
    }
    return Object.entries(map)
      .map(([company_name, total]) => ({ company_name, total }))
      .sort((a, b) => b.total - a.total)
  }, [data])

  if (stats.length === 0) return <InsightEmpty />

  const displayed = showAll ? stats : stats.slice(0, CAP)
  const hasMore = stats.length > CAP
  const max = stats[0]?.total ?? 1

  return (
    <div className="space-y-2">
      {displayed.map(({ company_name, total }) => (
        <div key={company_name} className="flex items-center gap-3">
          <p className="w-40 shrink-0 text-sm text-secondary truncate" title={company_name}>
            {company_name}
          </p>
          <div className="flex-1 bg-subtle rounded-full h-2 overflow-hidden">
            <div
              style={{ '--bar-pct': `${(total / max) * 100}%` } as CSSProperties}
              className="h-full bg-violet-600 rounded-full transition-all [width:var(--bar-pct)]"
            />
          </div>
          <span className="w-8 text-right text-xs text-muted shrink-0">{total}</span>
        </div>
      ))}
      {hasMore && (
        <button
          type="button"
          onClick={() => setShowAll((v) => !v)}
          className="mt-2 text-xs text-muted hover:text-primary transition-colors"
        >
          {showAll ? 'Show less' : `Show all (${stats.length})`}
        </button>
      )}
    </div>
  )
}

export function CompanyActivityPanel() {
  const { data, isLoading, isError } = useCompanyActivity()
  return (
    <InsightCard title="Company Activity" subtitle="Total jobs enriched per company (all time)">
      {isLoading ? (
        <InsightSkeleton />
      ) : isError || !data ? (
        <InsightError />
      ) : (
        <ActivityContent data={data} />
      )}
    </InsightCard>
  )
}
