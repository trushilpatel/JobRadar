import { useQuery } from '@tanstack/react-query'
import { fetchTechTrends } from '@/api/dashboard'
import type { TechTrendRow } from '@/types'
import { dashboardQueryKeys } from './queryKeys'

// Sparkline for a tech row — 4 quarter bars
function TechSparkline({
  values,
  activeColor,
  dimColor,
}: {
  values: [number, number, number, number]
  activeColor: string
  dimColor: string
}) {
  const max = Math.max(...values, 1)
  return (
    <div className="flex items-end gap-[3px]" style={{ height: '20px', flexShrink: 0 }}>
      {values.map((v, i) => {
        const h = Math.max(2, Math.round((v / max) * 20))
        const isCurrent = i === values.length - 1
        return (
          <div
            key={i}
            className="rounded-[2px]"
            style={{
              width: '22px',
              height: `${h}px`,
              background: isCurrent ? activeColor : dimColor,
            }}
          />
        )
      })}
    </div>
  )
}

function TechRow({
  row,
  deltaLabel,
  activeColor,
  dimColor,
  deltaClass,
}: {
  row: TechTrendRow
  deltaLabel: string
  activeColor: string
  dimColor: string
  deltaClass: string
}) {
  return (
    <div className="flex items-center gap-2.5 py-[7px] border-b border-white/[0.05] last:border-0">
      <span className="flex-1 text-[13px] font-medium text-white truncate">{row.tech}</span>
      <TechSparkline
        values={[row.q1, row.q2, row.q3, row.q4]}
        activeColor={activeColor}
        dimColor={dimColor}
      />
      <span className={`text-[11px] font-medium min-w-[44px] text-right shrink-0 ${deltaClass}`}>
        {deltaLabel}
      </span>
    </div>
  )
}

const Q_LABELS = ['Q3', 'Q4', 'Q1', 'Q2']

function ColHeader({ deltaHeading }: { deltaHeading: string }) {
  return (
    <div className="flex items-center gap-2.5 pb-1.5 border-b border-white/[0.05] text-[10px] text-slate-500">
      <span className="flex-1">technology</span>
      <div className="flex gap-[3px] shrink-0">
        {Q_LABELS.map((q) => (
          <span key={q} className="w-[22px] text-center">
            {q}
          </span>
        ))}
      </div>
      <span className="min-w-[44px] text-right shrink-0">{deltaHeading}</span>
    </div>
  )
}

export function TechTrendsPanel() {
  const { data, isLoading } = useQuery({
    queryKey: dashboardQueryKeys.techTrends(),
    queryFn: fetchTechTrends,
    refetchInterval: 300_000,
  })

  return (
    <div className="bg-white/[0.04] border border-white/[0.07] rounded-xl p-5">
      <p className="text-[10px] font-medium uppercase tracking-[0.07em] text-slate-500 mb-4">
        tech stack trends — by company adoption
        {data ? ` · ${data.total_jobs} jobs analyzed` : ''}
      </p>

      {isLoading ? (
        <div className="h-40 animate-pulse bg-white/[0.03] rounded-lg" />
      ) : !data ? null : (
        <div className="flex items-stretch gap-0">
          {/* Rising column */}
          <div className="flex-1 pr-6">
            <p className="text-[13px] font-medium text-white mb-0.5">Rising</p>
            <p className="text-[10px] text-slate-500 pb-2 border-b border-white/[0.05]">
              &gt;5% of jobs · grew &gt;15% vs last quarter
            </p>
            <ColHeader deltaHeading="growth" />
            {data.rising.length === 0 ? (
              <p className="text-[11px] text-slate-600 py-3">No rising techs yet</p>
            ) : (
              data.rising.map((row) => (
                <TechRow
                  key={row.tech}
                  row={row}
                  deltaLabel={row.is_new ? 'New' : `+${Math.round(row.growth_pct)}%`}
                  activeColor="#534ab7"
                  dimColor="#221f4a"
                  deltaClass={row.is_new ? 'text-emerald-400' : 'text-violet-400'}
                />
              ))
            )}
          </div>

          {/* Vertical divider */}
          <div className="w-px bg-white/[0.07] self-stretch" />

          {/* New Entrants column */}
          <div className="flex-1 pl-6">
            <p className="text-[13px] font-medium text-white mb-0.5">New entrants</p>
            <p className="text-[10px] text-slate-500 pb-2 border-b border-white/[0.05]">
              &gt;5% of jobs · was &lt;1% two quarters ago
            </p>
            <ColHeader deltaHeading="% co." />
            {data.new_entrants.length === 0 ? (
              <p className="text-[11px] text-slate-600 py-3">No new entrants yet</p>
            ) : (
              data.new_entrants.map((row) => (
                <TechRow
                  key={row.tech}
                  row={row}
                  deltaLabel={`${Math.round(row.adoption_pct)}%`}
                  activeColor="#1d9e75"
                  dimColor="#083426"
                  deltaClass="text-slate-500"
                />
              ))
            )}
          </div>
        </div>
      )}
    </div>
  )
}
