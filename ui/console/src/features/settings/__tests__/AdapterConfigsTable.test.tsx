import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, fireEvent, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { createElement } from 'react'
import { AdapterConfigsTable } from '../components/AdapterConfigsTable'
import type { AdapterConfig } from '@/types'

vi.mock('sonner', () => ({ toast: { success: vi.fn(), error: vi.fn() } }))

const mockRefetch = vi.fn()
const mockUpdateConfig = vi.fn()

vi.mock('../hooks/useAdapterConfigs', () => ({
  useAdapterConfigs: vi.fn(),
}))

vi.mock('../hooks/useAdapterConfigActions', () => ({
  useAdapterConfigActions: vi.fn(),
}))

import { useAdapterConfigs } from '../hooks/useAdapterConfigs'
import { useAdapterConfigActions } from '../hooks/useAdapterConfigActions'

const mockConfigs: AdapterConfig[] = [
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

function makeWrapper() {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return ({ children }: { children: React.ReactNode }) =>
    createElement(QueryClientProvider, { client }, children)
}

describe('AdapterConfigsTable', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    vi.mocked(useAdapterConfigActions).mockReturnValue({ updateConfig: mockUpdateConfig })
  })

  // ── Loading state ────────────────────────────────────────────────────────────

  it('renders skeleton rows while loading', () => {
    vi.mocked(useAdapterConfigs).mockReturnValue({
      data: undefined,
      isLoading: true,
      isError: false,
      refetch: mockRefetch,
    } as never)

    const { container } = render(<AdapterConfigsTable />, { wrapper: makeWrapper() })
    const pulseEls = container.querySelectorAll('.animate-pulse')
    expect(pulseEls.length).toBeGreaterThan(0)
    expect(screen.queryByRole('table')).not.toBeInTheDocument()
  })

  // ── Error state ──────────────────────────────────────────────────────────────

  it('renders error message and retry button when isError is true', () => {
    vi.mocked(useAdapterConfigs).mockReturnValue({
      data: undefined,
      isLoading: false,
      isError: true,
      refetch: mockRefetch,
    } as never)

    render(<AdapterConfigsTable />, { wrapper: makeWrapper() })
    expect(screen.getByText(/Failed to load adapter configs/)).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Retry' })).toBeInTheDocument()
  })

  it('calls refetch when Retry button is clicked', () => {
    vi.mocked(useAdapterConfigs).mockReturnValue({
      data: undefined,
      isLoading: false,
      isError: true,
      refetch: mockRefetch,
    } as never)

    render(<AdapterConfigsTable />, { wrapper: makeWrapper() })
    fireEvent.click(screen.getByRole('button', { name: 'Retry' }))
    expect(mockRefetch).toHaveBeenCalledTimes(1)
  })

  // ── Empty state ──────────────────────────────────────────────────────────────

  it('renders empty state when configs list is empty', () => {
    vi.mocked(useAdapterConfigs).mockReturnValue({
      data: [],
      isLoading: false,
      isError: false,
      refetch: mockRefetch,
    } as never)

    render(<AdapterConfigsTable />, { wrapper: makeWrapper() })
    expect(screen.getByText('No adapter configurations found.')).toBeInTheDocument()
    expect(screen.queryByRole('table')).not.toBeInTheDocument()
  })

  // ── Data rendering ──────────────────────────────────────────────────────────

  it('renders adapter config rows in the table', () => {
    vi.mocked(useAdapterConfigs).mockReturnValue({
      data: mockConfigs,
      isLoading: false,
      isError: false,
      refetch: mockRefetch,
    } as never)

    render(<AdapterConfigsTable />, { wrapper: makeWrapper() })
    expect(screen.getByRole('table')).toBeInTheDocument()
    expect(screen.getByText('greenhouse')).toBeInTheDocument()
    expect(screen.getByText('lever')).toBeInTheDocument()
  })

  it('renders "Never" for null last_scheduled_at', () => {
    vi.mocked(useAdapterConfigs).mockReturnValue({
      data: [mockConfigs[0]],
      isLoading: false,
      isError: false,
      refetch: mockRefetch,
    } as never)

    render(<AdapterConfigsTable />, { wrapper: makeWrapper() })
    expect(screen.getByText('Never')).toBeInTheDocument()
  })

  // ── Confirmation modal ──────────────────────────────────────────────────────

  it('opens confirmation modal when an AdapterConfigCell triggers a save', async () => {
    vi.mocked(useAdapterConfigs).mockReturnValue({
      data: mockConfigs,
      isLoading: false,
      isError: false,
      refetch: mockRefetch,
    } as never)

    render(<AdapterConfigsTable />, { wrapper: makeWrapper() })

    // Click the first editable cell (greenhouse scan interval value)
    const editButtons = screen.getAllByTitle('Click to edit')
    fireEvent.click(editButtons[0])

    // Change the value and press Enter
    const input = screen.getByRole('spinbutton')
    fireEvent.change(input, { target: { value: '720' } })
    fireEvent.keyDown(input, { key: 'Enter' })

    await waitFor(() => {
      expect(screen.getByText('Confirm Change')).toBeInTheDocument()
    })
  })

  it('closes confirmation modal when Cancel is clicked', async () => {
    vi.mocked(useAdapterConfigs).mockReturnValue({
      data: mockConfigs,
      isLoading: false,
      isError: false,
      refetch: mockRefetch,
    } as never)

    render(<AdapterConfigsTable />, { wrapper: makeWrapper() })

    // Open modal
    const editButtons = screen.getAllByTitle('Click to edit')
    fireEvent.click(editButtons[0])
    const input = screen.getByRole('spinbutton')
    fireEvent.change(input, { target: { value: '720' } })
    fireEvent.keyDown(input, { key: 'Enter' })

    await waitFor(() => {
      expect(screen.getByText('Confirm Change')).toBeInTheDocument()
    })

    // Click Cancel
    fireEvent.click(screen.getByRole('button', { name: 'Cancel' }))

    await waitFor(() => {
      expect(screen.queryByText('Confirm Change')).not.toBeInTheDocument()
    })
  })

  it('calls updateConfig when Confirm is clicked in modal', async () => {
    mockUpdateConfig.mockResolvedValue(undefined)
    vi.mocked(useAdapterConfigs).mockReturnValue({
      data: mockConfigs,
      isLoading: false,
      isError: false,
      refetch: mockRefetch,
    } as never)

    render(<AdapterConfigsTable />, { wrapper: makeWrapper() })

    // Open modal by editing the scan interval cell for greenhouse
    const editButtons = screen.getAllByTitle('Click to edit')
    fireEvent.click(editButtons[0])
    const input = screen.getByRole('spinbutton')
    fireEvent.change(input, { target: { value: '720' } })
    fireEvent.keyDown(input, { key: 'Enter' })

    await waitFor(() => {
      expect(screen.getByText('Confirm Change')).toBeInTheDocument()
    })

    fireEvent.click(screen.getByRole('button', { name: 'Confirm' }))

    await waitFor(() => {
      expect(mockUpdateConfig).toHaveBeenCalledWith(
        'greenhouse',
        expect.objectContaining({ scan_interval_minutes: 720 })
      )
    })
  })
})
