import { describe, it, expect, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import type { CellContext } from '@tanstack/react-table'
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
  job_keywords: [],
  location: null,
  remote: false,
  salary_range: null,
  added_at: '2024-01-01T00:00:00Z',
}

describe('buildApplyQueueColumns', () => {
  const actions = {
    isResumeUpdatePending: vi.fn().mockReturnValue(false),
    getEffectiveResume: vi.fn().mockReturnValue(null),
    getMatchSummary: vi.fn().mockReturnValue(null),
    onOpenResumePicker: vi.fn(),
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

  describe('resume column', () => {
    it('shows "Select resume" when getEffectiveResume returns null', () => {
      renderCell(columnById('resume'), baseItem)
      expect(screen.getByRole('button', { name: /select resume/i })).toHaveTextContent(
        'Select resume'
      )
    })

    it('shows the effective resume label when one is resolved', () => {
      const localActions = {
        ...actions,
        getEffectiveResume: vi.fn().mockReturnValue({ id: 'resume-1', label: 'Resume One' }),
      }
      const columns = buildApplyQueueColumns(localActions)
      const resumeColumn = columns.find((c) => c.id === 'resume')!
      const cellFn = resumeColumn.cell as (
        ctx: CellContext<ApplyQueueItem, unknown>
      ) => React.ReactNode
      render(<>{cellFn(makeContext(baseItem))}</>)
      expect(screen.getByRole('button')).toHaveTextContent('Resume One')
    })

    it('shows the match summary count alongside the resume label', () => {
      const localActions = {
        ...actions,
        getEffectiveResume: vi.fn().mockReturnValue({ id: 'resume-1', label: 'Resume One' }),
        getMatchSummary: vi.fn().mockReturnValue('3/5 matched'),
      }
      const columns = buildApplyQueueColumns(localActions)
      const resumeColumn = columns.find((c) => c.id === 'resume')!
      const cellFn = resumeColumn.cell as (
        ctx: CellContext<ApplyQueueItem, unknown>
      ) => React.ReactNode
      render(<>{cellFn(makeContext(baseItem))}</>)
      expect(screen.getByRole('button')).toHaveTextContent('3/5')
      expect(screen.getByRole('button')).not.toHaveTextContent('matched')
    })

    it('omits the count when there is no match summary', () => {
      renderCell(columnById('resume'), baseItem)
      const button = screen.getByRole('button')
      expect(button).not.toHaveTextContent('/')
    })

    it('calls onOpenResumePicker with the job id when clicked', async () => {
      const user = userEvent.setup()
      const onOpenResumePicker = vi.fn()
      const columns = buildApplyQueueColumns({ ...actions, onOpenResumePicker })
      const resumeColumn = columns.find((c) => c.id === 'resume')!
      const cellFn = resumeColumn.cell as (
        ctx: CellContext<ApplyQueueItem, unknown>
      ) => React.ReactNode
      render(<>{cellFn(makeContext(baseItem))}</>)
      await user.click(screen.getByRole('button'))
      expect(onOpenResumePicker).toHaveBeenCalledWith('job-1')
    })

    it('disables the trigger while a resume update is pending for this job', () => {
      const localActions = { ...actions, isResumeUpdatePending: vi.fn().mockReturnValue(true) }
      const columns = buildApplyQueueColumns(localActions)
      const resumeColumn = columns.find((c) => c.id === 'resume')!
      const cellFn = resumeColumn.cell as (
        ctx: CellContext<ApplyQueueItem, unknown>
      ) => React.ReactNode
      render(<>{cellFn(makeContext(baseItem))}</>)
      expect(screen.getByRole('button')).toBeDisabled()
    })
  })
})
