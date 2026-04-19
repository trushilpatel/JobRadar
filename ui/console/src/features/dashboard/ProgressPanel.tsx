import { useQuery } from '@tanstack/react-query'
import { fetchProgress } from '@/api/dashboard'
import { cn } from '@/lib/utils'

function StatCol({
  label,
  value,
  sub,
  valueClass,
}: {
  label: string
  value: number
  sub: string
  valueClass: string
}) {
  return (
    <div className="flex-1 flex flex-col gap-1 px-5 first:pl-0 last:pr-0">
      <span className="text-[10px] font-medium uppercase tracking-[0.07em] text-slate-500">
        {label}
      </span>
      <span className={cn('text-[36px] font-medium leading-none tabular-nums', valueClass)}>
        {value}
      </span>
      <span className="text-[11px] text-slate-500">{sub}</span>
    </div>
  )
}

export function ProgressPanel() {
  const { data, isLoading } = useQuery({
    queryKey: ['dashboard-progress'],
    queryFn: fetchProgress,
    refetchInterval: 60_000,
  })

  return (
    <div className="bg-white/[0.04] border border-white/[0.07] rounded-xl p-5">
      <p className="text-[10px] font-medium uppercase tracking-[0.07em] text-slate-500 mb-4">
        my progress
      </p>

      {isLoading || !data ? (
        <div className="flex gap-4 animate-pulse">
          {[...Array(3)].map((_, i) => (
            <div key={i} className="flex-1 h-14 bg-white/[0.04] rounded-lg" />
          ))}
        </div>
      ) : (
        <div className="flex items-stretch">
          <StatCol
            label="Applied"
            value={data.applied_count}
            sub={`${data.awaiting_reply} awaiting reply`}
            valueClass="text-emerald-400"
          />
          <div className="w-px bg-white/[0.07] self-stretch" />
          <StatCol
            label="Interviewing"
            value={data.interviewing_count}
            sub={`${data.interviewing_this_week} this week`}
            valueClass="text-violet-400"
          />
          <div className="w-px bg-white/[0.07] self-stretch" />
          <StatCol
            label="Offer"
            value={data.offer_count}
            sub="decision pending"
            valueClass="text-amber-400"
          />
        </div>
      )}
    </div>
  )
}
