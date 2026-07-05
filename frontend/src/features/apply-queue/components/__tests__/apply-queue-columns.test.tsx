import { describe, it, expect, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import type { CellContext } from '@tanstack/react-table'
import { flexRender, getCoreRowModel, useReactTable } from '@tanstack/react-table'
import { buildApplyQueueColumns } from '../apply-queue-columns'
import type { ApplyQueueItem } from '../../api/apply-queue.api'

const baseItem: ApplyQueueItem = {
  job_id: 'job-1',
  job_title: 'Senior Engineer',
  company: 'Acme',
  job_url: 'https://acme.example/jobs/1',
  status: 'saved',
  resume_id: null,
  resume_label: null,
  recommended_resume_id: null,
  recommended_resume_label: null,
  location: null,
  remote: false,
  salary_range: null,
  added_at: '2024-01-01T00:00:00Z',
}

describe('buildApplyQueueColumns', () => {
  const actions = {
    resumeOptions: [{ id: 'resume-1', label: 'Resume One' }],
    onResumeChange: vi.fn(),
    isResumeUpdatePending: vi.fn().mockReturnValue(false),
  }

  it('builds the job, location, salary, status, posted, and resume columns in order', () => {
    const columns = buildApplyQueueColumns(actions)
    expect(columns.map((c) => c.id)).toEqual([
      'job',
      'location',
      'salary',
      'status',
      'added_at',
      'resume',
    ])
  })

  it('disables sorting on every column except posted (added_at)', () => {
    const columns = buildApplyQueueColumns(actions)
    const byId = (id: string) => columns.find((c) => c.id === id)
    expect(byId('location')?.enableSorting).toBe(false)
    expect(byId('salary')?.enableSorting).toBe(false)
    expect(byId('status')?.enableSorting).toBe(false)
    expect(byId('resume')?.enableSorting).toBe(false)
    expect(byId('added_at')?.enableSorting).not.toBe(false)
  })

  function columnById(id: string) {
    const columns = buildApplyQueueColumns(actions)
    const column = columns.find((c) => c.id === id)
    if (!column) throw new Error(`column ${id} not found`)
    return column
  }

  // Minimal CellContext stand-in — only `row.original` and `getValue` are used
  // by any cell renderer in apply-queue-columns.tsx (mirrors the pattern in
  // features/jobs/components/__tests__/jobs-columns.test.tsx).
  function makeContext(item: ApplyQueueItem, value?: unknown) {
    return {
      row: { original: item },
      getValue: () => value,
    } as unknown as CellContext<ApplyQueueItem, unknown>
  }

  function renderCell(
    column: ReturnType<typeof columnById>,
    item: ApplyQueueItem,
    value?: unknown
  ) {
    const cellFn = column.cell as (ctx: CellContext<ApplyQueueItem, unknown>) => React.ReactNode
    return render(<>{cellFn(makeContext(item, value))}</>)
  }

  describe('location column', () => {
    it('renders a Remote badge when remote is true', () => {
      const item: ApplyQueueItem = { ...baseItem, remote: true, location: null }
      renderCell(columnById('location'), item)
      expect(screen.getByText('Remote')).toBeInTheDocument()
    })

    it('renders the location text when not remote and a location is present', () => {
      const item: ApplyQueueItem = { ...baseItem, remote: false, location: 'Austin, TX' }
      renderCell(columnById('location'), item)
      expect(screen.getByText('Austin, TX')).toBeInTheDocument()
    })

    it('renders nothing when there is no remote flag and no location', () => {
      const { container } = renderCell(columnById('location'), baseItem)
      expect(container).toBeEmptyDOMElement()
    })
  })

  describe('salary column', () => {
    it('renders the formatted salary range when present', () => {
      const item: ApplyQueueItem = {
        ...baseItem,
        salary_range: { min: 100000, max: 150000, currency: 'USD' },
      }
      renderCell(columnById('salary'), item)
      expect(screen.queryByText('Not listed')).not.toBeInTheDocument()
    })

    it('renders "Not listed" when salary_range is null', () => {
      renderCell(columnById('salary'), baseItem)
      expect(screen.getByText('Not listed')).toBeInTheDocument()
    })
  })

  describe('posted column', () => {
    it('renders a relative time with an exact-datetime tooltip', () => {
      renderCell(columnById('added_at'), baseItem, baseItem.added_at)
      expect(screen.getByTitle(/Added/)).toBeInTheDocument()
    })
  })

  describe('resume column recommendation wiring', () => {
    const item: ApplyQueueItem = {
      ...baseItem,
      resume_id: null,
      resume_label: null,
      recommended_resume_id: 'resume-1',
      recommended_resume_label: 'Resume One',
    }

    function TestTable({ data }: { data: ApplyQueueItem[] }) {
      const columns = buildApplyQueueColumns(actions)
      const table = useReactTable({ data, columns, getCoreRowModel: getCoreRowModel() })
      const resumeCell = table.getRowModel().rows[0]!.getVisibleCells()[5]!
      return <>{flexRender(resumeCell.column.columnDef.cell, resumeCell.getContext())}</>
    }

    it('marks the option matching recommended_resume_id as recommended', async () => {
      const user = userEvent.setup()
      render(<TestTable data={[item]} />)
      await user.click(screen.getByRole('combobox'))
      expect(await screen.findByText('Recommended')).toBeInTheDocument()
    })

    it('does not mark any option recommended when recommended_resume_id is null', async () => {
      const user = userEvent.setup()
      render(<TestTable data={[{ ...item, recommended_resume_id: null }]} />)
      await user.click(screen.getByRole('combobox'))
      expect(await screen.findByText('Resume One')).toBeInTheDocument()
      expect(screen.queryByText('Recommended')).not.toBeInTheDocument()
    })
  })
})
