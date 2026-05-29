import { ChevronLeft, ChevronRight, ChevronsLeft, MoreHorizontal } from 'lucide-react'
import { cn } from '@/lib/utils/utils'

/**
 * Unified pagination primitive used across the app.
 *
 * Two modes share a single compact look:
 *  - cursor: the only "position" the user has is the range itself, so the
 *    range sits INSIDE the navigation cluster between the chevrons. There's
 *    no numbered-page row because cursor pagination can't jump to an
 *    arbitrary page.
 *  - offset: numbered pages occupy the center of the cluster (they're the
 *    true "where am I"); the range sits on the left as supporting context.
 *
 * Both modes:
 *  - drop the word "Prev" / "Next" — icon-only chevrons
 *  - never render a "last page" button (cursor can't reach it cheaply; offset
 *    has explicit page numbers so it isn't needed)
 *  - optional `onFirst` — when provided, renders `«` (jump to page 1)
 */

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
  /** Optional — when provided, renders the `«` jump-to-first button. */
  onFirst?: () => void
}

interface CursorProps extends CommonProps {
  mode: 'cursor'
  page: number
  hasNext: boolean
  hasPrev: boolean
  itemsOnPage: number
  /**
   * Cumulative items in pages before this one. Drives the "X–Y" range
   * accurately even when pages are sparse (per-user overlay can return
   * fewer than `limit` items per cursor page).
   */
  itemsBeforePage: number
  onNext: () => void
  onPrev: () => void
  /** Optional — when provided, renders the `«` jump-to-first button. */
  onFirst?: () => void
  /** Optional; when provided, enables "of N" display in the range text. */
  knownTotal?: number
}

export type PaginationProps = OffsetProps | CursorProps

export function Pagination(props: PaginationProps) {
  if (props.mode === 'cursor') return <CursorPagination {...props} />
  return <OffsetPagination {...props} />
}

// ── Cursor mode (V4: range inside the cluster) ─────────────────────────────

function CursorPagination({
  page,
  limit,
  itemsOnPage,
  itemsBeforePage,
  hasNext,
  hasPrev,
  knownTotal,
  onNext,
  onPrev,
  onFirst,
  onLimitChange,
  limitOptions = PAGE_SIZE_OPTIONS,
  className,
  loading,
}: CursorProps) {
  const from = itemsOnPage === 0 ? 0 : itemsBeforePage + 1
  const to = itemsBeforePage + itemsOnPage

  // Middle-text logic. The range IS the position indicator — no separate
  // "Page N" label. See Pagination JSDoc for why.
  let middle: string
  if (loading && itemsOnPage === 0) middle = 'Loading…'
  else if (itemsOnPage === 0 && !hasPrev) middle = 'No results'
  else if (knownTotal != null) middle = `${from}–${to} of ${knownTotal.toLocaleString()}`
  else middle = `${from}–${to}`

  return (
    <Shell className={className}>
      {/* Empty left slot keeps the cluster centered without flex-grow gymnastics. */}
      <div className="flex-1" aria-hidden="true" />

      <NavCluster
        onFirst={onFirst}
        onPrev={onPrev}
        onNext={onNext}
        prevDisabled={!hasPrev}
        nextDisabled={!hasNext}
        firstDisabled={!hasPrev}
        loading={loading}
      >
        <RangeText page={page} loading={loading}>
          {middle}
        </RangeText>
      </NavCluster>

      <div className="flex-1 flex justify-end">
        <RowsSelector limit={limit} limitOptions={limitOptions} onLimitChange={onLimitChange} />
      </div>
    </Shell>
  )
}

// ── Offset mode (range left, numbered pages center) ───────────────────────

