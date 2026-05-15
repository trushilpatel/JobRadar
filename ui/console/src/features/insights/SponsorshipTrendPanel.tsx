import { useQuery } from '@tanstack/react-query'
import {
  LineChart,
  Line,
  XAxis,
  YAxis,
  Tooltip,
  CartesianGrid,
  ResponsiveContainer,
} from 'recharts'
import { fetchSponsorshipTrend } from '@/api/insights'
import type { SponsorshipDay } from '@/types'
import { InsightCard, InsightEmpty, InsightError, InsightSkeleton } from './InsightCard'
import { tooltipStyle } from './insightStyles'
import { insightQueryKeys } from './queryKeys'

function SponsorshipContent({ data }: { data: SponsorshipDay[] }) {
  if (data.length === 0) return <InsightEmpty />
  const chartData = data.map((d) => ({
    date: d.date.slice(5),
    total: d.total,
    sponsored: d.sponsored,
    pct: d.total > 0 ? Math.round((d.sponsored / d.total) * 100) : 0,
  }))
  return (
    <ResponsiveContainer width="100%" height={180}>
      <LineChart data={chartData} margin={{ left: 0, right: 16, top: 4, bottom: 4 }}>
        <CartesianGrid stroke="rgba(255,255,255,0.04)" vertical={false} />
        <XAxis
          dataKey="date"
          tick={{ fill: '#64748b', fontSize: 11 }}
          tickLine={false}
          axisLine={false}
        />
        <YAxis
          tick={{ fill: '#64748b', fontSize: 11 }}
          tickLine={false}
          axisLine={false}
          width={30}
        />
        <Tooltip contentStyle={tooltipStyle} formatter={(v) => [v, '']} />
        <Line
          type="monotone"
          dataKey="total"
          stroke="#64748b"
          dot={false}
          strokeWidth={1.5}
          name="Total"
        />
        <Line
          type="monotone"
          dataKey="sponsored"
          stroke="#7c3aed"
          dot={false}
          strokeWidth={2}
          name="Sponsored"
        />
      </LineChart>
    </ResponsiveContainer>
  )
}

export function SponsorshipTrendPanel() {
  const { data, isLoading, isError } = useQuery({
    queryKey: insightQueryKeys.sponsorshipTrend(30),
    queryFn: () => fetchSponsorshipTrend(30),
    staleTime: 30_000,
  })
  return (
    <InsightCard
      title="Sponsorship Availability"
      subtitle="Daily: total enriched vs sponsorship-offering jobs (last 30 days)"
    >
      {isLoading ? (
        <InsightSkeleton height="h-48" />
      ) : isError || !data ? (
        <InsightError />
      ) : (
        <SponsorshipContent data={data} />
      )}
    </InsightCard>
  )
}
