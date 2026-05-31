import { render, screen, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import * as insightsApi from '@/features/insights/api/insights.api'

import { TechStackPanel } from '../components/TechStackPanel'
import { CompanyVelocityPanel } from '../components/CompanyVelocityPanel'
import { PipelineFunnelPanel } from '../components/PipelineFunnelPanel'
import { RoleCategoryPanel } from '../components/RoleCategoryPanel'
import { SponsorshipTrendPanel } from '../components/SponsorshipTrendPanel'
import { CompanyActivityPanel } from '../components/CompanyActivityPanel'
import { LocationPanel } from '../components/LocationPanel'

vi.mock('@/features/insights/api/insights.api')

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
    return <QueryClientProvider client={qc}>{children}</QueryClientProvider>
  }
  return Wrapper
}

// ── TechStackPanel ─────────────────────────────────────────────────────────

describe('TechStackPanel', () => {
  it('shows skeleton while loading', () => {
    vi.mocked(insightsApi.fetchTechStack).mockReturnValue(new Promise(() => {}))
    render(<TechStackPanel />, { wrapper: makeWrapper() })
    expect(document.querySelector('.animate-pulse')).toBeInTheDocument()
  })

  it('renders chart without crashing when data is present', async () => {
    vi.mocked(insightsApi.fetchTechStack).mockResolvedValue([
      { tech: 'Go', count: 50 },
      { tech: 'TypeScript', count: 30 },
    ])
    render(<TechStackPanel />, { wrapper: makeWrapper() })
    await waitFor(() => expect(document.querySelector('.animate-pulse')).not.toBeInTheDocument())
    expect(screen.getByText('Tech Stack Trends')).toBeInTheDocument()
  })

  it('shows InsightEmpty when data is an empty array', async () => {
    vi.mocked(insightsApi.fetchTechStack).mockResolvedValue([])
    render(<TechStackPanel />, { wrapper: makeWrapper() })
    await waitFor(() => expect(screen.getByText('Not enough data yet')).toBeInTheDocument())
  })

  it('shows error state when response is null', async () => {
    vi.mocked(insightsApi.fetchTechStack).mockResolvedValue(
      null as unknown as Awaited<ReturnType<typeof insightsApi.fetchTechStack>>
    )
    render(<TechStackPanel />, { wrapper: makeWrapper() })
    await waitFor(() => expect(screen.getByText('Failed to load data')).toBeInTheDocument())
  })

  it('shows InsightError when query rejects', async () => {
    vi.mocked(insightsApi.fetchTechStack).mockRejectedValue(new Error('network error'))
    render(<TechStackPanel />, { wrapper: makeWrapper() })
    await waitFor(() => expect(screen.getByText('Failed to load data')).toBeInTheDocument())
  })
})

// ── CompanyVelocityPanel ───────────────────────────────────────────────────

describe('CompanyVelocityPanel', () => {
  const sampleRow = {
    company_id: '1',
    company_name: 'Acme',
    week_current: 5,
    week_1: 3,
    week_2: 2,
    week_3: 1,
    multiplier: 1.7,
    velocity_tag: 'rising' as const,
  }

  it('shows skeleton while loading', () => {
    vi.mocked(insightsApi.fetchVelocity).mockReturnValue(new Promise(() => {}))
    render(<CompanyVelocityPanel />, { wrapper: makeWrapper() })
    expect(document.querySelector('.animate-pulse')).toBeInTheDocument()
  })

  it('renders company name and current week count when data is present', async () => {
    vi.mocked(insightsApi.fetchVelocity).mockResolvedValue([sampleRow])
    render(<CompanyVelocityPanel />, { wrapper: makeWrapper() })
    await waitFor(() => expect(screen.getByText('Acme')).toBeInTheDocument())
    expect(screen.getByText('+5')).toBeInTheDocument()
  })

  it('shows InsightEmpty when data is an empty array', async () => {
    vi.mocked(insightsApi.fetchVelocity).mockResolvedValue([])
    render(<CompanyVelocityPanel />, { wrapper: makeWrapper() })
    await waitFor(() => expect(screen.getByText('Not enough data yet')).toBeInTheDocument())
  })

  it('shows error state when response is null', async () => {
    vi.mocked(insightsApi.fetchVelocity).mockResolvedValue(
      null as unknown as Awaited<ReturnType<typeof insightsApi.fetchVelocity>>
    )
    render(<CompanyVelocityPanel />, { wrapper: makeWrapper() })
    await waitFor(() => expect(screen.getByText('Failed to load data')).toBeInTheDocument())
  })

  it('shows InsightError when query rejects', async () => {
    vi.mocked(insightsApi.fetchVelocity).mockRejectedValue(new Error('network error'))
    render(<CompanyVelocityPanel />, { wrapper: makeWrapper() })
    await waitFor(() => expect(screen.getByText('Failed to load data')).toBeInTheDocument())
  })
})

