import { useState } from 'react'
import {
  type ColumnDef,
  type SortingState,
  flexRender,
  getCoreRowModel,
  getSortedRowModel,
  useReactTable,
} from '@tanstack/react-table'
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
import { cn } from '@/lib/utils'

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
  emptyMessage?: string
  /** Called when a row body is clicked */
  onRowClick?: (row: TData) => void
  /** Row id used to apply selected highlight */
  getRowId?: (row: TData) => string
  selectedId?: string
  /** Override per-row background/hover className (replaces selectedId highlight when provided) */
  getRowClassName?: (row: TData) => string
  /** When provided, renders server-side pagination controls below the table */
  serverPagination?: ServerPagination
  /** Called when the sort order changes — use to reset to page 1 */
  onSortingChange?: () => void
}

export function DataTable<TData>({
  columns,
  data,
  loading,
  emptyMessage = 'No results.',
  onRowClick,
  getRowId,
  selectedId,
  getRowClassName,
  serverPagination,
  onSortingChange,
}: DataTableProps<TData>) {
  const [sorting, setSorting] = useState<SortingState>([])

  const table = useReactTable({
    data,
    columns,
    state: { sorting },
    onSortingChange: (updater) => {
      setSorting(updater)
      onSortingChange?.()
    },
    getCoreRowModel: getCoreRowModel(),
    getSortedRowModel: getSortedRowModel(),
    getRowId,
    manualPagination: !!serverPagination,
    pageCount: serverPagination
      ? Math.ceil(serverPagination.total / serverPagination.limit)
      : undefined,
  })

  if (loading) {
    return (
      <div className="flex items-center justify-center h-48">
        <div className="w-6 h-6 border-2 border-violet-500 border-t-transparent rounded-full animate-spin" />
      </div>
    )
  }

  if (!loading && data.length === 0) {
    return <div className="text-center py-20 text-slate-500">{emptyMessage}</div>
  }

  return (
    <div className="flex flex-col">
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
          {table.getRowModel().rows.map((row) => {
            const isSelected = selectedId !== undefined && getRowId?.(row.original) === selectedId
            const rowBg = getRowClassName
              ? getRowClassName(row.original)
              : isSelected
                ? 'bg-violet-500/[0.08]'
                : 'hover:bg-white/[0.02]'
            return (
              <TableRow
                key={row.id}
                onClick={() => onRowClick?.(row.original)}
                className={cn(onRowClick && 'cursor-pointer', rowBg)}
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

  return (
    <button
      onClick={column.getToggleSortingHandler()}
      className={cn(
        'inline-flex items-center gap-1 text-slate-400 hover:text-white transition-colors select-none',
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
