import React from 'react'
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { MemoryRouter } from 'react-router-dom'
// ── Mock dependencies ─────────────────────────────────────────────────────────
vi.mock('@/features/ats')
vi.mock('@/features/settings/api/adapterConfigs.api')
vi.mock('sonner', () => ({ toast: { success: vi.fn(), error: vi.fn() } }))

// ── Lazy imports after mocks are registered ───────────────────────────────────
import { useAts, useAtsActions } from '@/features/ats'
import { SettingsPage } from '@/pages/SettingsPage'
import type { ATS } from '@/types'

// ── Helpers ───────────────────────────────────────────────────────────────────
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

const mockAtsList: ATS[] = [
  {
    name: 'greenhouse',
    display_name: 'Greenhouse',
    base_url: 'https://boards-api.greenhouse.io/v1/boards/',
    is_active: true,
    is_admin_creatable: true,
    scan_interval_min: 1440,
    inter_company_delay_s: 3,
  },
  {
    name: 'lever',
    display_name: 'Lever',
    base_url: 'https://jobs.lever.co/',
    is_active: true,
    is_admin_creatable: true,
    scan_interval_min: 720,
    inter_company_delay_s: 5,
  },
]

const mockMutateAsync = vi.fn()

// ─────────────────────────────────────────────────────────────────────────────
// SettingsPage
// ─────────────────────────────────────────────────────────────────────────────
describe('SettingsPage', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    vi.mocked(useAtsActions).mockReturnValue({
      mutateAsync: mockMutateAsync,
      isPending: false,
      isError: false,
      isSuccess: false,
      mutate: vi.fn(),
    } as never)
  })

  it('renders the Settings heading', () => {
    vi.mocked(useAts).mockReturnValue({
      data: undefined,
      isLoading: true,
      isError: false,
      refetch: vi.fn(),
    } as never)
    render(<SettingsPage />, { wrapper: makeWrapper() })
    expect(screen.getByRole('heading', { name: 'Settings' })).toBeInTheDocument()
  })

  it('renders the Adapter Configuration section heading', () => {
    vi.mocked(useAts).mockReturnValue({
      data: undefined,
      isLoading: true,
      isError: false,
      refetch: vi.fn(),
    } as never)
    render(<SettingsPage />, { wrapper: makeWrapper() })
    expect(screen.getByText('Adapter Configuration')).toBeInTheDocument()
  })

  it('shows loading skeletons while ATS configs are pending', () => {
    vi.mocked(useAts).mockReturnValue({
      data: undefined,
      isLoading: true,
      isError: false,
      refetch: vi.fn(),
    } as never)
    const { container } = render(<SettingsPage />, { wrapper: makeWrapper() })
    expect(container.querySelectorAll('.animate-pulse').length).toBeGreaterThan(0)
  })

  it('shows ATS rows when data resolves', async () => {
    vi.mocked(useAts).mockReturnValue({
      data: mockAtsList,
      isLoading: false,
      isError: false,
      refetch: vi.fn(),
    } as never)
    render(<SettingsPage />, { wrapper: makeWrapper() })
    await waitFor(() => {
      // AtsTable shows display_name as primary and name as subtitle
      expect(screen.getByText('Greenhouse')).toBeInTheDocument()
      expect(screen.getByText('Lever')).toBeInTheDocument()
    })
  })
})
