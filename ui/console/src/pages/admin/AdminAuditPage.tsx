import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { ChevronLeft, ChevronRight } from 'lucide-react'
import { fetchAuditLog, type AuditFilter } from '@/api/adminAudit'

export function AdminAuditPage() {
  const [filter, setFilter] = useState<AuditFilter>({})
  const [actorInput, setActorInput] = useState('')
  const [actionInput, setActionInput] = useState('')
  const [page, setPage] = useState(1)
  const limit = 50

  const { data, isLoading } = useQuery({
    queryKey: ['admin-audit', filter, page],
    queryFn: () => fetchAuditLog(filter, page, limit),
    staleTime: 15_000,
  })

  function applyFilters() {
    setFilter({
      actor_user_id: actorInput.trim() || undefined,
      action: actionInput.trim() || undefined,
    })
    setPage(1)
  }

  const entries = data?.data ?? []
  const total = data?.total ?? 0
  const totalPages = Math.ceil(total / limit)

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
          onKeyDown={(e) => e.key === 'Enter' && applyFilters()}
          className="w-64 rounded-md bg-white/[0.04] border border-white/[0.1] px-3 py-1.5 text-sm text-white placeholder:text-slate-600 focus:outline-none focus:ring-1 focus:ring-violet-500"
        />
        <input
          type="text"
          placeholder="Action (e.g. user.delete)…"
          value={actionInput}
          onChange={(e) => setActionInput(e.target.value)}
          onKeyDown={(e) => e.key === 'Enter' && applyFilters()}
          className="w-56 rounded-md bg-white/[0.04] border border-white/[0.1] px-3 py-1.5 text-sm text-white placeholder:text-slate-600 focus:outline-none focus:ring-1 focus:ring-violet-500"
        />
        <button
          onClick={applyFilters}
          className="px-3 py-1.5 rounded-md text-xs font-medium bg-white/[0.06] text-slate-300 hover:bg-white/[0.1] transition-colors"
        >
          Apply
        </button>
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
                  <td className="px-4 py-2.5 text-xs text-slate-400 truncate max-w-[200px]" title={e.actor_user_id ?? undefined}>
                    {e.actor_email ?? e.actor_user_id ?? '—'}
                  </td>
                  <td className="px-4 py-2.5 text-xs text-violet-300 font-medium">{e.action}</td>
                  <td className="px-4 py-2.5 text-xs text-slate-500" title={e.target_id ?? undefined}>
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

      {/* Pagination */}
      {totalPages > 1 && (
        <div className="border-t border-white/[0.07] px-6 py-3 flex items-center justify-between shrink-0">
          <button
            onClick={() => setPage((p) => Math.max(1, p - 1))}
            disabled={page === 1}
            className="flex items-center gap-1 px-3 py-1.5 rounded-md text-xs text-slate-400 hover:text-white hover:bg-white/[0.05] disabled:opacity-40 disabled:cursor-not-allowed transition-colors"
          >
            <ChevronLeft size={13} /> Prev
          </button>
          <span className="text-xs text-slate-500">
            Page {page} of {totalPages}
          </span>
          <button
            onClick={() => setPage((p) => Math.min(totalPages, p + 1))}
            disabled={page === totalPages}
            className="flex items-center gap-1 px-3 py-1.5 rounded-md text-xs text-slate-400 hover:text-white hover:bg-white/[0.05] disabled:opacity-40 disabled:cursor-not-allowed transition-colors"
          >
            Next <ChevronRight size={13} />
          </button>
        </div>
      )}
    </div>
  )
}
