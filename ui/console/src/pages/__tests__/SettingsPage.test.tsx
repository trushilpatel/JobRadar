import React from 'react'
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { MemoryRouter } from 'react-router-dom'
// ── Mock dependencies ─────────────────────────────────────────────────────────
vi.mock('@/api/adapterConfigs')
vi.mock('sonner', () => ({ toast: { success: vi.fn(), error: vi.fn() } }))

// ── Lazy imports after mocks are registered ───────────────────────────────────
import * as adapterConfigsApi from '@/api/adapterConfigs'
import { SettingsPage } from '@/pages/SettingsPage'
import type { AdapterConfig } from '@/api/adapterConfigs'

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

const mockAdapters: AdapterConfig[] = [
  {
    adapter_type: 'greenhouse',
    scan_interval_minutes: 1440,
    inter_company_delay_s: 3,
    last_scheduled_at: null,
    updated_at: '2026-01-01T00:00:00Z',
  },
  {
    adapter_type: 'lever',
    scan_interval_minutes: 720,
    inter_company_delay_s: 5,
    last_scheduled_at: '2026-04-24T12:00:00Z',
    updated_at: '2026-01-02T00:00:00Z',
  },
]

// ─────────────────────────────────────────────────────────────────────────────
// SettingsPage
// ─────────────────────────────────────────────────────────────────────────────
describe('SettingsPage', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('renders the Settings heading', () => {
    vi.mocked(adapterConfigsApi.getAdapterConfigs).mockReturnValue(pending())
    render(<SettingsPage />, { wrapper: makeWrapper() })
    expect(screen.getByRole('heading', { name: 'Settings' })).toBeInTheDocument()
  })

  it('renders the Adapter Configuration section heading', () => {
    vi.mocked(adapterConfigsApi.getAdapterConfigs).mockReturnValue(pending())
    render(<SettingsPage />, { wrapper: makeWrapper() })
    expect(screen.getByText('Adapter Configuration')).toBeInTheDocument()
  })

  it('shows loading skeletons while adapter configs are pending', () => {
    vi.mocked(adapterConfigsApi.getAdapterConfigs).mockReturnValue(pending())
    const { container } = render(<SettingsPage />, { wrapper: makeWrapper() })
    expect(container.querySelectorAll('.animate-pulse').length).toBeGreaterThan(0)
  })

  it('shows adapter rows when data resolves', async () => {
    vi.mocked(adapterConfigsApi.getAdapterConfigs).mockResolvedValue(mockAdapters)
    render(<SettingsPage />, { wrapper: makeWrapper() })
    await waitFor(() => {
      expect(screen.getByText('greenhouse')).toBeInTheDocument()
      expect(screen.getByText('lever')).toBeInTheDocument()
    })
  })
})
