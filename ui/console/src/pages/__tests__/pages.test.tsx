import React from 'react'
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { MemoryRouter } from 'react-router-dom'
import { NuqsTestingAdapter } from 'nuqs/adapters/testing'

// ── Mock all API modules so no real fetch calls are made ─────────────────────
vi.mock('@/api/dashboard')
vi.mock('@/api/insights')
vi.mock('@/api/jobs')
vi.mock('@/api/companies')
vi.mock('@/api/adminCompanies')
vi.mock('@/api/adminAudit')
vi.mock('@/api/adminUsers')
vi.mock('@/api/me')
vi.mock('@/api/adapterConfigs')

// ── Mock auth store so pages don't try to fetch identity ─────────────────────
vi.mock('@/features/auth/store', () => ({
  useAuthStore: vi.fn((selector: (s: unknown) => unknown) =>
    selector({
      session: { access_token: 'tok', user: { id: 'u1', email: 'test@test.com' } },
      identity: {
        user_id: 'u1',
        email: 'test@test.com',
        display_name: 'Test User',
        status: 'active',
        roles: ['user'],
      },
      loading: false,
      initialized: true,
      isAdmin: false,
      refreshIdentity: vi.fn(),
      signOut: vi.fn(),
    })
  ),
}))

// ── Mock supabase so AccountPage doesn't blow up ──────────────────────────────
vi.mock('@/lib/supabase', () => ({
  supabase: {
    auth: {
      signOut: vi.fn().mockResolvedValue({ error: null }),
    },
  },
}))

// ── ResizeObserver stub required for Recharts ────────────────────────────────
class MockResizeObserver {
  observe() {}
  unobserve() {}
  disconnect() {}
}
global.ResizeObserver = MockResizeObserver as unknown as typeof ResizeObserver

// ── Wrapper factory ───────────────────────────────────────────────────────────
function makeWrapper(initialEntries = ['/']) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: 0, gcTime: 0 } } })
  return function Wrapper({ children }: { children: React.ReactNode }) {
    return (
      <MemoryRouter initialEntries={initialEntries}>
        <QueryClientProvider client={qc}>
          <NuqsTestingAdapter hasMemory>{children}</NuqsTestingAdapter>
        </QueryClientProvider>
      </MemoryRouter>
    )
  }
}

// ── Lazy imports (after mocks are registered) ─────────────────────────────────
import * as dashboardApi from '@/api/dashboard'
import * as insightsApi from '@/api/insights'
import * as jobsApi from '@/api/jobs'
import * as adapterConfigsApi from '@/api/adapterConfigs'
import * as adminCompaniesApi from '@/api/adminCompanies'
import * as adminUsersApi from '@/api/adminUsers'
import * as adminAuditApi from '@/api/adminAudit'

import { DashboardPage } from '@/pages/DashboardPage'
import { JobsPage } from '@/pages/JobsPage'
import { InsightsPage } from '@/pages/InsightsPage'
import { SettingsPage } from '@/pages/SettingsPage'
import { AccountPage } from '@/pages/AccountPage'
import { CompaniesPage } from '@/pages/CompaniesPage'
import { AdminUsersPage } from '@/pages/admin/AdminUsersPage'
import { AdminAuditPage } from '@/pages/admin/AdminAuditPage'

// ── Shared pending-promise helper ─────────────────────────────────────────────
function pending(): Promise<never> {
  return new Promise(() => {})
}

// ─────────────────────────────────────────────────────────────────────────────
// DashboardPage
// ─────────────────────────────────────────────────────────────────────────────
describe('DashboardPage', () => {
  beforeEach(() => {
    vi.mocked(dashboardApi.fetchProgress).mockReturnValue(pending())
    vi.mocked(dashboardApi.fetchMarket).mockReturnValue(pending())
    vi.mocked(dashboardApi.fetchHeatmap).mockReturnValue(pending())
    vi.mocked(dashboardApi.fetchInterviews).mockReturnValue(pending())
    vi.mocked(dashboardApi.fetchVelocity).mockReturnValue(pending())
    vi.mocked(dashboardApi.fetchTechTrends).mockReturnValue(pending())
    vi.mocked(dashboardApi.fetchPipelineStats).mockReturnValue(pending())
  })

  it('renders without crashing', () => {
    expect(() => render(<DashboardPage />, { wrapper: makeWrapper() })).not.toThrow()
  })
})

