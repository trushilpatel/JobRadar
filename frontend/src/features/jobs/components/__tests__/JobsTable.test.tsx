import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import { JobsTable } from '../JobsTable'
import { JOB_STATUS } from '@/config/constants/jobStatus'
import type { Job } from '@/types'

// Mock sub-components that pull in complex deps (TanStack Table, avatars, etc.)
// We render JobsTable directly — it's a pure presentational component that
// accepts typed props, so we mock the heavy children.
vi.mock('@/components/ui/data-table', () => ({
  DataTable: ({
    data,
    loading,
    emptyMessage,
    getRowClassName,
    getRowProps,
  }: {
    data: Job[]
    loading?: boolean
    emptyMessage?: string
    columns: unknown
    getRowId?: (row: Job) => string
    selectedId?: string
    onRowClick?: (row: Job) => void
    getRowClassName?: (row: Job) => string
    getRowProps?: (row: Job) => Record<string, unknown>
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
        {data.map((job) => {
          // Invoke the real getRowClassName/getRowProps closures JobsTable
          // passes down (rather than stubbing them away) so tests can assert
          // on the actual hover/focus/edge-accent behavior they compute.
          // Mirrors the real DataTable's merge (className composes, it isn't
          // replaced by getRowProps' own className).
          const { style, className: extraClassName, ...rowProps } = getRowProps?.(job) ?? {}
          return (
            <div
              key={job.id}
              data-testid={`row-${job.id}`}
              className={[getRowClassName?.(job), extraClassName].filter(Boolean).join(' ')}
              style={style as React.CSSProperties}
              {...rowProps}
            >
              <span data-testid={`job-title-${job.id}`}>{job.title}</span>
              <span data-testid={`job-company-${job.id}`}>{job.company?.name}</span>
            </div>
          )
        })}
      </div>
    )
  },
}))

vi.mock('@/features/jobs/components/jobs-columns', () => ({
  buildJobColumns: () => [],
}))

// ── IntersectionObserver mock ────────────────────────────────────────────────
// jsdom has no native IntersectionObserver. Capture the callback per instance
// so tests can manually fire an "isIntersecting" entry to simulate the
// sentinel scrolling into view.
let ioInstances: { callback: IntersectionObserverCallback; disconnect: () => void }[] = []

class MockIntersectionObserver {
  callback: IntersectionObserverCallback
  constructor(callback: IntersectionObserverCallback) {
    this.callback = callback
    ioInstances.push(this)
  }
  observe() {}
  unobserve() {}
  disconnect() {}
}

function fireIntersection(isIntersecting: boolean) {
  const instance = ioInstances.at(-1)
  instance?.callback(
    [{ isIntersecting } as IntersectionObserverEntry],
    instance as unknown as IntersectionObserver
  )
}

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
  hasNextPage: false,
  activeJobId: null,
  onRowClick: vi.fn(),
  onToggleSave: vi.fn(),
  onDismiss: vi.fn(),
  onApply: vi.fn(),
  onLoadMore: vi.fn(),
  sorting: [],
  onSortingChange: vi.fn(),
}

// ── Tests ─────────────────────────────────────────────────────────────────────

