import { describe, it, expect, vi } from 'vitest'
import { render, screen, fireEvent } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import type { ColumnDef, SortingState } from '@tanstack/react-table'
import { DataTable, DataTableColumnHeader } from '@/components/ui/data-table'

interface Row {
  id: string
  name: string
  score: number
}

const data: Row[] = [
  { id: '1', name: 'Alpha', score: 3 },
  { id: '2', name: 'Bravo', score: 1 },
  { id: '3', name: 'Charlie', score: 2 },
]

const columns: ColumnDef<Row, unknown>[] = [
  {
    accessorKey: 'name',
    header: ({ column }) => <DataTableColumnHeader column={column} title="Name" />,
  },
  {
    accessorKey: 'score',
    header: 'Score',
    enableSorting: false,
  },
]

describe('DataTable', () => {
  it('renders rows for populated data', () => {
    render(<DataTable columns={columns} data={data} />)
    expect(screen.getByText('Alpha')).toBeInTheDocument()
    expect(screen.getByText('Bravo')).toBeInTheDocument()
    expect(screen.getByText('Charlie')).toBeInTheDocument()
  })

  it('renders the empty message when there are no rows', () => {
    render(<DataTable columns={columns} data={[]} emptyMessage="Nothing here" />)
    expect(screen.getByText('Nothing here')).toBeInTheDocument()
  })

  it('renders skeleton rows while loading', () => {
    const { container } = render(<DataTable columns={columns} data={[]} loading />)
    expect(container.querySelector('[aria-busy="true"]')).toBeInTheDocument()
    expect(container.querySelectorAll('.animate-pulse').length).toBeGreaterThan(0)
  })

  it('renders the error state with a retry button that fires onRetry', async () => {
    const onRetry = vi.fn()
    render(<DataTable columns={columns} data={[]} isError onRetry={onRetry} errorMessage="Boom" />)
    expect(screen.getByRole('alert')).toHaveTextContent('Boom')
    await userEvent.click(screen.getByRole('button', { name: 'Try again' }))
    expect(onRetry).toHaveBeenCalledOnce()
  })

  it('renders the error state without a retry button when onRetry is absent', () => {
    render(<DataTable columns={columns} data={[]} isError />)
    expect(screen.getByText("Couldn't load results")).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'Try again' })).not.toBeInTheDocument()
  })

  it('sorts internally when a sortable header is clicked', async () => {
    render(<DataTable columns={columns} data={data} />)
    const header = screen.getByRole('button', { name: /Sort by Name/i })
    await userEvent.click(header)
    expect(header).toHaveAttribute('aria-label', expect.stringContaining('ascending'))
  })

  it('uses controlled (server) sorting and forwards onSortingChange', async () => {
    const onSortingChange = vi.fn()
    const sorting: SortingState = [{ id: 'name', desc: false }]
    render(
      <DataTable
        columns={columns}
        data={data}
        sorting={sorting}
        onSortingChange={onSortingChange}
      />
    )
    await userEvent.click(screen.getByRole('button', { name: /Sort by Name/i }))
    expect(onSortingChange).toHaveBeenCalled()
  })

  it('fires onRowClick when a row is clicked', async () => {
    const onRowClick = vi.fn()
    render(
      <DataTable columns={columns} data={data} onRowClick={onRowClick} getRowId={(r) => r.id} />
    )
    await userEvent.click(screen.getByText('Alpha'))
    expect(onRowClick).toHaveBeenCalledWith(data[0])
  })

  it('navigates rows with arrow keys', () => {
    const onRowClick = vi.fn()
    render(
      <DataTable
        columns={columns}
        data={data}
        onRowClick={onRowClick}
        getRowId={(r) => r.id}
        selectedId="1"
      />
    )
    const container = screen.getByText('Alpha').closest('div[tabindex]') as HTMLElement
    fireEvent.keyDown(container, { key: 'ArrowDown' })
    expect(onRowClick).toHaveBeenCalledWith(data[1])
    fireEvent.keyDown(container, { key: 'ArrowUp' })
    expect(onRowClick).toHaveBeenCalledWith(data[0])
  })

  it('does not navigate via its own handler when disableArrowKeyNav is set', () => {
    // Regression: a caller with its own page-level keyboard nav (e.g.
    // JobsTable) must be able to opt out of DataTable's built-in
    // ArrowUp/ArrowDown handling, since both firing on the same keydown
    // advances the selection twice.
    const onRowClick = vi.fn()
    render(
      <DataTable
        columns={columns}
        data={data}
        onRowClick={onRowClick}
        getRowId={(r) => r.id}
        selectedId="1"
        disableArrowKeyNav
      />
    )
    const container = screen.getByText('Alpha').closest('div[tabindex]') as HTMLElement
    fireEvent.keyDown(container, { key: 'ArrowDown' })
    expect(onRowClick).not.toHaveBeenCalled()
  })

  it('applies the selected highlight to the matching row', () => {
    render(<DataTable columns={columns} data={data} getRowId={(r) => r.id} selectedId="2" />)
    const selectedRow = screen.getByText('Bravo').closest('tr')
    expect(selectedRow?.className).toContain('bg-[var(--accent-dim)]')
  })

  it('applies a custom row className and row props', () => {
    render(
      <DataTable
        columns={columns}
        data={data}
        getRowClassName={() => 'custom-row'}
        getRowProps={() => ({ 'aria-label': 'job row' })}
      />
    )
    const row = screen.getByText('Alpha').closest('tr')
    expect(row?.className).toContain('custom-row')
    expect(row).toHaveAttribute('aria-label', 'job row')
  })

  it('merges getRowProps className with getRowClassName instead of replacing it', () => {
    // Regression test: getRowProps used to spread onto <tr> after className
    // was set, so a caller-supplied className (e.g. focus-ring styles, as
    // JobsTable provides) silently discarded the hover/active classes from
    // getRowClassName instead of composing with them.
    render(
      <DataTable
        columns={columns}
        data={data}
        getRowClassName={() => 'hover:bg-hover group/row'}
        getRowProps={() => ({ className: 'focus-visible:ring-2' })}
      />
    )
    const row = screen.getByText('Alpha').closest('tr')
    expect(row?.className).toContain('hover:bg-hover')
    expect(row?.className).toContain('group/row')
    expect(row?.className).toContain('focus-visible:ring-2')
  })

  it('renders server pagination controls when serverPagination is provided', () => {
    render(
      <DataTable
        columns={columns}
        data={data}
        serverPagination={{
          page: 1,
          limit: 25,
          total: 100,
          onPageChange: vi.fn(),
          onLimitChange: vi.fn(),
        }}
      />
    )
    expect(screen.getByText('Alpha')).toBeInTheDocument()
    // Pagination renders a per-page limit selector
    expect(screen.getByRole('combobox')).toBeInTheDocument()
  })
})

