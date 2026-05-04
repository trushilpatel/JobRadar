import { useQuery } from '@tanstack/react-query'
import { fetchVelocity } from '@/api/insights'
import type { CompanyVelocityInsight } from '@/types'
import { cn } from '@/lib/utils'
import { InsightCard, InsightEmpty, InsightSkeleton } from './InsightCard'

const BADGE: Record<string, string> = {
  spike: 'bg-amber-500/15 text-amber-400',
  rising: 'bg-emerald-500/15 text-emerald-400',
  normal: 'bg-white/[0.06] text-slate-500',
}

function Sparkline({ row }: { row: CompanyVelocityInsight }) {
  const bars = [row.week_3, row.week_2, row.week_1, row.week_current]
  const max = Math.max(...bars, 1)
  return (
    <div className="flex items-end gap-[2px]" style={{ height: '20px' }}>
      {bars.map((v, i) => {
        const h = Math.max(3, Math.round((v / max) * 20))
        const isCurrent = i === bars.length - 1
        return (
          <div
            key={i}
            className="rounded-[2px]"
            style={{
              width: '7px',
              height: `${h}px`,
              background: isCurrent ? '#1d9e75' : '#1c3c34',
            }}
          />
        )
      })}
    </div>
  )
}

function VelocityContent({ data }: { data: CompanyVelocityInsight[] }) {
  if (data.length === 0) return <InsightEmpty />
  return (
    <div className="overflow-x-auto">
      <table className="w-full">
        <thead>
          <tr className="border-b border-white/[0.06]">
            <th className="py-2 px-3 text-left text-xs text-slate-500 font-medium">Company</th>
            <th className="py-2 px-3 text-center text-xs text-slate-500 font-medium">Velocity</th>
            <th className="py-2 px-3 text-center text-xs text-slate-500 font-medium">4 weeks</th>
            <th className="py-2 px-3 text-right text-xs text-slate-500 font-medium">This wk</th>
          </tr>
        </thead>
        <tbody>
          {data.map((row) => (
            <tr key={row.company_id} className="border-b border-white/[0.03] hover:bg-white/[0.02]">
              <td className="py-2.5 px-3 text-sm text-slate-200">{row.company_name}</td>
              <td className="py-2.5 px-3">
                <div className="flex justify-center">
                  <span
                    className={cn(
                      'inline-flex px-2 py-0.5 rounded-full text-[11px] font-medium',
                      BADGE[row.velocity_tag]
                    )}
                  >
                    {row.velocity_tag === 'normal' ? '—' : `${row.multiplier}×`}
                  </span>
                </div>
              </td>
              <td className="py-2.5 px-3">
                <div className="flex justify-center">
                  <Sparkline row={row} />
                </div>
              </td>
              <td className="py-2.5 px-3 text-right text-sm font-medium text-emerald-400 tabular-nums">
                +{row.week_current}
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  )
}

export function CompanyVelocityPanel() {
  const { data = [], isLoading } = useQuery({
    queryKey: ['insights-velocity'],
    queryFn: () => fetchVelocity(20),
    staleTime: 30_000,
  })
  return (
    <InsightCard
      title="Company Hiring Velocity"
      subtitle="Top companies by this week's activity — spike/rising shows multiplier vs 3-week avg"
    >
      {isLoading ? <InsightSkeleton height="h-48" /> : <VelocityContent data={data} />}
    </InsightCard>
  )
}