// ── PipelineFunnelPanel ────────────────────────────────────────────────────

describe('PipelineFunnelPanel', () => {
  const sampleFunnel = [
    { stage: 'enriched', count: 100 },
    { stage: 'saved', count: 20 },
    { stage: 'applied', count: 5 },
    { stage: 'dismissed', count: 10 },
  ]

  it('shows skeleton while loading', () => {
    vi.mocked(insightsApi.fetchFunnel).mockReturnValue(new Promise(() => {}))
    render(<PipelineFunnelPanel />, { wrapper: makeWrapper() })
    expect(document.querySelector('.animate-pulse')).toBeInTheDocument()
  })

  it('renders stage labels and counts when data is present', async () => {
    vi.mocked(insightsApi.fetchFunnel).mockResolvedValue(sampleFunnel)
    render(<PipelineFunnelPanel />, { wrapper: makeWrapper() })
    await waitFor(() => expect(screen.getByText('Enriched (total)')).toBeInTheDocument())
    expect(screen.getByText('Saved')).toBeInTheDocument()
    expect(screen.getByText('Applied')).toBeInTheDocument()
    expect(screen.getByText('Dismissed')).toBeInTheDocument()
  })

  it('shows InsightEmpty when data is an empty array', async () => {
    vi.mocked(insightsApi.fetchFunnel).mockResolvedValue([])
    render(<PipelineFunnelPanel />, { wrapper: makeWrapper() })
    await waitFor(() => expect(screen.getByText('Not enough data yet')).toBeInTheDocument())
  })

  it('shows error state when response is null', async () => {
    vi.mocked(insightsApi.fetchFunnel).mockResolvedValue(
      null as unknown as Awaited<ReturnType<typeof insightsApi.fetchFunnel>>
    )
    render(<PipelineFunnelPanel />, { wrapper: makeWrapper() })
    await waitFor(() => expect(screen.getByText('Failed to load data')).toBeInTheDocument())
  })

  it('shows InsightError when query rejects', async () => {
    vi.mocked(insightsApi.fetchFunnel).mockRejectedValue(new Error('network error'))
    render(<PipelineFunnelPanel />, { wrapper: makeWrapper() })
    await waitFor(() => expect(screen.getByText('Failed to load data')).toBeInTheDocument())
  })
})

// ── RoleCategoryPanel ──────────────────────────────────────────────────────

