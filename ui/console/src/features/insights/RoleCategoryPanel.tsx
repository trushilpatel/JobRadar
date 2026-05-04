import { useQuery } from '@tanstack/react-query'
import { fetchRoleCategories } from '@/api/insights'
import type { RoleCategoryCount } from '@/types'
import { InsightCard, InsightEmpty, InsightSkeleton } from './InsightCard'

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
          <span className="w-20 shrink-0 text-xs text-slate-300 capitalize">{category}</span>
          <div className="flex-1 bg-white/[0.04] rounded-full h-2 overflow-hidden">
            <div
              className="h-full rounded-full transition-all"
              style={{
                width: `${(count / max) * 100}%`,
                backgroundColor: ROLE_COLORS[category] ?? '#475569',
              }}
            />
          </div>
          <span className="w-8 text-right text-xs text-slate-400 shrink-0">{count}</span>
        </div>
      ))}
    </div>
  )
}

export function RoleCategoryPanel() {
  const { data = [], isLoading } = useQuery({
    queryKey: ['insights-role-categories'],
    queryFn: fetchRoleCategories,
    staleTime: 30_000,
  })
  return (
    <InsightCard title="Role Categories" subtitle="Distribution of enriched job types">
      {isLoading ? <InsightSkeleton /> : <RoleContent data={data} />}
    </InsightCard>
  )
}
