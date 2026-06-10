import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, fireEvent } from '@testing-library/react'
import { TooltipProvider } from '@/components/ui/tooltip'
import type { Company } from '@/types'
import { useCompanyScanStats } from '@/features/companies/hooks/useCompanyScanStats'
import { useCompanyAnalytics } from '@/features/companies/hooks/useCompanyAnalytics'
import { CompanyDetailDrawer } from '../CompanyDetailDrawer'

vi.mock('@/features/companies/hooks/useCompanyScanStats')
vi.mock('@/features/companies/hooks/useCompanyAnalytics')

function renderDrawer(ui: React.ReactElement) {
  return render(<TooltipProvider>{ui}</TooltipProvider>)
}

const company: Company = {
  id: 'stripe',
  name: 'Stripe',
  url: 'https://stripe.com',
  adapter_type: 'greenhouse',
  active: true,
  created_at: '2024-01-01T00:00:00Z',
  last_scan_at: null,
  last_scan_status: null,
}

const baseProps = {
  company,
  onClose: vi.fn(),
  onScan: vi.fn(),
  onEdit: vi.fn(),
  onDelete: vi.fn(),
}

beforeEach(() => {
  vi.clearAllMocks()
  vi.mocked(useCompanyScanStats).mockReturnValue({
    data: { total_new: 5, new_today: 1, new_this_month: 3, recent_runs: [] },
    isLoading: false,
  } as unknown as ReturnType<typeof useCompanyScanStats>)
  vi.mocked(useCompanyAnalytics).mockReturnValue({
    data: undefined,
    isError: false,
  } as unknown as ReturnType<typeof useCompanyAnalytics>)
})

describe('CompanyDetailDrawer — View new jobs navigation', () => {
  it('calls onViewJobs with the company name when the button is clicked', () => {
    const onViewJobs = vi.fn()
    renderDrawer(<CompanyDetailDrawer {...baseProps} onViewJobs={onViewJobs} />)

    fireEvent.click(screen.getByRole('button', { name: /view 5 new jobs/i }))

    expect(onViewJobs).toHaveBeenCalledOnce()
    expect(onViewJobs).toHaveBeenCalledWith('Stripe')
  })

  it('does not throw when onViewJobs is not provided', () => {
    renderDrawer(<CompanyDetailDrawer {...baseProps} />)

    // Button is only shown when total_new > 0; clicking it should be a no-op
    expect(() =>
      fireEvent.click(screen.getByRole('button', { name: /view 5 new jobs/i }))
    ).not.toThrow()
  })
})

type ScanStatsReturn = ReturnType<typeof useCompanyScanStats>
type AnalyticsReturn = ReturnType<typeof useCompanyAnalytics>

function setScanStats(value: Partial<ScanStatsReturn>) {
  vi.mocked(useCompanyScanStats).mockReturnValue(value as unknown as ScanStatsReturn)
}
function setAnalytics(value: Partial<AnalyticsReturn>) {
  vi.mocked(useCompanyAnalytics).mockReturnValue(value as unknown as AnalyticsReturn)
}

describe('CompanyDetailDrawer — header actions', () => {
  it('invokes onScan and onEdit from the header buttons', () => {
    const onScan = vi.fn()
    const onEdit = vi.fn()
    renderDrawer(<CompanyDetailDrawer {...baseProps} onScan={onScan} onEdit={onEdit} />)

    fireEvent.click(screen.getByRole('button', { name: 'Scan now' }))
    expect(onScan).toHaveBeenCalledWith(company)

    fireEvent.click(screen.getByTitle('Edit'))
    expect(onEdit).toHaveBeenCalledWith(company)
  })

  it('invokes onClose from the close button', () => {
    const onClose = vi.fn()
    renderDrawer(<CompanyDetailDrawer {...baseProps} onClose={onClose} />)
    fireEvent.click(screen.getByRole('button', { name: 'Close drawer' }))
    expect(onClose).toHaveBeenCalledOnce()
  })

  it('renders the host from the company url, falling back to the raw url when invalid', () => {
    const { unmount } = renderDrawer(<CompanyDetailDrawer {...baseProps} />)
    expect(screen.getByText('stripe.com')).toBeInTheDocument()
    unmount()

    renderDrawer(<CompanyDetailDrawer {...baseProps} company={{ ...company, url: 'not a url' }} />)
    expect(screen.getByText('not a url')).toBeInTheDocument()
  })
})

