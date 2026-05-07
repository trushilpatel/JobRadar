import { ChevronLeft, ChevronRight, MoreHorizontal } from 'lucide-react'
import { cn } from '@/lib/utils'

interface PaginationProps {
  page: number
  total: number
  limit: number
  onPageChange: (page: number) => void
  onLimitChange?: (limit: number) => void
  limitOptions?: number[]
  className?: string
  loading?: boolean
}

const PAGE_SIZE_OPTIONS = [20, 50, 100]

export function Pagination({
  page,
  total,
  limit,
  onPageChange,
  onLimitChange,
  limitOptions = PAGE_SIZE_OPTIONS,
  className,
  loading,
}: PaginationProps) {
  const pageCount = Math.max(1, Math.ceil(total / limit))
  const from = total === 0 ? 0 : (page - 1) * limit + 1
  const to = Math.min(page * limit, total)

  // Build page number list with ellipsis
  function getPages(): (number | 'ellipsis')[] {
    if (pageCount <= 7) return Array.from({ length: pageCount }, (_, i) => i + 1)
    const pages: (number | 'ellipsis')[] = [1]
    if (page > 3) pages.push('ellipsis')
    for (let i = Math.max(2, page - 1); i <= Math.min(pageCount - 1, page + 1); i++) {
      pages.push(i)
    }
    if (page < pageCount - 2) pages.push('ellipsis')
    pages.push(pageCount)
    return pages
  }

  return (
    <div className={cn('flex items-center justify-end gap-3 px-4 py-2.5', className)}>
      {/* Showing label */}
      <p className="text-xs text-slate-500 shrink-0 mr-1">
        {loading ? null : total === 0 ? 'No results' : `Showing ${from}–${to} of ${total}`}
      </p>

      {/* Page controls */}
      <div className="flex items-center gap-1">
        <button
          onClick={() => onPageChange(page - 1)}
          disabled={page <= 1}
          className="flex items-center gap-1 px-2.5 py-1.5 text-xs text-slate-400 hover:text-white hover:bg-white/[0.06] rounded-lg transition-colors disabled:opacity-30 disabled:cursor-not-allowed"
        >
          <ChevronLeft size={13} />
          Prev
        </button>

        <div className="flex items-center gap-0.5">
          {getPages().map((p, i) =>
            p === 'ellipsis' ? (
              <span key={`ellipsis-${i}`} className="px-2 py-1.5 text-xs text-slate-600">
                <MoreHorizontal size={13} />
              </span>
            ) : (
              <button
                key={p}
                onClick={() => onPageChange(p)}
                className={cn(
                  'min-w-[28px] px-2 py-1.5 text-xs rounded-lg transition-colors',
                  p === page
                    ? 'bg-violet-600 text-white font-medium'
                    : 'text-slate-400 hover:text-white hover:bg-white/[0.06]'
                )}
              >
                {p}
              </button>
            )
          )}
        </div>

        <button
          onClick={() => onPageChange(page + 1)}
          disabled={page >= pageCount}
          className="flex items-center gap-1 px-2.5 py-1.5 text-xs text-slate-400 hover:text-white hover:bg-white/[0.06] rounded-lg transition-colors disabled:opacity-30 disabled:cursor-not-allowed"
        >
          Next
          <ChevronRight size={13} />
        </button>
      </div>

      {/* Rows selector */}
      {onLimitChange && (
        <div className="flex items-center gap-2 shrink-0">
          <span className="text-xs text-slate-500">Rows</span>
          <select
            value={limit}
            onChange={e => {
              onLimitChange(Number(e.target.value))
              onPageChange(1)
            }}
            className="bg-white/[0.04] border border-white/[0.08] text-xs text-slate-300 rounded-lg px-2 py-1.5 focus:outline-none focus:border-violet-500 transition-colors cursor-pointer"
          >
            {limitOptions.map(n => (
              <option key={n} value={n}>{n}</option>
            ))}
          </select>
        </div>
      )}
    </div>
  )
}
