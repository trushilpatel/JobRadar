import { useQuery } from '@tanstack/react-query'
import { fetchVelocity } from '../api/dashboard.api'
import type { CompanyVelocityRow } from '@/types'
import { cn } from '@/lib/utils/utils'
import { dashboardQueryKeys } from '../queryKeys'

const BADGE: Record<string, string> = {
  spike: 'bg-amber-500/15 text-amber-400',
  rising: 'bg-emerald-500/15 text-emerald-400',
  normal: 'bg-subtle text-muted',
}

function Sparkline({ row }: { row: CompanyVelocityRow }) {
  const bars = [row.week_3, row.week_2, row.week_1, row.week_current]
  const max = Math.max(...bars, 1)
  return (
    <div
      className="flex items-end gap-[2px] h-[22px]"
      role="img"
      aria-label={`4-week trend: ${bars.join(', ')}`}
    >
      {bars.map((v, i) => {
        const h = Math.max(3, Math.round((v / max) * 22))
        const isCurrent = i === bars.length - 1
        return (
          <div
            key={i}
            className="rounded-[2px] w-2"
            style={{
              height: `${h}px`,
              background: isCurrent ? '#1d9e75' : 'var(--sparkline-bar-prev)',
            }}
          />
        )
      })}
    </div>
  )
}

export function VelocityPanel() {
  const {
    data: rows,
    isLoading,
    isError,
  } = useQuery({
    queryKey: dashboardQueryKeys.velocity(),
    queryFn: () => fetchVelocity(10),
    refetchInterval: 120_000,
  })

  return (
    <div className="bg-subtle border border-[var(--border)] rounded-xl p-5">
      <p className="text-[10px] font-medium uppercase tracking-[0.07em] text-muted mb-4">
        top companies by velocity
      </p>

      <div className="grid text-[10px] text-muted pb-2 border-b border-[var(--border)] [grid-template-columns:1fr_60px_50px_36px] gap-3">
        <span>company</span>
        <span>velocity</span>
        <span className="text-center">4 weeks</span>
        <span className="text-right">new</span>
      </div>

      {isLoading ? (
        <div className="space-y-3 mt-2 animate-pulse">
          {Array.from({ length: 5 }).map((_, i) => (
            <div key={i} className="h-8 bg-subtle rounded-lg" />
          ))}
        </div>
      ) : isError ? (
        <p className="text-sm text-muted py-4">Failed to load velocity data</p>
      ) : !rows || rows.length === 0 ? (
        <p className="text-sm text-placeholder py-4">No velocity data yet</p>
      ) : (
        <div>
          {rows.map((row, i) => (
            <div
              key={row.company_id}
              className={cn(
                'grid items-center py-2.5 [grid-template-columns:1fr_60px_50px_36px] gap-3',
                i < rows.length - 1 && 'border-b border-[var(--border)]'
              )}
            >
              <span className="text-[13px] font-medium text-primary truncate">
                {row.company_name}
              </span>
              <span
                className={cn(
                  'inline-flex justify-center px-2 py-0.5 rounded-full text-[11px] font-medium',
                  BADGE[row.velocity_tag]
                )}
              >
                {row.velocity_tag !== 'normal' ? `${row.multiplier}×` : '—'}
              </span>
              <div className="flex justify-center">
                <Sparkline row={row} />
              </div>
              <span className="text-[13px] font-medium text-emerald-400 text-right tabular-nums">
                {row.week_current > 0
                  ? `+${row.week_current}`
                  : row.week_current === 0
                    ? '—'
                    : row.week_current}
              </span>
            </div>
          ))}
        </div>
      )}
    </div>
  )
}
