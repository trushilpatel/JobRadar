import { useState, useRef, useEffect } from 'react'
import {
  type ColumnDef,
  type SortingState,
  flexRender,
  getCoreRowModel,
  getSortedRowModel,
  useReactTable,
} from '@tanstack/react-table'
export type { SortingState }
import { ChevronUp, ChevronDown, ChevronsUpDown } from 'lucide-react'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { Pagination } from '@/components/ui/pagination'
import { cn } from '@/lib/utils/utils'

// ── DataTable ────────────────────────────────────────────────────────────────

interface ServerPagination {
  page: number
  limit: number
  total: number
  onPageChange: (page: number) => void
  onLimitChange: (limit: number) => void
}

interface DataTableProps<TData> {
  columns: ColumnDef<TData, unknown>[]
  data: TData[]
  loading?: boolean
  /** Render error UI (with optional retry button) instead of the table body. */
  isError?: boolean
  /** When provided alongside isError, the error UI shows a Retry button. */
  onRetry?: () => void
  /** Custom error message; defaults to "Couldn't load results". */
  errorMessage?: string
  emptyMessage?: string
  /** Called when a row body is clicked */
  onRowClick?: (row: TData) => void
  /** Row id used to apply selected highlight */
  getRowId?: (row: TData) => string
  selectedId?: string
  /** Override per-row background/hover className (replaces selectedId highlight when provided) */
  getRowClassName?: (row: TData) => string
  /**
   * Per-row HTML attribute injection. Use for a11y (tabIndex, role, aria-label,
   * onKeyDown) — the table primitive stays generic; the caller knows what
   * a job/contact/company row should announce.
   */
  getRowProps?: (row: TData) => React.HTMLAttributes<HTMLTableRowElement>
  /** When provided, renders server-side pagination controls below the table */
  serverPagination?: ServerPagination
  /**
   * When provided, enables server-side (manual) sorting. The parent controls the
   * sort state and is responsible for fetching sorted data from the server.
   */
  sorting?: SortingState
  /**
   * Called when the sort state changes. Receives the new SortingState.
   * In server-side mode, use this to update your query params and reset to page 1.
   */
  onSortingChange?: (sorting: SortingState) => void
}

const SKELETON_ROWS = 8

