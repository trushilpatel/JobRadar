import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, waitFor, fireEvent } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { createElement } from 'react'
import { ScanIntervalOverrideModal } from '../components/ScanIntervalOverrideModal'

vi.mock('sonner', () => ({ toast: { success: vi.fn(), error: vi.fn() } }))

const mockSetOverride = vi.fn()
vi.mock('../hooks/useScanIntervalOverride', () => ({
  useScanIntervalOverride: () => ({
    setOverride: mockSetOverride,
    loading: false,
  }),
}))

function makeWrapper() {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return ({ children }: { children: React.ReactNode }) =>
    createElement(QueryClientProvider, { client }, children)
}

const defaultProps = {
  open: true,
  onClose: vi.fn(),
  companyId: 'stripe',
  companyName: 'Stripe',
  adapterDefault: 1440,
  currentOverride: null,
}

describe('ScanIntervalOverrideModal', () => {
  beforeEach(() => {
    mockSetOverride.mockReset()
    vi.mocked(defaultProps.onClose).mockReset?.()
  })

  it('renders with adapter default shown', () => {
    render(<ScanIntervalOverrideModal {...defaultProps} />, { wrapper: makeWrapper() })
    expect(screen.getByText(/Override the adapter default \(1440 min\)/)).toBeInTheDocument()
    expect(screen.getByText('Adapter default: 1440 min')).toBeInTheDocument()
  })

  it('prefills input when currentOverride is set', () => {
    render(
      <ScanIntervalOverrideModal {...defaultProps} currentOverride={120} />,
      { wrapper: makeWrapper() }
    )
    const input = screen.getByRole('spinbutton') as HTMLInputElement
    expect(input.value).toBe('120')
  })

  it('shows validation error for value below min', async () => {
    const user = userEvent.setup()
    render(<ScanIntervalOverrideModal {...defaultProps} />, { wrapper: makeWrapper() })

    const input = screen.getByRole('spinbutton')
    await user.clear(input)
    await user.type(input, '0')
    fireEvent.submit(input.closest('form')!)

    await waitFor(() => {
      expect(screen.getByText(/Enter a value between 1 and 10080/)).toBeInTheDocument()
    })
    expect(mockSetOverride).not.toHaveBeenCalled()
  })

  it('shows validation error for value above max', async () => {
    const user = userEvent.setup()
    render(<ScanIntervalOverrideModal {...defaultProps} />, { wrapper: makeWrapper() })

    const input = screen.getByRole('spinbutton')
    await user.clear(input)
    await user.type(input, '99999')
    fireEvent.submit(input.closest('form')!)

    await waitFor(() => {
      expect(screen.getByText(/Enter a value between 1 and 10080/)).toBeInTheDocument()
    })
  })

  it('calls setOverride with correct value on valid submit', async () => {
    mockSetOverride.mockResolvedValue(undefined)
    const onClose = vi.fn()
    const user = userEvent.setup()
    render(
      <ScanIntervalOverrideModal {...defaultProps} onClose={onClose} />,
      { wrapper: makeWrapper() }
    )

    const input = screen.getByRole('spinbutton')
    await user.clear(input)
    await user.type(input, '120')
    fireEvent.submit(input.closest('form')!)

    await waitFor(() => expect(mockSetOverride).toHaveBeenCalledWith(120))
    await waitFor(() => expect(onClose).toHaveBeenCalled())
  })

  it('shows inline error and keeps modal open on API failure', async () => {
    mockSetOverride.mockRejectedValue(new Error('Server error'))
    const onClose = vi.fn()
    const user = userEvent.setup()
    render(
      <ScanIntervalOverrideModal {...defaultProps} onClose={onClose} />,
      { wrapper: makeWrapper() }
    )

    const input = screen.getByRole('spinbutton')
    await user.clear(input)
    await user.type(input, '120')
    fireEvent.submit(input.closest('form')!)

    await waitFor(() => {
      expect(screen.getByText(/Failed to save override/)).toBeInTheDocument()
    })
    expect(onClose).not.toHaveBeenCalled()
  })

  it('calls onClose when Cancel is clicked', async () => {
    const onClose = vi.fn()
    render(
      <ScanIntervalOverrideModal {...defaultProps} onClose={onClose} />,
      { wrapper: makeWrapper() }
    )

    fireEvent.click(screen.getByRole('button', { name: 'Cancel' }))
    expect(onClose).toHaveBeenCalled()
  })
})