describe('RoleCategoryPanel', () => {
  const sampleRoles = [
    { category: 'backend', count: 80 },
    { category: 'frontend', count: 40 },
  ]

  it('shows skeleton while loading', () => {
    vi.mocked(insightsApi.fetchRoleCategories).mockReturnValue(new Promise(() => {}))
    render(<RoleCategoryPanel />, { wrapper: makeWrapper() })
    expect(document.querySelector('.animate-pulse')).toBeInTheDocument()
  })

  it('renders category names and counts when data is present', async () => {
    vi.mocked(insightsApi.fetchRoleCategories).mockResolvedValue(sampleRoles)
    render(<RoleCategoryPanel />, { wrapper: makeWrapper() })
    await waitFor(() => expect(screen.getByText('backend')).toBeInTheDocument())
    expect(screen.getByText('frontend')).toBeInTheDocument()
  })

  it('shows InsightEmpty when data is an empty array', async () => {
    vi.mocked(insightsApi.fetchRoleCategories).mockResolvedValue([])
    render(<RoleCategoryPanel />, { wrapper: makeWrapper() })
    await waitFor(() => expect(screen.getByText('Not enough data yet')).toBeInTheDocument())
  })

  it('shows error state when response is null', async () => {
    vi.mocked(insightsApi.fetchRoleCategories).mockResolvedValue(
      null as unknown as Awaited<ReturnType<typeof insightsApi.fetchRoleCategories>>
    )
    render(<RoleCategoryPanel />, { wrapper: makeWrapper() })
    await waitFor(() => expect(screen.getByText('Failed to load data')).toBeInTheDocument())
  })

  it('shows InsightError when query rejects', async () => {
    vi.mocked(insightsApi.fetchRoleCategories).mockRejectedValue(new Error('network error'))
    render(<RoleCategoryPanel />, { wrapper: makeWrapper() })
    await waitFor(() => expect(screen.getByText('Failed to load data')).toBeInTheDocument())
  })
})

// ── SponsorshipTrendPanel ──────────────────────────────────────────────────

describe('SponsorshipTrendPanel', () => {
  const sampleSponsorship = [{ date: '2024-01-15', total: 100, sponsored: 40 }]

  it('shows skeleton while loading', () => {
    vi.mocked(insightsApi.fetchSponsorshipTrend).mockReturnValue(new Promise(() => {}))
    render(<SponsorshipTrendPanel />, { wrapper: makeWrapper() })
    expect(document.querySelector('.animate-pulse')).toBeInTheDocument()
  })

  it('renders chart without crashing when data is present', async () => {
    vi.mocked(insightsApi.fetchSponsorshipTrend).mockResolvedValue(sampleSponsorship)
    render(<SponsorshipTrendPanel />, { wrapper: makeWrapper() })
    await waitFor(() => expect(document.querySelector('.animate-pulse')).not.toBeInTheDocument())
    expect(screen.getByText('Sponsorship Availability')).toBeInTheDocument()
  })

  it('shows InsightEmpty when data is an empty array', async () => {
    vi.mocked(insightsApi.fetchSponsorshipTrend).mockResolvedValue([])
    render(<SponsorshipTrendPanel />, { wrapper: makeWrapper() })
    await waitFor(() => expect(screen.getByText('Not enough data yet')).toBeInTheDocument())
  })

  it('shows error state when response is null', async () => {
    vi.mocked(insightsApi.fetchSponsorshipTrend).mockResolvedValue(
      null as unknown as Awaited<ReturnType<typeof insightsApi.fetchSponsorshipTrend>>
    )
    render(<SponsorshipTrendPanel />, { wrapper: makeWrapper() })
    await waitFor(() => expect(screen.getByText('Failed to load data')).toBeInTheDocument())
  })

  it('shows InsightError when query rejects', async () => {
    vi.mocked(insightsApi.fetchSponsorshipTrend).mockRejectedValue(new Error('network error'))
    render(<SponsorshipTrendPanel />, { wrapper: makeWrapper() })
    await waitFor(() => expect(screen.getByText('Failed to load data')).toBeInTheDocument())
  })
})

// ── CompanyActivityPanel ───────────────────────────────────────────────────

