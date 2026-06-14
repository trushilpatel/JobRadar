import { BarChart, Bar, XAxis, YAxis, Tooltip, ResponsiveContainer, Cell } from 'recharts'
import type { TechCount } from '@/types'
import { InsightCard, InsightEmpty, InsightError, InsightSkeleton } from './InsightCard'
import { tooltipStyle } from '../insightStyles'
import { useTechStack } from '../hooks/useInsights'

function TechContent({ data }: { data: TechCount[] }) {
  if (data.length === 0) return <InsightEmpty />
  return (
    <ResponsiveContainer width="100%" height={420}>
      <BarChart layout="vertical" data={data} margin={{ left: 10, right: 24, top: 4, bottom: 4 }}>
        <XAxis
          type="number"
          stroke="var(--border)"
          tick={{ fill: 'var(--text-muted)', fontSize: 11 }}
          tickLine={false}
          axisLine={false}
        />
        <YAxis
          type="category"
          dataKey="tech"
          stroke="var(--border)"
          tick={{ fill: 'var(--text-muted)', fontSize: 12 }}
          width={90}
          tickLine={false}
          axisLine={false}
        />
        <Tooltip
          cursor={{ fill: 'rgba(127,127,127,0.06)' }}
          contentStyle={tooltipStyle}
          formatter={(value) => [value ?? 0, 'Jobs']}
        />
        <Bar dataKey="count" radius={[0, 4, 4, 0]} maxBarSize={22}>
          {data.map((_, i) => (
            <Cell key={i} fill={i === 0 ? '#7c3aed' : i < 5 ? '#6d28d9' : '#4c1d95'} />
          ))}
        </Bar>
      </BarChart>
    </ResponsiveContainer>
  )
}

export function TechStackPanel() {
  const { data, isLoading, isError } = useTechStack(15)
  return (
    <InsightCard title="Tech Stack Trends" subtitle="Top 15 technologies across all enriched jobs">
      {isLoading ? (
        <InsightSkeleton height="h-48" />
      ) : isError || !data ? (
        <InsightError />
      ) : (
        <TechContent data={data} />
      )}
    </InsightCard>
  )
}