// ─────────────────────────────────────────────────────────────────────────────
// JobsPage
// ─────────────────────────────────────────────────────────────────────────────
describe('JobsPage', () => {
  beforeEach(() => {
    // useJobs calls fetchJobs (infinite query), fetchJobCounts, fetchJobCompanies
    vi.mocked(jobsApi.fetchJobs).mockReturnValue(pending())
    vi.mocked(jobsApi.fetchJobCounts).mockReturnValue(pending())
    vi.mocked(jobsApi.fetchJobCompanies).mockReturnValue(pending())
  })

  it('renders without crashing', () => {
    expect(() => render(<JobsPage />, { wrapper: makeWrapper() })).not.toThrow()
  })
})

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

  it('renders without crashing', () => {
    expect(() => render(<InsightsPage />, { wrapper: makeWrapper() })).not.toThrow()
  })
})

// ─────────────────────────────────────────────────────────────────────────────
// SettingsPage
// ─────────────────────────────────────────────────────────────────────────────
describe('SettingsPage', () => {
  beforeEach(() => {
    vi.mocked(adapterConfigsApi.getAdapterConfigs).mockReturnValue(pending())
  })

  it('renders without crashing', () => {
    expect(() => render(<SettingsPage />, { wrapper: makeWrapper() })).not.toThrow()
  })
})

// ─────────────────────────────────────────────────────────────────────────────
// AccountPage
// ─────────────────────────────────────────────────────────────────────────────
describe('AccountPage', () => {
  it('renders without crashing', () => {
    expect(() => render(<AccountPage />, { wrapper: makeWrapper() })).not.toThrow()
  })

  it('pre-populates display name from identity', () => {
    const { getByLabelText } = render(<AccountPage />, { wrapper: makeWrapper() })
    const input = getByLabelText(/display name/i) as HTMLInputElement
    expect(input.value).toBe('Test User')
  })

  it('renders email from identity', () => {
    const { getByText } = render(<AccountPage />, { wrapper: makeWrapper() })
    expect(getByText('test@test.com')).toBeInTheDocument()
  })

  it('shows Delete account button', () => {
    const { getByRole } = render(<AccountPage />, { wrapper: makeWrapper() })
    expect(getByRole('button', { name: /delete account/i })).toBeInTheDocument()
  })
})

// ─────────────────────────────────────────────────────────────────────────────
// AdminUsersPage  (isAdmin = true)
// ─────────────────────────────────────────────────────────────────────────────
describe('AdminUsersPage', () => {
  beforeEach(() => {
    vi.mocked(adminUsersApi.fetchAdminUsers).mockReturnValue(pending())
  })

  it('renders without crashing', () => {
    expect(() => render(<AdminUsersPage />, { wrapper: makeWrapper() })).not.toThrow()
  })
})

// ─────────────────────────────────────────────────────────────────────────────
// AdminAuditPage
// ─────────────────────────────────────────────────────────────────────────────
describe('AdminAuditPage', () => {
  beforeEach(() => {
    vi.mocked(adminAuditApi.fetchAuditLog).mockReturnValue(pending())
  })

  it('renders without crashing', () => {
    expect(() => render(<AdminAuditPage />, { wrapper: makeWrapper() })).not.toThrow()
  })
})

// ─────────────────────────────────────────────────────────────────────────────
// CompaniesPage
// ─────────────────────────────────────────────────────────────────────────────
describe('CompaniesPage', () => {
  beforeEach(() => {
    vi.mocked(adminCompaniesApi.fetchAdminCompanies).mockReturnValue(pending())
    vi.mocked(adapterConfigsApi.getAdapterConfigs).mockReturnValue(pending())
  })

  it('renders without crashing', () => {
    expect(() => render(<CompaniesPage />, { wrapper: makeWrapper() })).not.toThrow()
  })
})
