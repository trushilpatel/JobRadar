import { render, screen, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { MemoryRouter } from 'react-router-dom'
import * as dashboardApi from '@/api/dashboard'

import { ProgressPanel } from '../ProgressPanel'
import { MarketPanel } from '../MarketPanel'
import { HeatmapPanel } from '../HeatmapPanel'
import { InterviewingPanel } from '../InterviewingPanel'
import { VelocityPanel } from '../VelocityPanel'
import { TechTrendsPanel } from '../TechTrendsPanel'

vi.mock('@/api/dashboard')

// Recharts uses ResizeObserver which is not available in jsdom
class MockResizeObserver {
  observe() {}
  unobserve() {}
  disconnect() {}
}
global.ResizeObserver = MockResizeObserver as unknown as typeof ResizeObserver

function makeWrapper() {
  const qc = new QueryClient({
    defaultOptions: { queries: { retry: 0, gcTime: 0 } },
  })
  function Wrapper({ children }: { children: React.ReactNode }) {
    return (
      <MemoryRouter>
        <QueryClientProvider client={qc}>{children}</QueryClientProvider>
      </MemoryRouter>
    )
  }
  return Wrapper
}

// ── ProgressPanel ──────────────────────────────────────────────────────────

describe('ProgressPanel', () => {
  const sampleProgress = {
    applied_count: 5,
    awaiting_reply: 3,
    interviewing_count: 2,
    interviewing_this_week: 1,
    offer_count: 0,
  }

  it('shows skeleton while loading', () => {
    vi.mocked(dashboardApi.fetchProgress).mockReturnValue(new Promise(() => {}))
    render(<ProgressPanel />, { wrapper: makeWrapper() })
    expect(document.querySelector('.animate-pulse')).toBeInTheDocument()
  })

  it('renders applied count when data is present', async () => {
    vi.mocked(dashboardApi.fetchProgress).mockResolvedValue(sampleProgress)
    render(<ProgressPanel />, { wrapper: makeWrapper() })
    await waitFor(() => expect(screen.getByText('5')).toBeInTheDocument())
    expect(screen.getByText('3 awaiting reply')).toBeInTheDocument()
  })

  it('renders interviewing count when data is present', async () => {
    vi.mocked(dashboardApi.fetchProgress).mockResolvedValue(sampleProgress)
    render(<ProgressPanel />, { wrapper: makeWrapper() })
    await waitFor(() => expect(screen.getByText('2')).toBeInTheDocument())
    expect(screen.getByText('1 this week')).toBeInTheDocument()
  })

  it('renders offer count when data is present', async () => {
    vi.mocked(dashboardApi.fetchProgress).mockResolvedValue(sampleProgress)
    render(<ProgressPanel />, { wrapper: makeWrapper() })
    await waitFor(() => expect(screen.getByText('decision pending')).toBeInTheDocument())
  })

  it('renders stat columns when data is zero-valued', async () => {
    vi.mocked(dashboardApi.fetchProgress).mockResolvedValue({
      applied_count: 0,
      awaiting_reply: 0,
      interviewing_count: 0,
      interviewing_this_week: 0,
      offer_count: 0,
    })
    render(<ProgressPanel />, { wrapper: makeWrapper() })
    // Once data resolves, the stat labels are visible
    await waitFor(() => expect(screen.getByText('decision pending')).toBeInTheDocument())
    expect(document.querySelector('.animate-pulse')).not.toBeInTheDocument()
  })
})

// ── MarketPanel ────────────────────────────────────────────────────────────

describe('MarketPanel', () => {
  const sampleMarket = {
    new_since_yesterday: 12,
    total_open: 340,
    added_this_week: 50,
    added_last_week: 40,
    weekly_delta: 10,
  }

  it('shows skeleton while loading', () => {
    vi.mocked(dashboardApi.fetchMarket).mockReturnValue(new Promise(() => {}))
    render(<MarketPanel />, { wrapper: makeWrapper() })
    expect(document.querySelector('.animate-pulse')).toBeInTheDocument()
  })

  it('renders new_since_yesterday count when data is present', async () => {
    vi.mocked(dashboardApi.fetchMarket).mockResolvedValue(sampleMarket)
    render(<MarketPanel />, { wrapper: makeWrapper() })
    await waitFor(() => expect(screen.getByText('12')).toBeInTheDocument())
    expect(screen.getByText('since yesterday')).toBeInTheDocument()
  })

  it('renders total_open count when data is present', async () => {
    vi.mocked(dashboardApi.fetchMarket).mockResolvedValue(sampleMarket)
    render(<MarketPanel />, { wrapper: makeWrapper() })
    await waitFor(() => expect(screen.getByText('340')).toBeInTheDocument())
    expect(screen.getByText('+10 this week')).toBeInTheDocument()
  })

  it('shows negative weekly_delta correctly', async () => {
    vi.mocked(dashboardApi.fetchMarket).mockResolvedValue({
      ...sampleMarket,
      weekly_delta: -5,
    })
    render(<MarketPanel />, { wrapper: makeWrapper() })
    await waitFor(() => expect(screen.getByText('-5 this week')).toBeInTheDocument())
  })

  it('hides skeleton once data loads', async () => {
    vi.mocked(dashboardApi.fetchMarket).mockResolvedValue(sampleMarket)
    render(<MarketPanel />, { wrapper: makeWrapper() })
    await waitFor(() => expect(document.querySelector('.animate-pulse')).not.toBeInTheDocument())
  })
})

// ── HeatmapPanel ───────────────────────────────────────────────────────────

describe('HeatmapPanel', () => {
  it('shows skeleton while loading', () => {
    vi.mocked(dashboardApi.fetchHeatmap).mockReturnValue(new Promise(() => {}))
    render(<HeatmapPanel />, { wrapper: makeWrapper() })
    expect(document.querySelector('.animate-pulse')).toBeInTheDocument()
  })

  it('renders the grid when data is present', async () => {
    vi.mocked(dashboardApi.fetchHeatmap).mockResolvedValue([
      { week_start: '2024-01-01', job_count: 5 },
      { week_start: '2024-01-08', job_count: 12 },
    ])
    render(<HeatmapPanel />, { wrapper: makeWrapper() })
    // The legend only renders after loading completes
    await waitFor(() => expect(screen.getByText('fewer')).toBeInTheDocument())
    expect(document.querySelector('.animate-pulse')).not.toBeInTheDocument()
  })

  it('renders both "fewer" and "more" legend labels after loading', async () => {
    vi.mocked(dashboardApi.fetchHeatmap).mockResolvedValue([])
    render(<HeatmapPanel />, { wrapper: makeWrapper() })
    await waitFor(() => expect(screen.getByText('more')).toBeInTheDocument())
    expect(screen.getByText('fewer')).toBeInTheDocument()
  })

  it('shows error message when query rejects', async () => {
    vi.mocked(dashboardApi.fetchHeatmap).mockRejectedValue(new Error('network error'))
    render(<HeatmapPanel />, { wrapper: makeWrapper() })
    await waitFor(() => expect(screen.getByText('Failed to load heatmap data')).toBeInTheDocument())
  })
})

// ── InterviewingPanel ───────────────────────────────────────────────────────

describe('InterviewingPanel', () => {
  const sampleRows = [
    {
      application_id: 1,
      job_id: 'job-1',
      company_name: 'Stripe',
      job_title: 'Software Engineer',
      stage: 'phone_screen',
      display_stage: 'Interviewing',
      stage_at: '2024-01-15',
    },
  ]

  it('shows skeleton while loading', () => {
    vi.mocked(dashboardApi.fetchInterviews).mockReturnValue(new Promise(() => {}))
    // updateApplicationStage must be defined to avoid "not a function" in mutation
    vi.mocked(dashboardApi.updateApplicationStage).mockResolvedValue(undefined)
    render(<InterviewingPanel />, { wrapper: makeWrapper() })
    expect(document.querySelector('.animate-pulse')).toBeInTheDocument()
  })

  it('renders company name and job title when data is present', async () => {
    vi.mocked(dashboardApi.fetchInterviews).mockResolvedValue(sampleRows)
    vi.mocked(dashboardApi.updateApplicationStage).mockResolvedValue(undefined)
    render(<InterviewingPanel />, { wrapper: makeWrapper() })
    await waitFor(() => expect(screen.getByText('Stripe')).toBeInTheDocument())
    expect(screen.getByText('Software Engineer')).toBeInTheDocument()
  })

  it('shows empty state when no active applications', async () => {
    vi.mocked(dashboardApi.fetchInterviews).mockResolvedValue([])
    vi.mocked(dashboardApi.updateApplicationStage).mockResolvedValue(undefined)
    render(<InterviewingPanel />, { wrapper: makeWrapper() })
    await waitFor(() => expect(screen.getByText('No active applications')).toBeInTheDocument())
  })

  it('shows stage pill for an active row', async () => {
    vi.mocked(dashboardApi.fetchInterviews).mockResolvedValue(sampleRows)
    vi.mocked(dashboardApi.updateApplicationStage).mockResolvedValue(undefined)
    render(<InterviewingPanel />, { wrapper: makeWrapper() })
    await waitFor(() => expect(screen.getByText('Interviewing')).toBeInTheDocument())
  })
})

// ── VelocityPanel ───────────────────────────────────────────────────────────

describe('VelocityPanel', () => {
  const sampleRows = [
    {
      company_id: 'c1',
      company_name: 'Acme',
      week_current: 8,
      week_1: 5,
      week_2: 3,
      week_3: 2,
      multiplier: 1.6,
      velocity_tag: 'rising' as const,
    },
  ]

  it('shows skeleton while loading', () => {
    vi.mocked(dashboardApi.fetchVelocity).mockReturnValue(new Promise(() => {}))
    render(<VelocityPanel />, { wrapper: makeWrapper() })
    expect(document.querySelector('.animate-pulse')).toBeInTheDocument()
  })

  it('renders company name and current week count when data is present', async () => {
    vi.mocked(dashboardApi.fetchVelocity).mockResolvedValue(sampleRows)
    render(<VelocityPanel />, { wrapper: makeWrapper() })
    await waitFor(() => expect(screen.getByText('Acme')).toBeInTheDocument())
    expect(screen.getByText('+8')).toBeInTheDocument()
  })

  it('renders velocity tag badge when data is present', async () => {
    vi.mocked(dashboardApi.fetchVelocity).mockResolvedValue(sampleRows)
    render(<VelocityPanel />, { wrapper: makeWrapper() })
    await waitFor(() => expect(screen.getByText('1.6×')).toBeInTheDocument())
  })

  it('shows empty state when data is an empty array', async () => {
    vi.mocked(dashboardApi.fetchVelocity).mockResolvedValue([])
    render(<VelocityPanel />, { wrapper: makeWrapper() })
    await waitFor(() => expect(screen.getByText('No velocity data yet')).toBeInTheDocument())
  })

  it('renders normal velocity tag as dash', async () => {
    vi.mocked(dashboardApi.fetchVelocity).mockResolvedValue([
      { ...sampleRows[0], velocity_tag: 'normal', multiplier: 1 },
    ])
    render(<VelocityPanel />, { wrapper: makeWrapper() })
    await waitFor(() => expect(screen.getByText('Acme')).toBeInTheDocument())
    expect(screen.getByText('—')).toBeInTheDocument()
  })
})

// ── TechTrendsPanel ─────────────────────────────────────────────────────────

describe('TechTrendsPanel', () => {
  const sampleTrends = {
    total_jobs: 500,
    rising: [
      {
        tech: 'Go',
        q1: 10,
        q2: 15,
        q3: 20,
        q4: 30,
        growth_pct: 50,
        adoption_pct: 12,
        is_new: false,
      },
    ],
    new_entrants: [
      {
        tech: 'Rust',
        q1: 0,
        q2: 1,
        q3: 3,
        q4: 8,
        growth_pct: 0,
        adoption_pct: 8,
        is_new: true,
      },
    ],
  }

  it('shows skeleton while loading', () => {
    vi.mocked(dashboardApi.fetchTechTrends).mockReturnValue(new Promise(() => {}))
    render(<TechTrendsPanel />, { wrapper: makeWrapper() })
    expect(document.querySelector('.animate-pulse')).toBeInTheDocument()
  })

  it('renders rising tech name when data is present', async () => {
    vi.mocked(dashboardApi.fetchTechTrends).mockResolvedValue(sampleTrends)
    render(<TechTrendsPanel />, { wrapper: makeWrapper() })
    await waitFor(() => expect(screen.getByText('Go')).toBeInTheDocument())
  })

  it('renders new entrant tech name when data is present', async () => {
    vi.mocked(dashboardApi.fetchTechTrends).mockResolvedValue(sampleTrends)
    render(<TechTrendsPanel />, { wrapper: makeWrapper() })
    await waitFor(() => expect(screen.getByText('Rust')).toBeInTheDocument())
  })

  it('shows total_jobs count in header', async () => {
    vi.mocked(dashboardApi.fetchTechTrends).mockResolvedValue(sampleTrends)
    render(<TechTrendsPanel />, { wrapper: makeWrapper() })
    await waitFor(() => expect(screen.getByText(/500 jobs analyzed/)).toBeInTheDocument())
  })

  it('renders growth percentage for rising techs', async () => {
    vi.mocked(dashboardApi.fetchTechTrends).mockResolvedValue(sampleTrends)
    render(<TechTrendsPanel />, { wrapper: makeWrapper() })
    await waitFor(() => expect(screen.getByText('+50%')).toBeInTheDocument())
  })

  it('renders "New" label for is_new tech rows', async () => {
    vi.mocked(dashboardApi.fetchTechTrends).mockResolvedValue({
      ...sampleTrends,
      rising: [{ ...sampleTrends.rising[0], is_new: true }],
    })
    render(<TechTrendsPanel />, { wrapper: makeWrapper() })
    await waitFor(() => expect(screen.getByText('New')).toBeInTheDocument())
  })

  it('shows empty state messages when arrays are empty', async () => {
    vi.mocked(dashboardApi.fetchTechTrends).mockResolvedValue({
      total_jobs: 0,
      rising: [],
      new_entrants: [],
    })
    render(<TechTrendsPanel />, { wrapper: makeWrapper() })
    await waitFor(() => expect(screen.getByText('No rising techs yet')).toBeInTheDocument())
    expect(screen.getByText('No new entrants yet')).toBeInTheDocument()
  })

  it('renders nothing visible when data is undefined (null-like)', async () => {
    // fetchTechTrends always normalizes so !data path only renders nothing
    vi.mocked(dashboardApi.fetchTechTrends).mockResolvedValue(undefined as never)
    render(<TechTrendsPanel />, { wrapper: makeWrapper() })
    // After loading resolves, no skeleton, no content — panel shows header text only
    await waitFor(() => expect(document.querySelector('.animate-pulse')).not.toBeInTheDocument())
  })
})
