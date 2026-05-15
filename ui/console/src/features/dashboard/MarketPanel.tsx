import { useQuery } from '@tanstack/react-query'
import { useNavigate } from 'react-router-dom'
import { fetchMarket } from '@/api/dashboard'
import { cn } from '@/lib/utils'
import { dashboardQueryKeys } from './queryKeys'

function StatCol({
  label,
  value,
  sub,
  valueClass,
  onClick,
}: {
  label: string
  value: number
  sub: string
  valueClass: string
  onClick?: () => void
}) {
  return (
    <div
      className={cn(
        'flex-1 flex flex-col gap-1 px-5 first:pl-0 last:pr-0',
        onClick && 'cursor-pointer group'
      )}
      onClick={onClick}
    >
      <span className="text-[10px] font-medium uppercase tracking-[0.07em] text-slate-500">
        {label}
      </span>
      <span
        className={cn(
          'text-[36px] font-medium leading-none tabular-nums',
          valueClass,
          onClick && 'group-hover:opacity-80 transition-opacity'
        )}
      >
        {value}
      </span>
      <span
        className={cn(
          'text-[11px] text-slate-500',
          onClick && 'group-hover:text-slate-400 transition-colors'
        )}
      >
        {sub}
      </span>
    </div>
  )
}

export function MarketPanel() {
  const navigate = useNavigate()
  const { data, isLoading } = useQuery({
    queryKey: dashboardQueryKeys.market(),
    queryFn: fetchMarket,
    refetchInterval: 60_000,
  })

  const weeklyDeltaLabel = data
    ? data.weekly_delta >= 0
      ? `+${data.weekly_delta} this week`
      : `${data.weekly_delta} this week`
    : '—'

  return (
    <div className="bg-white/[0.04] border border-white/[0.07] rounded-xl p-5">
      <p className="text-[10px] font-medium uppercase tracking-[0.07em] text-slate-500 mb-4">
        market
      </p>

      {isLoading || !data ? (
        <div className="flex gap-4 animate-pulse">
          {[...Array(2)].map((_, i) => (
            <div key={i} className="flex-1 h-14 bg-white/[0.04] rounded-lg" />
          ))}
        </div>
      ) : (
        <div className="flex items-stretch">
          <StatCol
            label="New jobs"
            value={data.new_since_yesterday}
            sub="since yesterday"
            valueClass="text-blue-400"
            onClick={() => navigate('/jobs', { state: { dateFilter: '24h' } })}
          />
          <div className="w-px bg-white/[0.07] self-stretch" />
          <StatCol
            label="Total open"
            value={data.total_open}
            sub={weeklyDeltaLabel}
            valueClass="text-white"
          />
        </div>
      )}
    </div>
  )
}
