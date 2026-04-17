import { useQuery } from '@tanstack/react-query'
import { fetchVelocity } from '@/api/dashboard'
import type { CompanyVelocityRow } from '@/types'
import { cn } from '@/lib/utils'

const BADGE: Record<string, string> = {
  spike: 'bg-amber-500/15 text-amber-400',
  rising: 'bg-emerald-500/15 text-emerald-400',
  normal: 'bg-white/[0.06] text-slate-400',
}

function Sparkline({ row }: { row: CompanyVelocityRow }) {
  const bars = [row.week_3, row.week_2, row.week_1, row.week_current]
  const max = Math.max(...bars, 1)
  return (
    <div className="flex items-end gap-[2px]" style={{ height: '22px' }}>
      {bars.map((v, i) => {
        const h = Math.max(3, Math.round((v / max) * 22))
        const isCurrent = i === bars.length - 1
        return (
          <div
            key={i}
            className="rounded-[2px]"
            style={{
              width: '8px',
              height: `${h}px`,
              background: isCurrent ? '#1d9e75' : '#1c3c34',
            }}
          />
        )
      })}
    </div>
  )
}

export function VelocityPanel() {
  const { data: rows = [], isLoading } = useQuery({
    queryKey: ['dashboard-velocity'],
    queryFn: () => fetchVelocity(10),
    refetchInterval: 120_000,
  })

  return (
    <div className="bg-white/[0.04] border border-white/[0.07] rounded-xl p-5">
      <p className="text-[10px] font-medium uppercase tracking-[0.07em] text-slate-500 mb-4">
        top companies by velocity
      </p>

      {/* Header row */}
      <div
        className="grid text-[10px] text-slate-500 pb-2 border-b border-white/[0.05]"
        style={{ gridTemplateColumns: '1fr 60px 50px 36px', gap: '12px' }}
      >
        <span>company</span>
        <span>velocity</span>
        <span className="text-center">4 weeks</span>
        <span className="text-right">new</span>
      </div>

      {isLoading ? (
        <div className="space-y-3 mt-2 animate-pulse">
          {[...Array(5)].map((_, i) => (
            <div key={i} className="h-8 bg-white/[0.04] rounded-lg" />
          ))}
        </div>
      ) : rows.length === 0 ? (
        <p className="text-sm text-slate-600 py-4">No velocity data yet</p>
      ) : (
        <div>
          {rows.map((row, i) => (
            <div
              key={row.company_id}
              className={cn(
                'grid items-center py-2.5',
                i < rows.length - 1 && 'border-b border-white/[0.05]'
              )}
              style={{ gridTemplateColumns: '1fr 60px 50px 36px', gap: '12px' }}
            >
              <span className="text-[13px] font-medium text-white truncate">
                {row.company_name}
              </span>
              <span
                className={cn(
                  'inline-flex justify-center px-2 py-0.5 rounded-full text-[11px] font-medium',
                  BADGE[row.velocity_tag]
                )}
              >
                {row.velocity_tag === 'spike'
                  ? `${row.multiplier}×`
                  : row.velocity_tag === 'rising'
                    ? `${row.multiplier}×`
                    : '—'}
              </span>
              <div className="flex justify-center">
                <Sparkline row={row} />
              </div>
              <span className="text-[13px] font-medium text-emerald-400 text-right tabular-nums">
                +{row.week_current}
              </span>
            </div>
          ))}
        </div>
      )}
    </div>
  )
}
