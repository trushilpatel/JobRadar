import React from 'react'
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { MemoryRouter } from 'react-router-dom'
import { NuqsTestingAdapter } from 'nuqs/adapters/testing'

// ── Mock all API modules so no real fetch calls are made ─────────────────────
vi.mock('@/features/jobs/api/jobs.api')
vi.mock('@/features/companies/api/companies.api')
vi.mock('@/features/companies/api/adminCompanies.api')
vi.mock('@/features/auth/api/adminAudit.api')
vi.mock('@/features/auth/api/adminUsers.api')
vi.mock('@/features/auth/api/me.api')
vi.mock('@/features/settings/api/adapterConfigs.api')
vi.mock('@/features/ats')

// ── Mock Clerk so pages don't try to resolve auth state ──────────────────────
vi.mock('@clerk/react', () => ({
  useAuth: () => ({ isSignedIn: true, isLoaded: true }),
  useUser: () => ({
    user: {
      publicMetadata: { role: 'user' },
      primaryEmailAddress: { emailAddress: 'test@test.com' },
    },
    isLoaded: true,
  }),
  useClerk: () => ({ signOut: vi.fn().mockResolvedValue(undefined) }),
}))

// ── Mock useIdentity so pages that consume it get a stable identity ───────────
vi.mock('@/features/auth/hooks/useIdentity', () => ({
  useIdentity: vi.fn(() => ({
    identity: {
      user_id: 'u1',
      email: 'test@test.com',
      display_name: 'Test User',
      status: 'active',
      roles: ['user'],
    },
    isAdmin: false,
    isSignedIn: true,
    isLoaded: true,
  })),
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
import * as jobsApi from '@/features/jobs'
import * as adapterConfigsApi from '@/features/settings'
import * as atsFeature from '@/features/ats'
import * as adminCompaniesApi from '@/features/companies'
import * as adminUsersApi from '@/features/auth'
import * as adminAuditApi from '@/features/auth'

import { JobsPage } from '@/pages/JobsPage'
import { AdminPipelinePage } from '@/pages/admin/AdminPipelinePage'
import { AccountPage } from '@/pages/AccountPage'
import { CompaniesPage } from '@/pages/CompaniesPage'
import { AdminUsersPage } from '@/pages/admin/AdminUsersPage'
import { AdminAuditPage } from '@/pages/admin/AdminAuditPage'

// ── Shared pending-promise helper ─────────────────────────────────────────────
function pending(): Promise<never> {
  return new Promise(() => {})
}

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
// AdminPipelinePage
// ─────────────────────────────────────────────────────────────────────────────
describe('AdminPipelinePage', () => {
  beforeEach(() => {
    vi.mocked(adapterConfigsApi.getAdapterConfigs).mockReturnValue(pending())
    vi.mocked(atsFeature.useAts).mockReturnValue({
      data: undefined,
      isLoading: true,
      isError: false,
      refetch: vi.fn(),
    } as never)
    vi.mocked(atsFeature.useAtsActions).mockReturnValue({
      mutateAsync: vi.fn(),
      isPending: false,
      isError: false,
      isSuccess: false,
      mutate: vi.fn(),
    } as never)
  })

  it('renders without crashing', () => {
    expect(() => render(<AdminPipelinePage />, { wrapper: makeWrapper() })).not.toThrow()
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
    const { getAllByText } = render(<AccountPage />, { wrapper: makeWrapper() })
    const instances = getAllByText('test@test.com')
    expect(instances.length).toBeGreaterThan(0)
    instances.forEach((el) => expect(el).toBeInTheDocument())
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
    vi.mocked(atsFeature.useAts).mockReturnValue({
      data: undefined,
      isLoading: true,
      isError: false,
      refetch: vi.fn(),
    } as never)
  })

  it('renders without crashing', () => {
    expect(() => render(<CompaniesPage />, { wrapper: makeWrapper() })).not.toThrow()
  })
})
