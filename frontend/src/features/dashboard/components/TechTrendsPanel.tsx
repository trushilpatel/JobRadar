import { useQuery } from '@tanstack/react-query'
import { fetchTechTrends } from '../api/dashboard.api'
import type { TechTrendRow } from '@/types'
import { dashboardQueryKeys } from '../queryKeys'

// Sparkline for a tech row — 4 monthly bars
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
    <div className="flex items-end gap-[3px] h-5 shrink-0">
      {values.map((v, i) => {
        const h = Math.max(2, Math.round((v / max) * 20))
        const isCurrent = i === values.length - 1
        return (
          <div
            key={i}
            className="rounded-[2px] w-[22px]"
            style={{ height: `${h}px`, background: isCurrent ? activeColor : dimColor }}
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
      <span className="flex-1 min-w-0 text-[13px] font-medium text-white truncate">{row.tech}</span>
      <TechSparkline
        values={[row.m1, row.m2, row.m3, row.m4]}
        activeColor={activeColor}
        dimColor={dimColor}
      />
      <span className={`text-[11px] font-medium min-w-[44px] text-right shrink-0 ${deltaClass}`}>
        {deltaLabel}
      </span>
    </div>
  )
}

const M_LABELS = ['M1', 'M2', 'M3', 'M4']

function ColHeader({ deltaHeading }: { deltaHeading: string }) {
  return (
    <div className="flex items-center gap-2.5 pb-1.5 border-b border-white/[0.05] text-[10px] text-slate-500">
      <span className="flex-1">technology</span>
      <div className="flex gap-[3px] shrink-0">
        {M_LABELS.map((m) => (
          <span key={m} className="w-[22px] text-center">
            {m}
          </span>
        ))}
      </div>
      <span className="min-w-[44px] text-right shrink-0">{deltaHeading}</span>
    </div>
  )
}

export function TechTrendsPanel() {
  const { data, isLoading, isError } = useQuery({
    queryKey: dashboardQueryKeys.techTrends(),
    queryFn: fetchTechTrends,
    refetchInterval: 300_000,
  })

  return (
    <div className="bg-white/[0.04] border border-white/[0.07] rounded-xl p-5">
      <p className="text-[10px] font-medium uppercase tracking-[0.07em] text-slate-500 mb-4">
        tech stack trends — by job demand
        {data ? ` · ${data.total_jobs} jobs analyzed` : ''}
      </p>

      {isLoading ? (
        <div className="h-40 animate-pulse bg-white/[0.03] rounded-lg" />
      ) : isError ? (
        <p className="text-sm text-slate-500 py-4">Failed to load tech trends</p>
      ) : !data ? (
        <p className="text-sm text-slate-600 py-4">No tech data yet</p>
      ) : (
        <div className="flex items-stretch gap-0">
          {/* Rising column */}
          <div className="flex-1 pr-6">
            <p className="text-[13px] font-medium text-white mb-0.5">Rising</p>
            <p className="text-[10px] text-slate-500 pb-2 border-b border-white/[0.05]">
              ≥2% of jobs · ≥10 jobs · grew &gt;15% vs prev month
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
              ≥2% of jobs · ≥10 jobs · was &lt;1% two months ago
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