describe('CompanyDetailDrawer — delete confirmation', () => {
  it('opens the confirm dialog and calls onDelete on confirm', () => {
    const onDelete = vi.fn()
    renderDrawer(<CompanyDetailDrawer {...baseProps} onDelete={onDelete} />)

    fireEvent.click(screen.getByTitle('Delete'))
    expect(screen.getByText('Delete company?')).toBeInTheDocument()

    fireEvent.click(screen.getByRole('button', { name: 'Delete' }))
    expect(onDelete).toHaveBeenCalledWith('stripe')
  })

  it('closes the confirm dialog on cancel without deleting', () => {
    const onDelete = vi.fn()
    renderDrawer(<CompanyDetailDrawer {...baseProps} onDelete={onDelete} />)

    fireEvent.click(screen.getByTitle('Delete'))
    fireEvent.click(screen.getByRole('button', { name: 'Cancel' }))
    expect(onDelete).not.toHaveBeenCalled()
  })
})

describe('CompanyDetailDrawer — recent scans + stats', () => {
  it('renders a loading skeleton while scan stats load', () => {
    setScanStats({ data: undefined, isLoading: true })
    setAnalytics({ data: undefined, isError: false })
    const { container } = renderDrawer(<CompanyDetailDrawer {...baseProps} />)
    expect(container.querySelector('.animate-pulse')).toBeInTheDocument()
  })

  it('renders last scan time and average duration when present', () => {
    setScanStats({
      data: {
        total_scans: 3,
        total_fetched: 30,
        total_new: 5,
        error_count: 0,
        avg_duration_ms: 2500,
        new_today: 1,
        new_this_month: 4,
        recent_runs: [],
      },
      isLoading: false,
    })
    setAnalytics({ data: undefined, isError: false })
    renderDrawer(
      <CompanyDetailDrawer
        {...baseProps}
        company={{ ...company, last_scan_at: '2024-01-01T00:00:00Z' }}
      />
    )
    expect(screen.getByText('2.5s')).toBeInTheDocument()
    expect(screen.getByText('Avg time')).toBeInTheDocument()
  })

  it('formats sub-second average durations in milliseconds', () => {
    setScanStats({
      data: {
        total_scans: 1,
        total_fetched: 1,
        total_new: 0,
        error_count: 0,
        avg_duration_ms: 450,
        new_today: 0,
        new_this_month: 0,
        recent_runs: [],
      },
      isLoading: false,
    })
    setAnalytics({ data: undefined, isError: false })
    renderDrawer(<CompanyDetailDrawer {...baseProps} />)
    expect(screen.getByText('450ms')).toBeInTheDocument()
  })
})

