import React from 'react'
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { MemoryRouter } from 'react-router-dom'
// ── Mock dependencies ─────────────────────────────────────────────────────────
vi.mock('@/features/ats')
vi.mock('@/features/settings/api/adapterConfigs.api')
vi.mock('@/lib/api/pipeline')
vi.mock('sonner', () => ({ toast: { success: vi.fn(), error: vi.fn() } }))

// ── Lazy imports after mocks are registered ───────────────────────────────────
import { useAts, useAtsActions } from '@/features/ats'
import * as pipelineApi from '@/lib/api/pipeline'
import { AdminPipelinePage } from '@/pages/admin/AdminPipelinePage'
import type { ATS, PipelineStats } from '@/types'

const emptyPipelineStats: PipelineStats = {
  total_companies: 5,
  active_companies: 5,
  queued: 0,
  crawling: 0,
  awaiting_filter: 0,
  filter_retries: { '1m': 0, '10m': 0, '1h': 0, dlq: 0 },
  awaiting_enrichment: 0,
  enricher_retries: { '1m': 0, '10m': 0, '1h': 0, dlq: 0 },
  new_jobs_24h: 12,
  errors_24h: 0,
}

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
// AdminPipelinePage
// ─────────────────────────────────────────────────────────────────────────────
describe('AdminPipelinePage', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    vi.mocked(useAtsActions).mockReturnValue({
      mutateAsync: mockMutateAsync,
      isPending: false,
      isError: false,
      isSuccess: false,
      mutate: vi.fn(),
    } as never)
    vi.mocked(pipelineApi.fetchPipelineStats).mockResolvedValue(emptyPipelineStats)
  })

  it('renders the Pipeline heading', () => {
    vi.mocked(useAts).mockReturnValue({
      data: undefined,
      isLoading: true,
      isError: false,
      refetch: vi.fn(),
    } as never)
    render(<AdminPipelinePage />, { wrapper: makeWrapper() })
    expect(screen.getByRole('heading', { name: 'Pipeline' })).toBeInTheDocument()
  })

  it('renders the Adapter Configuration section heading', () => {
    vi.mocked(useAts).mockReturnValue({
      data: undefined,
      isLoading: true,
      isError: false,
      refetch: vi.fn(),
    } as never)
    render(<AdminPipelinePage />, { wrapper: makeWrapper() })
    expect(screen.getByText('Adapter Configuration')).toBeInTheDocument()
  })

  it('shows loading skeletons while ATS configs are pending', () => {
    vi.mocked(useAts).mockReturnValue({
      data: undefined,
      isLoading: true,
      isError: false,
      refetch: vi.fn(),
    } as never)
    const { container } = render(<AdminPipelinePage />, { wrapper: makeWrapper() })
    expect(container.querySelectorAll('.animate-pulse').length).toBeGreaterThan(0)
  })

  it('shows ATS rows when data resolves', async () => {
    vi.mocked(useAts).mockReturnValue({
      data: mockAtsList,
      isLoading: false,
      isError: false,
      refetch: vi.fn(),
    } as never)
    render(<AdminPipelinePage />, { wrapper: makeWrapper() })
    await waitFor(() => {
      // AtsTable shows display_name as primary and name as subtitle
      expect(screen.getByText('Greenhouse')).toBeInTheDocument()
      expect(screen.getByText('Lever')).toBeInTheDocument()
    })
  })

  it('shows error state when ATS fetch fails', () => {
    vi.mocked(useAts).mockReturnValue({
      data: undefined,
      isLoading: false,
      isError: true,
      refetch: vi.fn(),
    } as never)
    render(<AdminPipelinePage />, { wrapper: makeWrapper() })
    expect(screen.getByText('Failed to load ATS configs.')).toBeInTheDocument()
    // Multiple Retry buttons exist (ATS + pipeline retry tiers); verify at least one renders.
    expect(screen.getAllByRole('button', { name: 'Retry' }).length).toBeGreaterThanOrEqual(1)
  })

  it('shows pipeline health stats when pipeline data resolves', async () => {
    vi.mocked(useAts).mockReturnValue({
      data: [],
      isLoading: false,
      isError: false,
      refetch: vi.fn(),
    } as never)
    vi.mocked(pipelineApi.fetchPipelineStats).mockResolvedValue({
      ...emptyPipelineStats,
      queued: 3,
      new_jobs_24h: 42,
    })
    render(<AdminPipelinePage />, { wrapper: makeWrapper() })
    await waitFor(() => {
      expect(screen.getByText('Pipeline Health')).toBeInTheDocument()
      expect(screen.getByText('42')).toBeInTheDocument()
    })
  })

  it('shows pipeline error state when pipeline fetch fails', async () => {
    vi.mocked(useAts).mockReturnValue({
      data: [],
      isLoading: false,
      isError: false,
      refetch: vi.fn(),
    } as never)
    vi.mocked(pipelineApi.fetchPipelineStats).mockRejectedValue(new Error('network'))
    render(<AdminPipelinePage />, { wrapper: makeWrapper() })
    await waitFor(() => {
      expect(screen.getByText(/Pipeline stats unavailable/)).toBeInTheDocument()
    })
  })

  it('shows healthy status indicator when no errors or retries', async () => {
    vi.mocked(useAts).mockReturnValue({
      data: [],
      isLoading: false,
      isError: false,
      refetch: vi.fn(),
    } as never)
    render(<AdminPipelinePage />, { wrapper: makeWrapper() })
    await waitFor(() => {
      expect(screen.getByText('Pipeline healthy')).toBeInTheDocument()
    })
  })

  it('shows unhealthy status when errors_24h > 0', async () => {
    vi.mocked(useAts).mockReturnValue({
      data: [],
      isLoading: false,
      isError: false,
      refetch: vi.fn(),
    } as never)
    vi.mocked(pipelineApi.fetchPipelineStats).mockResolvedValue({
      ...emptyPipelineStats,
      errors_24h: 7,
    })
    render(<AdminPipelinePage />, { wrapper: makeWrapper() })
    await waitFor(() => {
      expect(screen.getByText(/7 errors in 24h/)).toBeInTheDocument()
    })
  })
})