describe('DataTableColumnHeader', () => {
  it('renders a plain span for a non-sortable column', () => {
    const column = {
      getCanSort: () => false,
    } as unknown as Parameters<typeof DataTableColumnHeader>[0]['column']
    render(<DataTableColumnHeader column={column} title="Plain" />)
    expect(screen.getByText('Plain')).toBeInTheDocument()
    expect(screen.queryByRole('button')).not.toBeInTheDocument()
  })

  it('renders a sort button with the ascending state label', async () => {
    const onToggle = vi.fn()
    const column = {
      getCanSort: () => true,
      getIsSorted: () => 'asc' as const,
      getToggleSortingHandler: () => onToggle,
    } as unknown as Parameters<typeof DataTableColumnHeader>[0]['column']
    render(<DataTableColumnHeader column={column} title="Name" />)
    const btn = screen.getByRole('button', { name: /currently ascending/i })
    await userEvent.click(btn)
    expect(onToggle).toHaveBeenCalled()
  })

  it('renders the descending state label', () => {
    const column = {
      getCanSort: () => true,
      getIsSorted: () => 'desc' as const,
      getToggleSortingHandler: () => vi.fn(),
    } as unknown as Parameters<typeof DataTableColumnHeader>[0]['column']
    render(<DataTableColumnHeader column={column} title="Name" />)
    expect(screen.getByRole('button', { name: /currently descending/i })).toBeInTheDocument()
  })

  it('renders the not-sorted state label', () => {
    const column = {
      getCanSort: () => true,
      getIsSorted: () => false as const,
      getToggleSortingHandler: () => vi.fn(),
    } as unknown as Parameters<typeof DataTableColumnHeader>[0]['column']
    render(<DataTableColumnHeader column={column} title="Name" />)
    expect(screen.getByRole('button', { name: /currently not sorted/i })).toBeInTheDocument()
  })
})
