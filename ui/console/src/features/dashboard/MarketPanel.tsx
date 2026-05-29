import { useQuery } from '@tanstack/react-query'
import { useNavigate } from 'react-router-dom'
import { fetchMarket } from '@/api/dashboard'
import { dashboardQueryKeys } from './queryKeys'
import { StatCol } from './components/StatCol'

export function MarketPanel() {
  const navigate = useNavigate()
  const { data, isLoading, isError } = useQuery({
    queryKey: dashboardQueryKeys.market(),
    queryFn: fetchMarket,
    refetchInterval: 60_000,
  })

  const weeklyDeltaLabel = data
    ? data.weekly_delta >= 0
      ? `+${data.weekly_delta} this week`
      : `${data.weekly_delta} this week`
    : '—'

  const handleNavClick = () => navigate('/jobs', { state: { dateFilter: '24h' } })
  const handleNavKeyDown = (e: React.KeyboardEvent) => {
    if (e.key === 'Enter' || e.key === ' ') {
      e.preventDefault()
      handleNavClick()
    }
  }

  return (
    <div className="bg-white/[0.04] border border-white/[0.07] rounded-xl p-5">
      <p className="text-[10px] font-medium uppercase tracking-[0.07em] text-slate-500 mb-4">
        market
      </p>

      {isLoading ? (
        <div className="flex gap-4 animate-pulse">
          {Array.from({ length: 2 }).map((_, i) => (
            <div key={i} className="flex-1 h-14 bg-white/[0.04] rounded-lg" />
          ))}
        </div>
      ) : isError || !data ? (
        <p className="text-sm text-slate-500 py-2">Failed to load market data</p>
      ) : (
        <div className="flex items-stretch">
          <StatCol
            label="New jobs"
            value={data.new_since_yesterday}
            sub="since yesterday"
            valueClass="text-blue-400"
            onClick={handleNavClick}
            onKeyDown={handleNavKeyDown}
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
