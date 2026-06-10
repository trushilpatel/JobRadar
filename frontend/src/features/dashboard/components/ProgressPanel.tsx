import { useQuery } from '@tanstack/react-query'
import { fetchProgress } from '../api/dashboard.api'
import { dashboardQueryKeys } from '../queryKeys'
import { StatCol } from './StatCol'

export function ProgressPanel() {
  const { data, isLoading, isError } = useQuery({
    queryKey: dashboardQueryKeys.progress(),
    queryFn: fetchProgress,
    refetchInterval: 60_000,
  })

  const hasActivity =
    data && (data.applied_count > 0 || data.interviewing_count > 0 || data.offer_count > 0)

  return (
    <div className="bg-subtle border border-[var(--border)] rounded-xl p-5">
      <p className="text-[10px] font-medium uppercase tracking-[0.07em] text-muted mb-4">
        my progress
      </p>

      {isLoading ? (
        <div className="flex gap-4 animate-pulse">
          {Array.from({ length: 3 }).map((_, i) => (
            <div key={i} className="flex-1 h-14 bg-subtle rounded-lg" />
          ))}
        </div>
      ) : isError || !data ? (
        <p className="text-sm text-muted py-2">Failed to load progress data</p>
      ) : (
        <div className="flex items-stretch">
          <StatCol
            label="Applied"
            value={hasActivity ? data.applied_count : '—'}
            sub={`${data.awaiting_reply} awaiting reply`}
            valueClass="text-emerald-400"
          />
          <div className="w-px bg-[var(--border)] self-stretch" />
          <StatCol
            label="Interviewing"
            value={hasActivity ? data.interviewing_count : '—'}
            sub={`${data.interviewing_this_week} this week`}
            valueClass="text-violet-400"
          />
          <div className="w-px bg-[var(--border)] self-stretch" />
          <StatCol
            label="Offer"
            value={hasActivity ? data.offer_count : '—'}
            sub={
              data.offer_count === 1
                ? 'decision pending'
                : data.offer_count > 1
                  ? `${data.offer_count} decisions pending`
                  : 'no offers yet'
            }
            valueClass="text-amber-400"
          />
        </div>
      )}
    </div>
  )
}
