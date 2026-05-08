import { ChevronLeft, ChevronRight, MoreHorizontal } from 'lucide-react'
import { cn } from '@/lib/utils'

const PAGE_SIZE_OPTIONS = [20, 50, 100]

interface CommonProps {
  limit: number
  onLimitChange?: (limit: number) => void
  limitOptions?: number[]
  className?: string
  loading?: boolean
}

interface OffsetProps extends CommonProps {
  mode?: 'offset'
  page: number
  total: number
  onPageChange: (page: number) => void
}

interface CursorProps extends CommonProps {
  mode: 'cursor'
  page: number
  hasNext: boolean
  hasPrev: boolean
  itemsOnPage: number
  onNext: () => void
  onPrev: () => void
  // Optional: when known (e.g. unfiltered tab counts), enables "of N" display
  knownTotal?: number
}

export type PaginationProps = OffsetProps | CursorProps

export function Pagination(props: PaginationProps) {
  if (props.mode === 'cursor') return <CursorPagination {...props} />
  return <OffsetPagination {...props} />
}

function OffsetPagination({
  page,
  total,
  limit,
  onPageChange,
  onLimitChange,
  limitOptions = PAGE_SIZE_OPTIONS,
  className,
  loading,
}: OffsetProps) {
  const pageCount = Math.max(1, Math.ceil(total / limit))
  const from = total === 0 ? 0 : (page - 1) * limit + 1
  const to = Math.min(page * limit, total)

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
    <Shell className={className}>
      <Status
        loading={loading}
        text={total === 0 ? 'No results' : `Showing ${from}–${to} of ${total.toLocaleString()}`}
      />

      <div className="flex items-center gap-1">
        <PrevButton disabled={page <= 1} onClick={() => onPageChange(page - 1)} />

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

        <NextButton disabled={page >= pageCount} onClick={() => onPageChange(page + 1)} />
      </div>

      <RowsSelector
        limit={limit}
        limitOptions={limitOptions}
        onLimitChange={onLimitChange}
        onResetPage={() => onPageChange(1)}
      />
    </Shell>
  )
}

function CursorPagination({
  page,
  limit,
  itemsOnPage,
  hasNext,
  hasPrev,
  knownTotal,
  onNext,
  onPrev,
  onLimitChange,
  limitOptions = PAGE_SIZE_OPTIONS,
  className,
  loading,
}: CursorProps) {
  const from = itemsOnPage === 0 ? 0 : (page - 1) * limit + 1
  const to = (page - 1) * limit + itemsOnPage

  let text: string
  if (itemsOnPage === 0 && !hasPrev) text = 'No results'
  else if (knownTotal != null)
    text = `Showing ${from}–${to} of ${knownTotal.toLocaleString()}`
  else text = `Showing ${from}–${to}`

  return (
    <Shell className={className}>
      <Status loading={loading} text={text} />

      <div className="flex items-center gap-1">
        <PrevButton disabled={!hasPrev} onClick={onPrev} />
        <span className="px-2.5 py-1.5 text-xs text-slate-400">Page {page}</span>
        <NextButton disabled={!hasNext} onClick={onNext} />
      </div>

      <RowsSelector
        limit={limit}
        limitOptions={limitOptions}
        onLimitChange={onLimitChange}
      />
    </Shell>
  )
}

function Shell({ children, className }: { children: React.ReactNode; className?: string }) {
  return (
    <div className={cn('flex items-center justify-center gap-3 px-4 py-2.5', className)}>
      {children}
    </div>
  )
}

function Status({ loading, text }: { loading?: boolean; text: string }) {
  return (
    <p className="text-xs text-slate-500 shrink-0 mr-1">{loading ? null : text}</p>
  )
}

function PrevButton({ disabled, onClick }: { disabled: boolean; onClick: () => void }) {
  return (
    <button
      onClick={onClick}
      disabled={disabled}
      className="flex items-center gap-1 px-2.5 py-1.5 text-xs text-slate-400 hover:text-white hover:bg-white/[0.06] rounded-lg transition-colors disabled:opacity-30 disabled:cursor-not-allowed"
    >
      <ChevronLeft size={13} />
      Prev
    </button>
  )
}

function NextButton({ disabled, onClick }: { disabled: boolean; onClick: () => void }) {
  return (
    <button
      onClick={onClick}
      disabled={disabled}
      className="flex items-center gap-1 px-2.5 py-1.5 text-xs text-slate-400 hover:text-white hover:bg-white/[0.06] rounded-lg transition-colors disabled:opacity-30 disabled:cursor-not-allowed"
    >
      Next
      <ChevronRight size={13} />
    </button>
  )
}

function RowsSelector({
  limit,
  limitOptions,
  onLimitChange,
  onResetPage,
}: {
  limit: number
  limitOptions: number[]
  onLimitChange?: (limit: number) => void
  onResetPage?: () => void
}) {
  if (!onLimitChange) return null
  return (
    <div className="flex items-center gap-2 shrink-0">
      <span className="text-xs text-slate-500">Rows</span>
      <select
        value={limit}
        onChange={(e) => {
          onLimitChange(Number(e.target.value))
          onResetPage?.()
        }}
        className="bg-white/[0.04] border border-white/[0.08] text-xs text-slate-300 rounded-lg px-2 py-1.5 focus:outline-none focus:border-violet-500 transition-colors cursor-pointer"
      >
        {limitOptions.map((n) => (
          <option key={n} value={n}>
            {n}
          </option>
        ))}
      </select>
    </div>
  )
}
