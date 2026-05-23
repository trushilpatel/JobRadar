import { useMemo } from 'react'
import type { CompanyWeekCount } from '@/types'
import { InsightCard, InsightEmpty, InsightError, InsightSkeleton } from './InsightCard'
import { useCompanyActivity } from './hooks/useInsights'

function ActivityContent({ data }: { data: CompanyWeekCount[] }) {
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
  const max = stats[0]?.total ?? 1
  return (
    <div className="space-y-2">
      {stats.map(({ company_name, total }) => (
        <div key={company_name} className="flex items-center gap-3">
          <p className="w-40 shrink-0 text-sm text-slate-300 truncate" title={company_name}>
            {company_name}
          </p>
          <div className="flex-1 bg-white/[0.04] rounded-full h-2 overflow-hidden">
            <div
              className="h-full bg-violet-600 rounded-full transition-all"
              style={{ width: `${(total / max) * 100}%` }}
            />
          </div>
          <span className="w-8 text-right text-xs text-slate-400 shrink-0">{total}</span>
        </div>
      ))}
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
