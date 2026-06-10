import type { CSSProperties } from 'react'
import type { RoleCategoryCount } from '@/types'
import { InsightCard, InsightEmpty, InsightError, InsightSkeleton } from './InsightCard'
import { useRoleCategories } from '../hooks/useInsights'

const ROLE_COLORS: Record<string, string> = {
  backend: '#7c3aed',
  frontend: '#2563eb',
  fullstack: '#0891b2',
  devops: '#059669',
  ml: '#d97706',
  data: '#dc2626',
  other: '#475569',
}

function RoleContent({ data }: { data: RoleCategoryCount[] }) {
  if (data.length === 0) return <InsightEmpty />
  const max = data[0]?.count ?? 1
  return (
    <div className="space-y-2.5">
      {data.map(({ category, count }) => (
        <div key={category} className="flex items-center gap-3">
          <span className="w-20 shrink-0 text-xs text-secondary capitalize">{category}</span>
          <div className="flex-1 bg-subtle rounded-full h-2 overflow-hidden">
            <div
              style={
                {
                  '--bar-pct': `${(count / max) * 100}%`,
                  '--bar-color': ROLE_COLORS[category] ?? '#475569',
                } as CSSProperties
              }
              className="h-full rounded-full transition-all [width:var(--bar-pct)] [background-color:var(--bar-color)]"
            />
          </div>
          <span className="w-8 text-right text-xs text-muted shrink-0">{count}</span>
        </div>
      ))}
    </div>
  )
}

export function RoleCategoryPanel() {
  const { data, isLoading, isError } = useRoleCategories()
  return (
    <InsightCard title="Role Categories" subtitle="Distribution of enriched job types">
      {isLoading ? (
        <InsightSkeleton />
      ) : isError || !data ? (
        <InsightError />
      ) : (
        <RoleContent data={data} />
      )}
    </InsightCard>
  )
}
