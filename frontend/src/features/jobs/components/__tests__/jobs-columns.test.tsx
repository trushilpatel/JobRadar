import { describe, it, expect, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import type { CellContext } from '@tanstack/react-table'
import { buildJobColumns, type JobTableActions } from '../jobs-columns'
import { JOB_STATUS } from '@/config/constants/jobStatus'
import type { Job } from '@/types'

// buildJobColumns returns plain TanStack ColumnDefs — no React context or
// table instance needed to exercise a single cell renderer, so each test
// invokes `column.cell(context)` directly with a hand-built CellContext
// instead of mounting the whole DataTable/TanStack machinery.

const baseJob: Job = {
  id: 'j1',
  job_post_id: 'p1',
  company_id: 'c1',
  title: 'Senior Software Engineer',
  location: null,
  remote: false,
  url: 'https://example.com/job/1',
  department: null,
  employ_type: null,
  experience_years: null,
  tech_stack: null,
  salary_range: null,
  llm_summary: null,
  sponsorship: false,
  requires_citizen: false,
  security_clearance: false,
  role_brief: null,
  responsibilities: null,
  status: JOB_STATUS.NEW,
  saved: false,
  notes: null,
  user_state: {
    status: JOB_STATUS.NEW,
    notes: null,
    saved_at: null,
    applied_at: null,
    dismissed_at: null,
    dismissal_reason: null,
    dismissal_note: null,
  },
  added_at: '2024-01-01T00:00:00Z',
  company: {
    id: 'c1',
    name: 'Acme Corp',
    url: 'https://acme.com',
    adapter_type: 'greenhouse',
    active: true,
    created_at: '2024-01-01T00:00:00Z',
    last_scan_at: null,
    last_scan_status: null,
  },
}

function defaultActions(overrides: Partial<JobTableActions> = {}): JobTableActions {
  return {
    onToggleSave: vi.fn(),
    onApply: vi.fn(),
    ...overrides,
  }
}

function columnById(actions: JobTableActions, id: string) {
  const columns = buildJobColumns(actions)
  const column = columns.find(
    (c) => c.id === id || (c as { accessorKey?: string }).accessorKey === id
  )
  if (!column) throw new Error(`column ${id} not found`)
  return column
}

// Minimal CellContext stand-in — only `row.original` and `getValue` are used
// by any cell renderer in jobs-columns.tsx.
function makeContext(job: Job, value?: unknown) {
  return {
    row: { original: job },
    getValue: () => value,
  } as unknown as CellContext<Job, unknown>
}

function renderCell(column: ReturnType<typeof columnById>, job: Job, value?: unknown) {
  const cellFn = column.cell as (ctx: CellContext<Job, unknown>) => React.ReactNode
  return render(<>{cellFn(makeContext(job, value))}</>)
}

describe('buildJobColumns — bookmark column', () => {
  it('renders the empty bookmark icon and calls onToggleSave when unsaved', async () => {
    const onToggleSave = vi.fn()
    const column = columnById(defaultActions({ onToggleSave }), 'bookmark')
    renderCell(column, baseJob)
    const btn = screen.getByRole('button', { name: `Save ${baseJob.title}` })
    expect(btn).toHaveAttribute('aria-pressed', 'false')
    btn.click()
    expect(onToggleSave).toHaveBeenCalledWith(baseJob)
  })

  it('renders the filled bookmark and remove label when saved', () => {
    const job: Job = {
      ...baseJob,
      user_state: { ...baseJob.user_state, saved_at: '2024-01-02T00:00:00Z' },
    }
    const column = columnById(defaultActions(), 'bookmark')
    renderCell(column, job)
    const btn = screen.getByRole('button', { name: `Remove ${job.title} from saved` })
    expect(btn).toHaveAttribute('aria-pressed', 'true')
  })
})

describe('buildJobColumns — title column', () => {
  it('renders the title text with a tooltip for the full string', () => {
    const column = columnById(defaultActions(), 'title')
    renderCell(column, baseJob, baseJob.title)
    const el = screen.getByText(baseJob.title)
    expect(el).toHaveAttribute('title', baseJob.title)
  })

  it('shows the resume-attached paperclip when getAttachmentState returns a state', () => {
    const getAttachmentState = vi.fn().mockReturnValue('attached')
    const column = columnById(defaultActions({ getAttachmentState }), 'title')
    renderCell(column, baseJob, baseJob.title)
    expect(getAttachmentState).toHaveBeenCalledWith(baseJob.id)
    expect(screen.getByLabelText('Resume attached (attached)')).toBeInTheDocument()
  })

  it('renders no paperclip when getAttachmentState is absent', () => {
    const column = columnById(defaultActions(), 'title')
    renderCell(column, baseJob, baseJob.title)
    expect(screen.queryByLabelText(/Resume attached/)).not.toBeInTheDocument()
  })

  it('renders no paperclip when getAttachmentState returns null', () => {
    const getAttachmentState = vi.fn().mockReturnValue(null)
    const column = columnById(defaultActions({ getAttachmentState }), 'title')
    renderCell(column, baseJob, baseJob.title)
    expect(screen.queryByLabelText(/Resume attached/)).not.toBeInTheDocument()
  })
})

describe('buildJobColumns — company column', () => {
  it('renders the company name when present', () => {
    const column = columnById(defaultActions(), 'company')
    renderCell(column, baseJob)
    expect(screen.getByText('Acme Corp')).toBeInTheDocument()
  })

  it('falls back to company_id when the embedded company is missing', () => {
    // `row.company` is typed as required, but the cell defensively guards
    // with `?.` for API responses that omit it — exercise that branch.
    const job = {
      ...baseJob,
      company: undefined,
      company_id: 'fallback-id',
    } as unknown as Job
    const column = columnById(defaultActions(), 'company')
    renderCell(column, job)
    expect(screen.getByText('fallback-id')).toBeInTheDocument()
  })
})

describe('buildJobColumns — status column', () => {
  it('renders the status pill for the job status', () => {
    const job: Job = { ...baseJob, status: JOB_STATUS.SAVED }
    const column = columnById(defaultActions(), 'status')
    renderCell(column, job)
    expect(screen.getByText(/saved/i)).toBeInTheDocument()
  })
})

describe('buildJobColumns — location column', () => {
  it('renders a Remote badge when remote is true', () => {
    const job: Job = { ...baseJob, remote: true, location: null }
    const column = columnById(defaultActions(), 'location')
    renderCell(column, job)
    expect(screen.getByText('Remote')).toBeInTheDocument()
  })

  it('renders the location text with the remote flag off and a location present', () => {
    const job: Job = { ...baseJob, remote: false, location: 'New York, NY' }
    const column = columnById(defaultActions(), 'location')
    renderCell(column, job)
    expect(screen.getByText('New York, NY')).toBeInTheDocument()
  })

  it('renders nothing when there is no remote flag and no location', () => {
    const job: Job = { ...baseJob, remote: false, location: null }
    const column = columnById(defaultActions(), 'location')
    const { container } = renderCell(column, job)
    expect(container).toBeEmptyDOMElement()
  })
})

describe('buildJobColumns — salary column', () => {
  it('renders the formatted salary range when present', () => {
    const job: Job = { ...baseJob, salary_range: { min: 100000, max: 150000, currency: 'USD' } }
    const column = columnById(defaultActions(), 'salary')
    renderCell(column, job)
    expect(screen.queryByText('Not listed')).not.toBeInTheDocument()
  })

  it('renders "Not listed" when salary_range is null', () => {
    const column = columnById(defaultActions(), 'salary')
    renderCell(column, { ...baseJob, salary_range: null })
    expect(screen.getByText('Not listed')).toBeInTheDocument()
  })
})

describe('buildJobColumns — posted column', () => {
  it('renders a relative time with an exact-datetime tooltip', () => {
    const column = columnById(defaultActions(), 'added_at')
    renderCell(column, baseJob, baseJob.added_at)
    const el = screen.getByTitle(/Added/)
    expect(el).toBeInTheDocument()
  })
})

describe('buildJobColumns — apply column', () => {
  it('calls onApply with the job when clicked', () => {
    const onApply = vi.fn()
    const column = columnById(defaultActions({ onApply }), 'apply')
    renderCell(column, baseJob)
    screen.getByRole('button', { name: `Apply to ${baseJob.title}` }).click()
    expect(onApply).toHaveBeenCalledWith(baseJob)
  })
})