describe('CompanyDetailDrawer — logs section', () => {
  beforeEach(() => {
    setAnalytics({ data: undefined, isError: false })
  })

  it('renders log skeletons while scan stats are loading', () => {
    setScanStats({ data: undefined, isLoading: true })
    const { container } = renderDrawer(<CompanyDetailDrawer {...baseProps} />)
    fireEvent.click(screen.getByText('Logs'))
    // Skeleton rows appear inside the expanded logs region while loading.
    expect(container.querySelectorAll('.animate-pulse').length).toBeGreaterThan(1)
  })

  it('shows "No runs yet" when there are no recent runs', () => {
    setScanStats({
      data: {
        total_scans: 0,
        total_fetched: 0,
        error_count: 0,
        avg_duration_ms: null,
        total_new: 0,
        new_today: 0,
        new_this_month: 0,
        recent_runs: [],
      },
      isLoading: false,
    })
    renderDrawer(<CompanyDetailDrawer {...baseProps} />)
    fireEvent.click(screen.getByText('Logs'))
    expect(screen.getByText('No runs yet')).toBeInTheDocument()
  })

  it('lists OK runs with fetched + new counts and expands error runs', () => {
    setScanStats({
      data: {
        total_scans: 2,
        total_fetched: 12,
        error_count: 1,
        avg_duration_ms: 1500,
        total_new: 0,
        new_today: 0,
        new_this_month: 0,
        recent_runs: [
          {
            company_id: 'stripe',
            scanned_at: '2024-01-01T00:00:00Z',
            jobs_fetched: 12,
            jobs_new: 4,
            error: null,
            duration_ms: 1500,
          },
          {
            company_id: 'stripe',
            scanned_at: '2024-01-02T00:00:00Z',
            jobs_fetched: 0,
            jobs_new: 0,
            error: 'boom: scan failed',
            duration_ms: null,
          },
        ],
      },
      isLoading: false,
    })
    renderDrawer(<CompanyDetailDrawer {...baseProps} />)
    fireEvent.click(screen.getByText('Logs'))

    expect(screen.getByText(/OK · 12 fetched · 4 new/)).toBeInTheDocument()

    // Error row is collapsed initially, expands on click.
    expect(screen.queryByText('boom: scan failed')).not.toBeInTheDocument()
    fireEvent.click(screen.getByText('Error'))
    expect(screen.getByText('boom: scan failed')).toBeInTheDocument()

    // Toggling again collapses it.
    fireEvent.click(screen.getByText('Error'))
    expect(screen.queryByText('boom: scan failed')).not.toBeInTheDocument()
  })
})

describe('CompanyDetailDrawer — hiring activity', () => {
  beforeEach(() => {
    setScanStats({
      data: {
        total_scans: 0,
        total_fetched: 0,
        error_count: 0,
        avg_duration_ms: null,
        total_new: 0,
        new_today: 0,
        new_this_month: 0,
        recent_runs: [],
      },
      isLoading: false,
    })
  })

  it('shows an error message when analytics fail to load', () => {
    setAnalytics({ data: undefined, isError: true })
    renderDrawer(<CompanyDetailDrawer {...baseProps} />)
    expect(screen.getByText('Failed to load hiring data')).toBeInTheDocument()
  })

  it('shows "No hiring data yet" when every month is empty', () => {
    setAnalytics({
      data: {
        monthly: [
          { month: '2024-01', new: 0, removed: 0 },
          { month: '2024-02', new: 0, removed: 0 },
        ],
        velocity: { this_week: 0, last_week: 0 },
      },
      isError: false,
    })
    renderDrawer(<CompanyDetailDrawer {...baseProps} />)
    expect(screen.getByText('No hiring data yet')).toBeInTheDocument()
  })

  it('renders the chart and an upward velocity label when there is data', () => {
    setAnalytics({
      data: {
        monthly: [
          { month: '2024-01', new: 10, removed: 2 },
          { month: '2024-02', new: 15, removed: 1 },
        ],
        velocity: { this_week: 8, last_week: 3 },
      },
      isError: false,
    })
    renderDrawer(<CompanyDetailDrawer {...baseProps} />)
    // velocity label: this_week 8 vs last_week 3 → ↑ 5
    expect(screen.getByText(/↑ 5 · 8 this week/)).toBeInTheDocument()
  })

  it('renders a flat velocity label when this week equals last week', () => {
    setAnalytics({
      data: {
        monthly: [{ month: '2024-01', new: 5, removed: 0 }],
        velocity: { this_week: 4, last_week: 4 },
      },
      isError: false,
    })
    renderDrawer(<CompanyDetailDrawer {...baseProps} />)
    expect(screen.getByText('4 new this week')).toBeInTheDocument()
  })

  it('renders a downward velocity label when this week is lower', () => {
    setAnalytics({
      data: {
        monthly: [{ month: '2024-01', new: 1, removed: 0 }],
        velocity: { this_week: 1, last_week: 6 },
      },
      isError: false,
    })
    renderDrawer(<CompanyDetailDrawer {...baseProps} />)
    expect(screen.getByText(/↓ 5 · 1 this week/)).toBeInTheDocument()
  })
})
