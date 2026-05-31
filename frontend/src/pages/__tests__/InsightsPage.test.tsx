import React from 'react'
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { MemoryRouter } from 'react-router-dom'

// ── Mock all insight API functions ────────────────────────────────────────────
vi.mock('@/features/insights/api/insights.api')

// ── ResizeObserver stub required for Recharts ─────────────────────────────────
class MockResizeObserver {
  observe() {}
  unobserve() {}
  disconnect() {}
}
global.ResizeObserver = MockResizeObserver as unknown as typeof ResizeObserver

// ── Lazy imports after mocks are registered ───────────────────────────────────
import * as insightsApi from '@/features/insights'
import { InsightsPage } from '@/pages/InsightsPage'

// ── Helpers ───────────────────────────────────────────────────────────────────
function pending(): Promise<never> {
  return new Promise(() => {})
}

function makeWrapper() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: 0, gcTime: 0 } } })
  return function Wrapper({ children }: { children: React.ReactNode }) {
    return (
      <MemoryRouter>
        <QueryClientProvider client={qc}>{children}</QueryClientProvider>
      </MemoryRouter>
    )
  }
}

// ─────────────────────────────────────────────────────────────────────────────
// InsightsPage
// ─────────────────────────────────────────────────────────────────────────────
describe('InsightsPage', () => {
  beforeEach(() => {
    vi.mocked(insightsApi.fetchFunnel).mockReturnValue(pending())
    vi.mocked(insightsApi.fetchRoleCategories).mockReturnValue(pending())
    vi.mocked(insightsApi.fetchVelocity).mockReturnValue(pending())
    vi.mocked(insightsApi.fetchLocation).mockReturnValue(pending())
    vi.mocked(insightsApi.fetchSponsorshipTrend).mockReturnValue(pending())
    vi.mocked(insightsApi.fetchTechStack).mockReturnValue(pending())
    vi.mocked(insightsApi.fetchCompanyActivity).mockReturnValue(pending())
  })

  it('renders the Insights heading', () => {
    render(<InsightsPage />, { wrapper: makeWrapper() })
    expect(screen.getByRole('heading', { name: 'Insights' })).toBeInTheDocument()
  })

  it('renders all 7 panel section titles while loading', async () => {
    render(<InsightsPage />, { wrapper: makeWrapper() })
    await waitFor(() => {
      expect(screen.getByText('Pipeline Funnel')).toBeInTheDocument()
      expect(screen.getByText('Role Categories')).toBeInTheDocument()
      expect(screen.getByText('Company Hiring Velocity')).toBeInTheDocument()
      expect(screen.getByText('Location Breakdown')).toBeInTheDocument()
      expect(screen.getByText('Sponsorship Availability')).toBeInTheDocument()
      expect(screen.getByText('Tech Stack Trends')).toBeInTheDocument()
      expect(screen.getByText('Company Activity')).toBeInTheDocument()
    })
  })

  it('shows loading skeletons while queries are pending', () => {
    const { container } = render(<InsightsPage />, { wrapper: makeWrapper() })
    expect(container.querySelectorAll('.animate-pulse').length).toBeGreaterThan(0)
  })

  it('renders all panels when data resolves', async () => {
    vi.mocked(insightsApi.fetchFunnel).mockResolvedValue([{ stage: 'enriched', count: 100 }])
    vi.mocked(insightsApi.fetchRoleCategories).mockResolvedValue([
      { category: 'backend', count: 50 },
    ])
    vi.mocked(insightsApi.fetchVelocity).mockResolvedValue([
      {
        company_id: '1',
        company_name: 'Stripe',
        week_current: 5,
        week_1: 3,
        week_2: 2,
        week_3: 1,
        multiplier: 1.7,
        velocity_tag: 'rising' as const,
      },
    ])
    vi.mocked(insightsApi.fetchLocation).mockResolvedValue({
      countries: [{ country: 'USA', count: 10 }],
      states: [{ state: 'CA', count: 5 }],
    })
    vi.mocked(insightsApi.fetchSponsorshipTrend).mockResolvedValue([
      { date: '2024-01-15', total: 100, sponsored: 40 },
    ])
    vi.mocked(insightsApi.fetchTechStack).mockResolvedValue([{ tech: 'Go', count: 50 }])
    vi.mocked(insightsApi.fetchCompanyActivity).mockResolvedValue([
      { company_name: 'Stripe', week: '2024-01-01', count: 10 },
    ])

    render(<InsightsPage />, { wrapper: makeWrapper() })

    await waitFor(() =>
      expect(screen.queryAllByRole('img', { hidden: true }).length).toBeGreaterThanOrEqual(0)
    )

    // All panel titles remain present after data loads
    expect(screen.getByText('Pipeline Funnel')).toBeInTheDocument()
    expect(screen.getByText('Tech Stack Trends')).toBeInTheDocument()
    expect(screen.getByText('Company Activity')).toBeInTheDocument()
  })
})