describe('JobsTable', () => {
  beforeEach(() => {
    ioInstances = []
    vi.stubGlobal('IntersectionObserver', MockIntersectionObserver)
  })

  afterEach(() => {
    vi.unstubAllGlobals()
  })

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

  it('shows tab-specific empty message for the tracking tab', () => {
    render(<JobsTable {...defaultProps} jobs={[]} tab="tracking" />)
    const empty = screen.getByTestId('data-table-empty')
    expect(empty).toHaveTextContent('No jobs in the selected stages')
  })

  it('renders multiple jobs', () => {
    const job2: Job = { ...mockJob, id: 'j2', title: 'Product Manager' }
    render(<JobsTable {...defaultProps} jobs={[mockJob, job2]} />)
    expect(screen.getByTestId('row-j1')).toBeInTheDocument()
    expect(screen.getByTestId('row-j2')).toBeInTheDocument()
  })

  it('accepts a focusedIndex prop without crashing', () => {
    const job2: Job = { ...mockJob, id: 'j2', title: 'Product Manager' }
    expect(() =>
      render(<JobsTable {...defaultProps} jobs={[mockJob, job2]} focusedIndex={1} />)
    ).not.toThrow()
  })

  // ── Infinite-scroll sentinel ─────────────────────────────────────────────────

  it('does not render a sentinel when there is no next page and nothing is fetching', () => {
    render(<JobsTable {...defaultProps} hasNextPage={false} isFetchingMore={false} />)
    expect(screen.queryByTestId('jobs-table-sentinel')).not.toBeInTheDocument()
  })

  it('renders a sentinel when hasNextPage is true', () => {
    render(<JobsTable {...defaultProps} hasNextPage={true} />)
    expect(screen.getByTestId('jobs-table-sentinel')).toBeInTheDocument()
  })

  it('calls onLoadMore when the sentinel intersects', () => {
    const onLoadMore = vi.fn()
    render(<JobsTable {...defaultProps} hasNextPage={true} onLoadMore={onLoadMore} />)
    fireIntersection(true)
    expect(onLoadMore).toHaveBeenCalledTimes(1)
  })

  it('does not call onLoadMore when the sentinel is not intersecting', () => {
    const onLoadMore = vi.fn()
    render(<JobsTable {...defaultProps} hasNextPage={true} onLoadMore={onLoadMore} />)
    fireIntersection(false)
    expect(onLoadMore).not.toHaveBeenCalled()
  })

  it('shows the "Loading more roles…" indicator while isFetchingMore is true', () => {
    render(<JobsTable {...defaultProps} hasNextPage={true} isFetchingMore={true} />)
    expect(screen.getByText('Loading more roles…')).toBeInTheDocument()
  })

  it('does not show the loading-more indicator when isFetchingMore is false', () => {
    render(<JobsTable {...defaultProps} hasNextPage={true} isFetchingMore={false} />)
    expect(screen.queryByText('Loading more roles…')).not.toBeInTheDocument()
  })

  // ── Apply action ─────────────────────────────────────────────────────────────

  it('passes onApply through to buildJobColumns without crashing', () => {
    const onApply = vi.fn()
    expect(() => render(<JobsTable {...defaultProps} onApply={onApply} />)).not.toThrow()
  })

  // ── Row hover / focus / active classes ──────────────────────────────────────
  // Regression coverage for the DataTable merge bug: JobsTable relies on
  // getRowProps' className composing with (not replacing) getRowClassName's
  // hover/focus/active classes.

  it('gives a default row the hover background and the violet edge-accent hook', () => {
    render(<JobsTable {...defaultProps} />)
    const row = screen.getByTestId('row-j1')
    expect(row.className).toContain('hover:bg-hover')
    expect(row).toHaveAttribute('data-has-edge', 'true')
    expect(row.style.getPropertyValue('--row-edge')).toBe('var(--violet)')
  })

  it('rings the keyboard-focused row', () => {
    render(<JobsTable {...defaultProps} focusedIndex={0} />)
    const row = screen.getByTestId('row-j1')
    expect(row.className).toContain('ring-2')
    expect(row.className).toContain('ring-violet')
  })

  it('highlights the active row with the accent-dim background instead of the hover class', () => {
    render(<JobsTable {...defaultProps} activeJobId="j1" />)
    const row = screen.getByTestId('row-j1')
    expect(row.className).toContain('bg-[var(--accent-dim)]')
    expect(row.className).not.toContain('hover:bg-hover')
  })

  it('applies the row-level focus-ring className passed via getRowProps alongside the hover class', () => {
    render(<JobsTable {...defaultProps} />)
    const row = screen.getByTestId('row-j1')
    expect(row.className).toContain('focus-visible:ring-2')
    expect(row.className).toContain('focus-visible:ring-violet')
  })
})
