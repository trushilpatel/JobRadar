import { render, screen, waitFor, fireEvent } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { MemoryRouter } from 'react-router-dom'
import * as dashboardApi from '@/features/dashboard/api/dashboard.api'

import { ProgressPanel } from '../components/ProgressPanel'
import { MarketPanel } from '../components/MarketPanel'
import { HeatmapPanel } from '../components/HeatmapPanel'
import { InterviewingPanel } from '../components/InterviewingPanel'
import { VelocityPanel } from '../components/VelocityPanel'
import { TechTrendsPanel } from '../components/TechTrendsPanel'

vi.mock('@/features/dashboard/api/dashboard.api')

const navigateMock = vi.fn()
vi.mock('react-router-dom', async () => {
  const actual = await vi.importActual<typeof import('react-router-dom')>('react-router-dom')
  return { ...actual, useNavigate: () => navigateMock }
})

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
    // sampleProgress.offer_count = 0 → sub-label is 'no offers yet'
    await waitFor(() => expect(screen.getByText('no offers yet')).toBeInTheDocument())
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
    await waitFor(() => expect(screen.getByText('no offers yet')).toBeInTheDocument())
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

  it('shows the error message when the query rejects', async () => {
    vi.mocked(dashboardApi.fetchMarket).mockRejectedValue(new Error('boom'))
    render(<MarketPanel />, { wrapper: makeWrapper() })
    await waitFor(() => expect(screen.getByText('Failed to load market data')).toBeInTheDocument())
  })

  it('navigates to the 24h jobs feed when the New jobs stat is clicked', async () => {
    navigateMock.mockClear()
    vi.mocked(dashboardApi.fetchMarket).mockResolvedValue(sampleMarket)
    render(<MarketPanel />, { wrapper: makeWrapper() })
    await waitFor(() => expect(screen.getByRole('button')).toBeInTheDocument())
    await userEvent.click(screen.getByRole('button'))
    expect(navigateMock).toHaveBeenCalledWith('/jobs?since=24h')
  })

  it('navigates on Enter key on the New jobs stat', async () => {
    navigateMock.mockClear()
    vi.mocked(dashboardApi.fetchMarket).mockResolvedValue(sampleMarket)
    render(<MarketPanel />, { wrapper: makeWrapper() })
    await waitFor(() => expect(screen.getByRole('button')).toBeInTheDocument())
    fireEvent.keyDown(screen.getByRole('button'), { key: 'Enter' })
    expect(navigateMock).toHaveBeenCalledWith('/jobs?since=24h')
  })

  it('navigates on Space key on the New jobs stat', async () => {
    navigateMock.mockClear()
    vi.mocked(dashboardApi.fetchMarket).mockResolvedValue(sampleMarket)
    render(<MarketPanel />, { wrapper: makeWrapper() })
    await waitFor(() => expect(screen.getByRole('button')).toBeInTheDocument())
    fireEvent.keyDown(screen.getByRole('button'), { key: ' ' })
    expect(navigateMock).toHaveBeenCalledWith('/jobs?since=24h')
  })

  it('ignores unrelated keys on the New jobs stat', async () => {
    navigateMock.mockClear()
    vi.mocked(dashboardApi.fetchMarket).mockResolvedValue(sampleMarket)
    render(<MarketPanel />, { wrapper: makeWrapper() })
    await waitFor(() => expect(screen.getByRole('button')).toBeInTheDocument())
    fireEvent.keyDown(screen.getByRole('button'), { key: 'a' })
    expect(navigateMock).not.toHaveBeenCalled()
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
      { ...sampleRows[0]!, velocity_tag: 'normal', multiplier: 1 },
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
        m1: 10,
        m2: 15,
        m3: 20,
        m4: 30,
        growth_pct: 50,
        adoption_pct: 12,
        is_new: false,
      },
    ],
    new_entrants: [
      {
        tech: 'Rust',
        m1: 0,
        m2: 1,
        m3: 3,
        m4: 8,
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
      rising: [{ ...sampleTrends.rising[0]!, is_new: true, growth_pct: 0 }],
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

  it('renders subtitle "by job demand" (AC-7)', async () => {
    vi.mocked(dashboardApi.fetchTechTrends).mockResolvedValue(sampleTrends)
    render(<TechTrendsPanel />, { wrapper: makeWrapper() })
    await waitFor(() =>
      expect(screen.getByText(/tech stack trends — by job demand/i)).toBeInTheDocument()
    )
  })

  it('renders monthly column headers M1..M4 (AC-8)', async () => {
    vi.mocked(dashboardApi.fetchTechTrends).mockResolvedValue(sampleTrends)
    render(<TechTrendsPanel />, { wrapper: makeWrapper() })
    await waitFor(() => expect(screen.getAllByText('M1').length).toBeGreaterThan(0))
    expect(screen.getAllByText('M2').length).toBeGreaterThan(0)
    expect(screen.getAllByText('M3').length).toBeGreaterThan(0)
    expect(screen.getAllByText('M4').length).toBeGreaterThan(0)
  })

  it('renders Rising subtext with monthly thresholds (AC-9)', async () => {
    vi.mocked(dashboardApi.fetchTechTrends).mockResolvedValue(sampleTrends)
    render(<TechTrendsPanel />, { wrapper: makeWrapper() })
    await waitFor(() =>
      expect(screen.getByText(/≥2% of jobs · ≥10 jobs · grew/)).toBeInTheDocument()
    )
  })

  it('renders New Entrants subtext with monthly lookback (AC-10)', async () => {
    vi.mocked(dashboardApi.fetchTechTrends).mockResolvedValue(sampleTrends)
    render(<TechTrendsPanel />, { wrapper: makeWrapper() })
    await waitFor(() =>
      expect(screen.getByText(/≥2% of jobs · ≥10 jobs · was/)).toBeInTheDocument()
    )
  })
})