export function DataTable<TData>({
  columns,
  data,
  loading,
  isError,
  onRetry,
  errorMessage = "Couldn't load results",
  emptyMessage = 'No results.',
  onRowClick,
  getRowId,
  selectedId,
  getRowClassName,
  getRowProps,
  serverPagination,
  sorting: controlledSorting,
  onSortingChange,
}: DataTableProps<TData>) {
  const [internalSorting, setInternalSorting] = useState<SortingState>([])
  const isControlled = controlledSorting !== undefined
  const effectiveSorting = isControlled ? controlledSorting : internalSorting

  const table = useReactTable({
    data,
    columns,
    state: { sorting: effectiveSorting },
    manualSorting: isControlled,
    onSortingChange: (updater) => {
      const next = typeof updater === 'function' ? updater(effectiveSorting) : updater
      if (isControlled) {
        onSortingChange?.(next)
      } else {
        setInternalSorting(next)
        onSortingChange?.(next)
      }
    },
    getCoreRowModel: getCoreRowModel(),
    getSortedRowModel: getSortedRowModel(),
    getRowId,
    manualPagination: !!serverPagination,
    pageCount: serverPagination
      ? Math.ceil(serverPagination.total / serverPagination.limit)
      : undefined,
  })

  // Refs for each rendered row — used to scroll the active row into view.
  const rowRefs = useRef<(HTMLTableRowElement | null)[]>([])
  const containerRef = useRef<HTMLDivElement | null>(null)

  // Scroll the active row into view whenever selectedId changes (keyboard nav).
  useEffect(() => {
    if (!selectedId || !getRowId) return
    const rows = table.getRowModel().rows
    const idx = rows.findIndex((r) => getRowId(r.original) === selectedId)
    if (idx >= 0) rowRefs.current[idx]?.scrollIntoView({ block: 'nearest' })
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [selectedId])

  const handleKeyDown = (e: React.KeyboardEvent<HTMLDivElement>) => {
    if (!onRowClick) return
    const rows = table.getRowModel().rows
    if (rows.length === 0) return

    const currentIdx =
      selectedId && getRowId ? rows.findIndex((r) => getRowId(r.original) === selectedId) : -1

    if (e.key === 'ArrowDown') {
      e.preventDefault()
      const next = Math.min(currentIdx + 1, rows.length - 1)
      const row = rows[next]
      if (row) onRowClick(row.original)
    } else if (e.key === 'ArrowUp') {
      e.preventDefault()
      const prev = Math.max(currentIdx - 1, 0)
      const row = rows[prev]
      if (row) onRowClick(row.original)
    }
  }

  // Bring focus to the table container after clicking a row so subsequent
  // arrow-key presses are received by the keydown handler.
  function handleRowClick(row: TData) {
    onRowClick?.(row)
    containerRef.current?.focus()
  }

  if (isError) {
    return (
      <div
        role="alert"
        className="flex flex-col items-center justify-center gap-3 py-16 text-center"
      >
        <p className="text-sm text-slate-300">{errorMessage}</p>
        {onRetry && (
          <button
            type="button"
            onClick={onRetry}
            className="rounded-lg border border-white/10 bg-white/[0.04] px-3 py-1.5 text-xs text-slate-200 hover:bg-white/[0.08] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-violet-400"
          >
            Try again
          </button>
        )}
      </div>
    )
  }

  if (loading) {
    return (
      <div className="flex flex-col" aria-busy="true" aria-live="polite">
        <Table>
          <TableHeader>
            {table.getHeaderGroups().map((hg) => (
              <tr key={hg.id} className="border-b border-white/[0.07]">
                {hg.headers.map((header) => (
                  <TableHead
                    key={header.id}
                    style={{ width: header.getSize() !== 150 ? header.getSize() : undefined }}
                  >
                    {header.isPlaceholder
                      ? null
                      : flexRender(header.column.columnDef.header, header.getContext())}
                  </TableHead>
                ))}
              </tr>
            ))}
          </TableHeader>
          <TableBody>
            {Array.from({ length: SKELETON_ROWS }).map((_, rowIdx) => (
              <TableRow key={`skeleton-${rowIdx}`} aria-hidden="true">
                {table.getAllColumns().map((col) => (
                  <TableCell key={col.id}>
                    <div className="h-3 w-3/4 animate-pulse rounded bg-white/[0.06]" />
                  </TableCell>
                ))}
              </TableRow>
            ))}
          </TableBody>
        </Table>
      </div>
    )
  }

  if (data.length === 0) {
    return <div className="text-center py-20 text-slate-500">{emptyMessage}</div>
  }

  return (
    <div
      ref={containerRef}
      className="flex flex-col outline-none"
      tabIndex={onRowClick ? 0 : undefined}
      onKeyDown={handleKeyDown}
    >
      <Table>
        <TableHeader>
          {table.getHeaderGroups().map((hg) => (
            <tr key={hg.id} className="border-b border-white/[0.07]">
              {hg.headers.map((header) => {
                const sorted = header.column.getCanSort() ? header.column.getIsSorted() : false
                const ariaSort: React.AriaAttributes['aria-sort'] = !header.column.getCanSort()
                  ? undefined
                  : sorted === 'asc'
                    ? 'ascending'
                    : sorted === 'desc'
                      ? 'descending'
                      : 'none'
                return (
                  <TableHead
                    key={header.id}
                    aria-sort={ariaSort}
                    style={{ width: header.getSize() !== 150 ? header.getSize() : undefined }}
                  >
                    {header.isPlaceholder
                      ? null
                      : flexRender(header.column.columnDef.header, header.getContext())}
                  </TableHead>
                )
              })}
            </tr>
          ))}
        </TableHeader>
        <TableBody>
          {table.getRowModel().rows.map((row, index) => {
            const isSelected = selectedId !== undefined && getRowId?.(row.original) === selectedId
            const rowBg = getRowClassName
              ? getRowClassName(row.original)
              : isSelected
                ? 'bg-violet-500/[0.08]'
                : 'hover:bg-white/[0.02]'
            const extraProps = getRowProps?.(row.original) ?? {}
            return (
              <TableRow
                key={row.id}
                ref={(el) => {
                  rowRefs.current[index] = el
                }}
                onClick={() => handleRowClick(row.original)}
                className={cn(onRowClick && 'cursor-pointer', rowBg)}
                {...extraProps}
              >
                {row.getVisibleCells().map((cell) => (
                  <TableCell key={cell.id}>
                    {flexRender(cell.column.columnDef.cell, cell.getContext())}
                  </TableCell>
                ))}
              </TableRow>
            )
          })}
        </TableBody>
      </Table>

      {serverPagination && (
        <div className="border-t border-white/[0.07]">
          <Pagination
            page={serverPagination.page}
            total={serverPagination.total}
            limit={serverPagination.limit}
            onPageChange={serverPagination.onPageChange}
            onLimitChange={serverPagination.onLimitChange}
          />
        </div>
      )}
    </div>
  )
}

// ── DataTableColumnHeader ────────────────────────────────────────────────────
// Drop-in sortable header. Use as the `header` in any ColumnDef.

import type { Column } from '@tanstack/react-table'

interface ColumnHeaderProps<TData, TValue> {
  column: Column<TData, TValue>
  title: string
  className?: string
}

export function DataTableColumnHeader<TData, TValue>({
  column,
  title,
  className,
}: ColumnHeaderProps<TData, TValue>) {
  if (!column.getCanSort()) {
    return <span className={cn('text-slate-400', className)}>{title}</span>
  }

  const sorted = column.getIsSorted()
  const direction = sorted === 'asc' ? 'ascending' : sorted === 'desc' ? 'descending' : 'not sorted'

  return (
    <button
      onClick={column.getToggleSortingHandler()}
      aria-label={`Sort by ${title}, currently ${direction}`}
      className={cn(
        'inline-flex items-center gap-1 text-slate-400 hover:text-white transition-colors select-none focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-violet-400 focus-visible:rounded',
        className
      )}
    >
      {title}
      {sorted === 'asc' ? (
        <ChevronUp size={12} />
      ) : sorted === 'desc' ? (
        <ChevronDown size={12} />
      ) : (
        <ChevronsUpDown size={12} className="opacity-40" />
      )}
    </button>
  )
}