describe('CompanyActivityPanel', () => {
  const sampleActivity = [
    { company_name: 'Stripe', week: '2024-01-01', count: 10 },
    { company_name: 'Stripe', week: '2024-01-08', count: 5 },
    { company_name: 'Notion', week: '2024-01-01', count: 8 },
  ]

  it('shows skeleton while loading', () => {
    vi.mocked(insightsApi.fetchCompanyActivity).mockReturnValue(new Promise(() => {}))
    render(<CompanyActivityPanel />, { wrapper: makeWrapper() })
    expect(document.querySelector('.animate-pulse')).toBeInTheDocument()
  })

  it('renders aggregated company totals when data is present', async () => {
    vi.mocked(insightsApi.fetchCompanyActivity).mockResolvedValue(sampleActivity)
    render(<CompanyActivityPanel />, { wrapper: makeWrapper() })
    await waitFor(() => expect(screen.getByText('Stripe')).toBeInTheDocument())
    expect(screen.getByText('Notion')).toBeInTheDocument()
    // Stripe total = 15, Notion total = 8
    expect(screen.getByText('15')).toBeInTheDocument()
    expect(screen.getByText('8')).toBeInTheDocument()
  })

  it('shows InsightEmpty when data is an empty array', async () => {
    vi.mocked(insightsApi.fetchCompanyActivity).mockResolvedValue([])
    render(<CompanyActivityPanel />, { wrapper: makeWrapper() })
    await waitFor(() => expect(screen.getByText('Not enough data yet')).toBeInTheDocument())
  })

  it('shows error state when response is null', async () => {
    vi.mocked(insightsApi.fetchCompanyActivity).mockResolvedValue(
      null as unknown as Awaited<ReturnType<typeof insightsApi.fetchCompanyActivity>>
    )
    render(<CompanyActivityPanel />, { wrapper: makeWrapper() })
    await waitFor(() => expect(screen.getByText('Failed to load data')).toBeInTheDocument())
  })

  it('shows InsightError when query rejects', async () => {
    vi.mocked(insightsApi.fetchCompanyActivity).mockRejectedValue(new Error('network error'))
    render(<CompanyActivityPanel />, { wrapper: makeWrapper() })
    await waitFor(() => expect(screen.getByText('Failed to load data')).toBeInTheDocument())
  })
})

// ── LocationPanel ──────────────────────────────────────────────────────────

describe('LocationPanel', () => {
  const sampleLocation = {
    countries: [{ country: 'USA', count: 10 }],
    states: [{ state: 'CA', count: 5 }],
  }

  it('shows skeleton while loading', () => {
    vi.mocked(insightsApi.fetchLocation).mockReturnValue(new Promise(() => {}))
    render(<LocationPanel />, { wrapper: makeWrapper() })
    expect(document.querySelector('.animate-pulse')).toBeInTheDocument()
  })

  it('renders country and state data when present', async () => {
    vi.mocked(insightsApi.fetchLocation).mockResolvedValue(sampleLocation)
    render(<LocationPanel />, { wrapper: makeWrapper() })
    await waitFor(() => expect(screen.getByText('USA')).toBeInTheDocument())
    expect(screen.getByText('CA')).toBeInTheDocument()
  })

  it('shows InsightEmpty when countries and states are both empty', async () => {
    vi.mocked(insightsApi.fetchLocation).mockResolvedValue({ countries: [], states: [] })
    render(<LocationPanel />, { wrapper: makeWrapper() })
    await waitFor(() => expect(screen.getByText('Not enough data yet')).toBeInTheDocument())
  })

  it('shows error state when response is null', async () => {
    vi.mocked(insightsApi.fetchLocation).mockResolvedValue(
      null as unknown as Awaited<ReturnType<typeof insightsApi.fetchLocation>>
    )
    render(<LocationPanel />, { wrapper: makeWrapper() })
    await waitFor(() => expect(screen.getByText('Failed to load data')).toBeInTheDocument())
  })

  it('shows InsightError when query rejects', async () => {
    vi.mocked(insightsApi.fetchLocation).mockRejectedValue(new Error('network error'))
    render(<LocationPanel />, { wrapper: makeWrapper() })
    await waitFor(() => expect(screen.getByText('Failed to load data')).toBeInTheDocument())
  })
})
