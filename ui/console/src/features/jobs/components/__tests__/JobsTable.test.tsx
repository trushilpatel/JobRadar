import { describe, it, expect, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import { JobsTable } from '../JobsTable'
import { JOB_STATUS } from '@/constants/jobStatus'
import type { Job } from '@/types'

// Mock sub-components that pull in complex deps (TanStack Table, avatars, etc.)
// We render JobsTable directly — it's a pure presentational component that
// accepts typed props, so we mock the heavy children.
vi.mock('@/components/ui/data-table', () => ({
  DataTable: ({
    data,
    loading,
    emptyMessage,
  }: {
    data: Job[]
    loading?: boolean
    emptyMessage?: string
    columns: unknown
    getRowId?: (row: Job) => string
    selectedId?: string
    onRowClick?: (row: Job) => void
    getRowClassName?: (row: Job) => string
    sorting?: unknown
    onSortingChange?: (sorting: unknown) => void
  }) => {
    if (loading) return <div data-testid="data-table-loading">Loading...</div>
    if (data.length === 0) return <div data-testid="data-table-empty">{emptyMessage}</div>
    return (
      <div data-testid="data-table">
        <div data-testid="column-header-company">Company</div>
        <div data-testid="column-header-title">Title</div>
        <div data-testid="column-header-location">Location</div>
        <div data-testid="column-header-added">Added</div>
        {data.map((job) => (
          <div key={job.id} data-testid={`row-${job.id}`}>
            <span data-testid={`job-title-${job.id}`}>{job.title}</span>
            <span data-testid={`job-company-${job.id}`}>{job.company?.name}</span>
          </div>
        ))}
      </div>
    )
  },
}))

vi.mock('@/components/ui/pagination', () => ({
  Pagination: () => <div data-testid="pagination" />,
}))

vi.mock('@/features/jobs/components/jobs-columns', () => ({
  buildJobColumns: () => [],
}))

// ── Fixtures ─────────────────────────────────────────────────────────────────

const mockJob: Job = {
  id: 'j1',
  job_post_id: 'p1',
  company_id: 'c1',
  title: 'Software Engineer',
  location: 'New York',
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

const defaultProps = {
  jobs: [mockJob],
  loading: false,
  tab: JOB_STATUS.NEW,
  page: 1,
  limit: 20,
  itemsOnPage: 1,
  itemsBeforePage: 0,
  hasNextPage: false,
  hasPrevPage: false,
  selectedIds: new Set<string>(),
  activeJobId: null,
  onRowClick: vi.fn(),
  onToggleSelect: vi.fn(),
  onSelectAll: vi.fn(),
  onNextPage: vi.fn(),
  onPrevPage: vi.fn(),
  onLimitChange: vi.fn(),
  sorting: [],
  onSortingChange: vi.fn(),
}

// ── Tests ─────────────────────────────────────────────────────────────────────

describe('JobsTable', () => {
  it('renders without crashing', () => {
    const { container } = render(<JobsTable {...defaultProps} />)
    expect(container).toBeTruthy()
  })

  it('shows loading state when loading=true', () => {
    render(<JobsTable {...defaultProps} loading={true} jobs={[]} />)
    expect(screen.getByTestId('data-table-loading')).toBeInTheDocument()
  })

  it('renders data table when jobs are present', () => {
    render(<JobsTable {...defaultProps} />)
    expect(screen.getByTestId('data-table')).toBeInTheDocument()
  })

  it('renders job title in the table', () => {
    render(<JobsTable {...defaultProps} />)
    expect(screen.getByTestId('job-title-j1')).toHaveTextContent('Software Engineer')
  })

  it('renders job company name in the table', () => {
    render(<JobsTable {...defaultProps} />)
    expect(screen.getByTestId('job-company-j1')).toHaveTextContent('Acme Corp')
  })

  it('shows column headers', () => {
    render(<JobsTable {...defaultProps} />)
    expect(screen.getByTestId('column-header-company')).toBeInTheDocument()
    expect(screen.getByTestId('column-header-title')).toBeInTheDocument()
  })

  it('renders pagination component', () => {
    render(<JobsTable {...defaultProps} />)
    expect(screen.getByTestId('pagination')).toBeInTheDocument()
  })

  it('shows empty state message when jobs is empty and not loading', () => {
    render(<JobsTable {...defaultProps} jobs={[]} />)
    const empty = screen.getByTestId('data-table-empty')
    expect(empty).toHaveTextContent('No new jobs')
  })

  it('shows filter-specific empty message when hasActiveFilters=true', () => {
    render(<JobsTable {...defaultProps} jobs={[]} hasActiveFilters={true} />)
    const empty = screen.getByTestId('data-table-empty')
    expect(empty).toHaveTextContent('No results match your current filters')
  })

  it('shows tab-specific empty message for saved tab', () => {
    render(<JobsTable {...defaultProps} jobs={[]} tab="saved" />)
    const empty = screen.getByTestId('data-table-empty')
    expect(empty).toHaveTextContent('No saved jobs yet')
  })

  it('shows tab-specific empty message for applied tab', () => {
    render(<JobsTable {...defaultProps} jobs={[]} tab="applied" />)
    const empty = screen.getByTestId('data-table-empty')
    expect(empty).toHaveTextContent('No applied jobs yet')
  })

  it('renders multiple jobs', () => {
    const job2: Job = { ...mockJob, id: 'j2', title: 'Product Manager' }
    render(<JobsTable {...defaultProps} jobs={[mockJob, job2]} itemsOnPage={2} />)
    expect(screen.getByTestId('row-j1')).toBeInTheDocument()
    expect(screen.getByTestId('row-j2')).toBeInTheDocument()
  })

  it('accepts a focusedIndex prop without crashing', () => {
    const job2: Job = { ...mockJob, id: 'j2', title: 'Product Manager' }
    expect(() =>
      render(<JobsTable {...defaultProps} jobs={[mockJob, job2]} focusedIndex={1} />)
    ).not.toThrow()
  })
})