function OffsetPagination({
  page,
  total,
  limit,
  onPageChange,
  onFirst,
  onLimitChange,
  limitOptions = PAGE_SIZE_OPTIONS,
  className,
  loading,
}: OffsetProps) {
  const pageCount = Math.max(1, Math.ceil(total / limit))
  const from = total === 0 ? 0 : (page - 1) * limit + 1
  const to = Math.min(page * limit, total)

  // Page-number window — keeps the cluster compact for any pageCount.
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

  const rangeText = total === 0 ? 'No results' : `${from}–${to} of ${total.toLocaleString()}`

  return (
    <Shell className={className}>
      <RangeText page={page} loading={loading} className="flex-1">
        {rangeText}
      </RangeText>

      <NavCluster
        onFirst={onFirst}
        onPrev={() => onPageChange(page - 1)}
        onNext={() => onPageChange(page + 1)}
        prevDisabled={page <= 1}
        nextDisabled={page >= pageCount}
        firstDisabled={page <= 1}
        loading={loading}
      >
        <div className="flex items-center gap-0.5">
          {getPages().map((p, i) =>
            p === 'ellipsis' ? (
              <span key={`ellipsis-${i}`} className="px-1.5 py-1 text-xs text-slate-600">
                <MoreHorizontal size={13} />
              </span>
            ) : (
              <button
                key={p}
                onClick={() => onPageChange(p)}
                className={cn(
                  'min-w-[26px] px-2 py-1 text-xs rounded-md transition-colors',
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
      </NavCluster>

      <div className="flex-1 flex justify-end">
        <RowsSelector
          limit={limit}
          limitOptions={limitOptions}
          onLimitChange={onLimitChange}
          onResetPage={() => onPageChange(1)}
        />
      </div>
    </Shell>
  )
}

// ── Shared building blocks ────────────────────────────────────────────────

function Shell({ children, className }: { children: React.ReactNode; className?: string }) {
  return <div className={cn('flex items-center gap-3 px-4 py-2', className)}>{children}</div>
}

interface NavClusterProps {
  onFirst?: () => void
  onPrev: () => void
  onNext: () => void
  prevDisabled: boolean
  nextDisabled: boolean
  firstDisabled: boolean
  loading?: boolean
  children: React.ReactNode
}

function NavCluster({
  onFirst,
  onPrev,
  onNext,
  prevDisabled,
  nextDisabled,
  firstDisabled,
  loading,
  children,
}: NavClusterProps) {
  return (
    <div className="flex items-center gap-1 shrink-0">
      {onFirst && (
        <IconButton
          onClick={onFirst}
          disabled={firstDisabled || !!loading}
          ariaLabel="First page"
          title="First page (←←)"
        >
          <ChevronsLeft size={14} />
        </IconButton>
      )}
      <IconButton
        onClick={onPrev}
        disabled={prevDisabled || !!loading}
        ariaLabel="Previous page"
        title="Previous page (←)"
      >
        <ChevronLeft size={14} />
      </IconButton>

      <div className="px-2 py-1 text-xs text-slate-400 tabular-nums whitespace-nowrap">
        {children}
      </div>

      <IconButton
        onClick={onNext}
        disabled={nextDisabled || !!loading}
        ariaLabel="Next page"
        title="Next page (→)"
      >
        <ChevronRight size={14} />
      </IconButton>
    </div>
  )
}

function IconButton({
  onClick,
  disabled,
  children,
  ariaLabel,
  title,
}: {
  onClick: () => void
  disabled: boolean
  children: React.ReactNode
  ariaLabel: string
  title: string
}) {
  return (
    <button
      type="button"
      onClick={onClick}
      disabled={disabled}
      aria-label={ariaLabel}
      title={title}
      className={cn(
        'flex items-center justify-center w-7 h-7 rounded-md text-slate-400 transition-colors',
        'hover:text-white hover:bg-white/[0.06]',
        'focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-violet-400',
        'disabled:opacity-30 disabled:cursor-not-allowed disabled:hover:bg-transparent disabled:hover:text-slate-400'
      )}
    >
      {children}
    </button>
  )
}

function RangeText({
  children,
  loading,
  page,
  className,
}: {
  children: React.ReactNode
  loading?: boolean
  page: number
  className?: string
}) {
  return (
    <p
      // The `aria-live` lets screen readers announce page changes as the
      // user navigates with the keyboard.
      aria-live="polite"
      aria-label={`Page ${page}`}
      className={cn(
        'text-xs shrink-0 tabular-nums whitespace-nowrap',
        loading ? 'text-slate-600' : 'text-slate-500',
        className
      )}
    >
      {children}
    </p>
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
        className="bg-white/[0.04] border border-white/[0.08] text-xs text-slate-300 rounded-md px-2 py-1 focus:outline-none focus:border-violet-500 transition-colors cursor-pointer"
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
