import { useQuery } from '@tanstack/react-query'
import { fetchPipelineStats } from '@/api/dashboard'

function Stat({ label, value, accent }: { label: string; value: number; accent?: string }) {
  return (
    <div className="rounded-lg border border-white/[0.07] bg-white/[0.02] p-4">
      <p className="text-xs text-slate-500 mb-1">{label}</p>
      <p className={`text-2xl font-semibold ${accent ?? 'text-white'}`}>{value.toLocaleString()}</p>
    </div>
  )
}

export function AdminPipelinePage() {
  const { data, isLoading } = useQuery({
    queryKey: ['admin-pipeline-stats'],
    queryFn: fetchPipelineStats,
    staleTime: 60_000,
    refetchInterval: 60_000,
  })

  if (isLoading || !data) {
    return (
      <div className="flex items-center justify-center h-32 text-slate-500 text-sm">
        Loading…
      </div>
    )
  }

  return (
    <div className="flex flex-col h-full bg-[#0d0d14] overflow-y-auto">
      <div className="border-b border-white/[0.07] px-6 py-4 shrink-0">
        <h1 className="text-lg font-semibold text-white">Pipeline</h1>
      </div>

      <div className="p-6 grid grid-cols-2 sm:grid-cols-4 gap-4">
        <Stat label="Total companies" value={data.total_companies} />
        <Stat label="Active companies" value={data.active_companies} accent="text-emerald-400" />
        <Stat label="Queued" value={data.queued} />
        <Stat label="Crawling" value={data.crawling} accent="text-violet-300" />
        <Stat label="Awaiting filter" value={data.awaiting_filter} />
        <Stat label="Awaiting enrichment" value={data.awaiting_enrichment} />
        <Stat label="New jobs (24h)" value={data.new_jobs_24h} accent="text-emerald-400" />
        <Stat label="Errors (24h)" value={data.errors_24h} accent={data.errors_24h > 0 ? 'text-red-400' : undefined} />
      </div>
    </div>
  )
}
