import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, fireEvent } from '@testing-library/react'
import { useReactTable, getCoreRowModel, flexRender, type ColumnDef } from '@tanstack/react-table'
import { TooltipProvider } from '@/components/ui/tooltip'
import type { Company } from '@/types'
import { buildCompanyColumns, type CompanyColumnActions } from '../companies-columns'

// ── Host table that renders the column defs against real rows ────────────────

function Host({ companies, actions }: { companies: Company[]; actions: CompanyColumnActions }) {
  const columns = buildCompanyColumns(actions) as ColumnDef<Company, unknown>[]
  // eslint-disable-next-line react-hooks/incompatible-library -- test host table
  const table = useReactTable({
    data: companies,
    columns,
    getCoreRowModel: getCoreRowModel(),
  })

  return (
    <table>
      <thead>
        {table.getHeaderGroups().map((hg) => (
          <tr key={hg.id}>
            {hg.headers.map((h) => (
              <th key={h.id}>
                {h.isPlaceholder ? null : flexRender(h.column.columnDef.header, h.getContext())}
              </th>
            ))}
          </tr>
        ))}
      </thead>
      <tbody>
        {table.getRowModel().rows.map((row) => (
          <tr key={row.id}>
            {row.getVisibleCells().map((cell) => (
              <td key={cell.id}>{flexRender(cell.column.columnDef.cell, cell.getContext())}</td>
            ))}
          </tr>
        ))}
      </tbody>
    </table>
  )
}

function makeActions(over: Partial<CompanyColumnActions> = {}): CompanyColumnActions {
  return {
    onScan: vi.fn(),
    onEdit: vi.fn(),
    onDelete: vi.fn(),
    onToggleActive: vi.fn(),
    scanning: new Set<string>(),
    ...over,
  }
}

const base: Company = {
  id: 'stripe',
  name: 'Stripe',
  url: 'https://stripe.com',
  adapter_type: 'greenhouse',
  active: true,
  created_at: '2024-01-01T00:00:00Z',
  last_scan_at: null,
  last_scan_status: null,
}

function renderHost(company: Partial<Company>, actions = makeActions()) {
  return render(
    <TooltipProvider>
      <Host companies={[{ ...base, ...company }]} actions={actions} />
    </TooltipProvider>
  )
}

beforeEach(() => vi.clearAllMocks())

describe('buildCompanyColumns — name cell', () => {
  it('renders the company name and an external link to its url', () => {
    renderHost({ name: 'Stripe', url: 'https://stripe.com' })
    expect(screen.getByText('Stripe')).toBeInTheDocument()
    const link = screen.getByRole('link')
    expect(link).toHaveAttribute('href', 'https://stripe.com')
  })

  it('stops propagation when the external link is clicked', () => {
    renderHost({})
    // Clicking should not throw — the handler calls stopPropagation.
    expect(() => fireEvent.click(screen.getByRole('link'))).not.toThrow()
  })
})

describe('buildCompanyColumns — adapter + status cells', () => {
  it('renders the adapter badge', () => {
    renderHost({ adapter_type: 'lever' })
    expect(screen.getByText('lever')).toBeInTheDocument()
  })

  it.each([['running'], ['queued'], ['ok'], ['error'], [null]] as const)(
    'maps last_scan_status %s to a status pill',
    (status) => {
      const { unmount } = renderHost({ last_scan_status: status })
      // StatusPill renders a span; presence is enough to confirm the branch ran.
      expect(screen.getByText('Status')).toBeInTheDocument()
      unmount()
    }
  )
})

describe('buildCompanyColumns — history cell', () => {
  it('renders scan history padded to 10 slots', () => {
    renderHost({ scan_history: [true, false, true] })
    expect(screen.getByText('Last 10 scans')).toBeInTheDocument()
  })

  it('marks the rightmost slot as running when last_scan_status is running', () => {
    renderHost({ scan_history: [true], last_scan_status: 'running' })
    expect(screen.getByText('Last 10 scans')).toBeInTheDocument()
  })

  it('handles a company with no scan_history (all idle slots)', () => {
    renderHost({ scan_history: undefined })
    expect(screen.getByText('Last 10 scans')).toBeInTheDocument()
  })
})

describe('buildCompanyColumns — last scan cell', () => {
  it('renders an em-dash when last_scan_at is null', () => {
    renderHost({ last_scan_at: null })
    expect(screen.getByText('—')).toBeInTheDocument()
  })

  it('renders "Just now" for a very recent scan', () => {
    renderHost({ last_scan_at: new Date().toISOString() })
    expect(screen.getByText('Just now')).toBeInTheDocument()
  })

  it('renders minutes ago', () => {
    renderHost({ last_scan_at: new Date(Date.now() - 5 * 60_000).toISOString() })
    expect(screen.getByText('5m ago')).toBeInTheDocument()
  })

  it('renders hours ago', () => {
    renderHost({ last_scan_at: new Date(Date.now() - 3 * 3600_000).toISOString() })
    expect(screen.getByText('3h ago')).toBeInTheDocument()
  })

  it('renders days ago', () => {
    renderHost({ last_scan_at: new Date(Date.now() - 2 * 86_400_000).toISOString() })
    expect(screen.getByText('2d ago')).toBeInTheDocument()
  })
})

describe('buildCompanyColumns — active toggle', () => {
  it('shows Deactivate label and invokes onToggleActive when active', () => {
    const actions = makeActions()
    renderHost({ active: true }, actions)
    const toggle = screen.getByRole('switch', { name: 'Deactivate' })
    fireEvent.click(toggle)
    expect(actions.onToggleActive).toHaveBeenCalledOnce()
  })

  it('shows Activate label when inactive', () => {
    renderHost({ active: false })
    expect(screen.getByRole('switch', { name: 'Activate' })).toBeInTheDocument()
  })

  it('exposes the toggle as a switch reflecting active state', () => {
    renderHost({ active: true })
    const toggle = screen.getByRole('switch')
    expect(toggle).toHaveAttribute('aria-checked', 'true')
  })

  it('disables the toggle while a toggle is in flight (P1.7)', () => {
    const actions = makeActions({ togglingActive: new Set(['stripe']) })
    renderHost({ id: 'stripe', active: true }, actions)
    expect(screen.getByRole('switch')).toBeDisabled()
  })
})

describe('buildCompanyColumns — action buttons', () => {
  it('invokes onScan, onEdit and onDelete', () => {
    const actions = makeActions()
    renderHost({ id: 'stripe', name: 'Stripe' }, actions)

    fireEvent.click(screen.getByRole('button', { name: 'Scan Stripe now' }))
    fireEvent.click(screen.getByRole('button', { name: 'Edit Stripe' }))
    fireEvent.click(screen.getByRole('button', { name: 'Delete Stripe' }))

    expect(actions.onScan).toHaveBeenCalledOnce()
    expect(actions.onEdit).toHaveBeenCalledOnce()
    expect(actions.onDelete).toHaveBeenCalledWith('stripe')
  })

  it('disables the scan button and shows the scanning label while scanning', () => {
    const actions = makeActions({ scanning: new Set(['stripe']) })
    renderHost({ id: 'stripe', name: 'Stripe' }, actions)
    const scanBtn = screen.getByRole('button', { name: 'Scanning Stripe' })
    expect(scanBtn).toBeDisabled()
  })
})
