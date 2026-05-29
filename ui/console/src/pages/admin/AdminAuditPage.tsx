import { useMemo, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { fetchAuditLog, type AuditFilter } from '@/features/auth'
import { useDebouncedValue } from '@/lib/hooks/use-debounced-value'
import { Pagination } from '@/components/ui/pagination'

export function AdminAuditPage() {
  const [actorInput, setActorInput] = useState('')
  const [actionInput, setActionInput] = useState('')
  const debouncedActor = useDebouncedValue(actorInput, 300)
  const debouncedAction = useDebouncedValue(actionInput, 300)
  const [page, setPage] = useState(1)
  const [limit, setLimit] = useState(50)

  const filter = useMemo<AuditFilter>(
    () => ({
      actor_user_id: debouncedActor.trim() || undefined,
      action: debouncedAction.trim() || undefined,
    }),
    [debouncedActor, debouncedAction]
  )

  const [prevKey, setPrevKey] = useState({ filter, limit })
  if (prevKey.filter !== filter || prevKey.limit !== limit) {
    setPrevKey({ filter, limit })
    setPage(1)
  }

  const { data, isLoading } = useQuery({
    queryKey: ['admin-audit', filter, page, limit],
    queryFn: () => fetchAuditLog(filter, page, limit),
    staleTime: 15_000,
  })

  const entries = data?.data ?? []
  const total = data?.total ?? 0

  return (
    <div className="flex flex-col h-full bg-[#0d0d14] overflow-y-auto">
      <div className="border-b border-white/[0.07] px-6 py-4 shrink-0">
        <h1 className="text-lg font-semibold text-white">Audit Log</h1>
      </div>

      {/* Filters */}
      <div className="px-6 py-3 border-b border-white/[0.07] flex items-center gap-3 shrink-0 flex-wrap">
        <input
          type="text"
          placeholder="Actor user ID or email…"
          value={actorInput}
          onChange={(e) => setActorInput(e.target.value)}
          className="w-64 rounded-md bg-white/[0.04] border border-white/[0.1] px-3 py-1.5 text-sm text-white placeholder:text-slate-600 focus:outline-none focus:ring-1 focus:ring-violet-500"
        />
        <input
          type="text"
          placeholder="Action (e.g. user.delete)…"
          value={actionInput}
          onChange={(e) => setActionInput(e.target.value)}
          className="w-56 rounded-md bg-white/[0.04] border border-white/[0.1] px-3 py-1.5 text-sm text-white placeholder:text-slate-600 focus:outline-none focus:ring-1 focus:ring-violet-500"
        />
        <span className="text-xs text-slate-500 ml-auto">{total} entries</span>
      </div>

      {/* Table */}
      <div className="flex-1 overflow-auto">
        {isLoading ? (
          <div className="flex items-center justify-center h-32 text-slate-500 text-sm">
            Loading…
          </div>
        ) : entries.length === 0 ? (
          <div className="flex items-center justify-center h-32 text-slate-500 text-sm">
            No entries found.
          </div>
        ) : (
          <table className="w-full text-sm">
            <thead>
              <tr className="border-b border-white/[0.07] text-xs text-slate-500 text-left">
                <th className="px-4 py-2 font-medium">Time</th>
                <th className="px-4 py-2 font-medium">Actor</th>
                <th className="px-4 py-2 font-medium">Action</th>
                <th className="px-4 py-2 font-medium">Target</th>
              </tr>
            </thead>
            <tbody>
              {entries.map((e) => (
                <tr
                  key={e.id}
                  className="border-b border-white/[0.05] hover:bg-white/[0.02] transition-colors"
                >
                  <td className="px-4 py-2.5 text-xs text-slate-500 whitespace-nowrap">
                    {new Date(e.created_at).toLocaleString()}
                  </td>
                  <td
                    className="px-4 py-2.5 text-xs text-slate-400 truncate max-w-[200px]"
                    title={e.actor_user_id ?? undefined}
                  >
                    {e.actor_email ?? e.actor_user_id ?? '—'}
                  </td>
                  <td className="px-4 py-2.5 text-xs text-violet-300 font-medium">{e.action}</td>
                  <td
                    className="px-4 py-2.5 text-xs text-slate-500"
                    title={e.target_id ?? undefined}
                  >
                    {e.target_type
                      ? e.target_id
                        ? `${e.target_type} · ${e.target_id.slice(0, 8)}…`
                        : e.target_type
                      : '—'}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </div>

      <div className="border-t border-white/[0.07] shrink-0">
        <Pagination
          mode="offset"
          page={page}
          total={total}
          limit={limit}
          onPageChange={setPage}
          onLimitChange={setLimit}
          loading={isLoading}
        />
      </div>
    </div>
  )
}
