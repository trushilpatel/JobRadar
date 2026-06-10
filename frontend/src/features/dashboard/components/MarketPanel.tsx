import { useQuery } from '@tanstack/react-query'
import { useNavigate } from 'react-router-dom'
import { fetchMarket } from '../api/dashboard.api'
import { dashboardQueryKeys } from '../queryKeys'
import { routes } from '@/config/routes'
import { StatCol } from './StatCol'

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

  const handleNavClick = () => navigate(routes.jobs({ since: '24h' }))
  const handleNavKeyDown = (e: React.KeyboardEvent) => {
    if (e.key === 'Enter' || e.key === ' ') {
      e.preventDefault()
      handleNavClick()
    }
  }

  return (
    <div className="bg-subtle border border-[var(--border)] rounded-xl p-5">
      <p className="text-[10px] font-medium uppercase tracking-[0.07em] text-muted mb-4">market</p>

      {isLoading ? (
        <div className="flex gap-4 animate-pulse">
          {Array.from({ length: 2 }).map((_, i) => (
            <div key={i} className="flex-1 h-14 bg-subtle rounded-lg" />
          ))}
        </div>
      ) : isError || !data ? (
        <p className="text-sm text-muted py-2">Failed to load market data</p>
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
          <div className="w-px bg-[var(--border)] self-stretch" />
          <StatCol
            label="Total open"
            value={data.total_open}
            sub={weeklyDeltaLabel}
            valueClass="text-primary"
          />
        </div>
      )}
    </div>
  )
}
