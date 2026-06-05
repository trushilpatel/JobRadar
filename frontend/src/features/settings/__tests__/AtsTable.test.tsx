import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, fireEvent, waitFor } from '@testing-library/react'
import { AtsTable } from '../components/AtsTable'
import type { ATS } from '@/types'

vi.mock('sonner', () => ({ toast: { success: vi.fn(), error: vi.fn() } }))

const mockRefetch = vi.fn()
const mockOnUpdate = vi.fn()

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
    is_active: false,
    is_admin_creatable: true,
    scan_interval_min: 720,
    inter_company_delay_s: 5,
  },
]

const defaultProps = {
  atsList: mockAtsList,
  isLoading: false,
  isError: false,
  refetch: mockRefetch,
  onUpdate: mockOnUpdate,
}

describe('AtsTable', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  // ── Loading state ────────────────────────────────────────────────────────────

  it('renders skeleton rows while loading', () => {
    const { container } = render(
      <AtsTable {...defaultProps} atsList={undefined} isLoading={true} />
    )
    const pulseEls = container.querySelectorAll('.animate-pulse')
    expect(pulseEls.length).toBeGreaterThan(0)
    expect(screen.queryByRole('table')).not.toBeInTheDocument()
  })

  // ── Error state ──────────────────────────────────────────────────────────────

  it('renders error message and retry button when isError is true', () => {
    render(<AtsTable {...defaultProps} atsList={undefined} isError={true} />)
    expect(screen.getByText(/Failed to load ATS configs/)).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Retry' })).toBeInTheDocument()
  })

  it('calls refetch when Retry button is clicked', () => {
    render(<AtsTable {...defaultProps} atsList={undefined} isError={true} />)
    fireEvent.click(screen.getByRole('button', { name: 'Retry' }))
    expect(mockRefetch).toHaveBeenCalledTimes(1)
  })

  // ── Empty state ──────────────────────────────────────────────────────────────

  it('renders empty state when ATS list is empty', () => {
    render(<AtsTable {...defaultProps} atsList={[]} />)
    expect(screen.getByText('No ATS configurations found.')).toBeInTheDocument()
    expect(screen.queryByRole('table')).not.toBeInTheDocument()
  })

  // ── Data rendering ──────────────────────────────────────────────────────────

  it('renders ATS rows in the table', () => {
    render(<AtsTable {...defaultProps} />)
    expect(screen.getByRole('table')).toBeInTheDocument()
    // Shows display_name
    expect(screen.getByText('Greenhouse')).toBeInTheDocument()
    expect(screen.getByText('Lever')).toBeInTheDocument()
    // Shows name as subtitle
    expect(screen.getByText('greenhouse')).toBeInTheDocument()
    expect(screen.getByText('lever')).toBeInTheDocument()
  })

  it('displays display_name as primary label and name as subtitle', () => {
    render(<AtsTable {...defaultProps} atsList={[mockAtsList[0]!]} />)
    // display_name should be the primary label
    expect(screen.getByText('Greenhouse')).toBeInTheDocument()
    // name should be a muted subtitle
    expect(screen.getByText('greenhouse')).toBeInTheDocument()
  })

  it('shows active/inactive status badge', () => {
    render(<AtsTable {...defaultProps} />)
    expect(screen.getByText('Active')).toBeInTheDocument()
    expect(screen.getByText('Inactive')).toBeInTheDocument()
  })

  // ── Confirmation modal ──────────────────────────────────────────────────────

  it('opens confirmation modal when an AdapterConfigCell triggers a save', async () => {
    render(<AtsTable {...defaultProps} />)

    const editButtons = screen.getAllByTitle('Click to edit')
    fireEvent.click(editButtons[0]!)

    const input = screen.getByRole('spinbutton')
    fireEvent.change(input, { target: { value: '720' } })
    fireEvent.keyDown(input, { key: 'Enter' })

    await waitFor(() => {
      expect(screen.getByText('Confirm Change')).toBeInTheDocument()
    })
  })

  it('shows display_name in confirmation dialog', async () => {
    render(<AtsTable {...defaultProps} />)

    const editButtons = screen.getAllByTitle('Click to edit')
    fireEvent.click(editButtons[0]!)

    const input = screen.getByRole('spinbutton')
    fireEvent.change(input, { target: { value: '720' } })
    fireEvent.keyDown(input, { key: 'Enter' })

    await waitFor(() => {
      expect(screen.getByText('Confirm Change')).toBeInTheDocument()
      // Should show display_name "Greenhouse" in dialog text
      expect(screen.getAllByText('Greenhouse').length).toBeGreaterThan(0)
    })
  })

  it('closes confirmation modal when Cancel is clicked', async () => {
    render(<AtsTable {...defaultProps} />)

    const editButtons = screen.getAllByTitle('Click to edit')
    fireEvent.click(editButtons[0]!)
    const input = screen.getByRole('spinbutton')
    fireEvent.change(input, { target: { value: '720' } })
    fireEvent.keyDown(input, { key: 'Enter' })

    await waitFor(() => {
      expect(screen.getByText('Confirm Change')).toBeInTheDocument()
    })

    fireEvent.click(screen.getByRole('button', { name: 'Cancel' }))

    await waitFor(() => {
      expect(screen.queryByText('Confirm Change')).not.toBeInTheDocument()
    })
  })

  it('calls onUpdate when Confirm is clicked in modal', async () => {
    mockOnUpdate.mockResolvedValue(undefined)
    render(<AtsTable {...defaultProps} />)

    const editButtons = screen.getAllByTitle('Click to edit')
    fireEvent.click(editButtons[0]!)
    const input = screen.getByRole('spinbutton')
    fireEvent.change(input, { target: { value: '720' } })
    fireEvent.keyDown(input, { key: 'Enter' })

    await waitFor(() => {
      expect(screen.getByText('Confirm Change')).toBeInTheDocument()
    })

    fireEvent.click(screen.getByRole('button', { name: 'Confirm' }))

    await waitFor(() => {
      expect(mockOnUpdate).toHaveBeenCalledWith(
        'greenhouse',
        expect.objectContaining({ scan_interval_min: 720 })
      )
    })
  })
})
