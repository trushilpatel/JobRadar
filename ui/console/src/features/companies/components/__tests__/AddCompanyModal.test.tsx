import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, fireEvent, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import * as adminApi from '@/api/adminCompanies'
import { AddCompanyModal } from '../AddCompanyModal'
import type { Company } from '@/types'

vi.mock('@/api/adminCompanies')

// Radix Dialog uses a Portal — make sure it renders into the document body
// which jsdom handles automatically, but we need a container for queries.

const baseProps = {
  open: true,
  onClose: vi.fn(),
  onCreated: vi.fn(),
  onUpdated: vi.fn(),
}

const mockAdminCompanyResponse = {
  id: 'acme',
  name: 'Acme Corp',
  website: 'https://boards-api.greenhouse.io/v1/boards/acme/jobs',
  created_at: '2024-01-01T00:00:00Z',
  scan_settings: null,
}

describe('AddCompanyModal', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  // ── Rendering ───────────────────────────────────────────────────────────────

  it('renders the dialog when open={true}', () => {
    render(<AddCompanyModal {...baseProps} />)
    expect(screen.getByRole('dialog')).toBeInTheDocument()
    // Title text (DialogTitle) — use heading role to be specific
    expect(screen.getByRole('heading', { name: 'Add Company' })).toBeInTheDocument()
  })

  it('does not render dialog content when open={false}', () => {
    render(<AddCompanyModal {...baseProps} open={false} />)
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
  })

  it('shows the Name field', () => {
    render(<AddCompanyModal {...baseProps} />)
    expect(screen.getByPlaceholderText('Stripe')).toBeInTheDocument()
  })

  it('shows the adapter select', () => {
    render(<AddCompanyModal {...baseProps} />)
    // The select element is rendered for adapter choices
    const selects = screen.getAllByRole('combobox')
    expect(selects.length).toBeGreaterThan(0)
  })

  it('shows the identifier input', () => {
    render(<AddCompanyModal {...baseProps} />)
    expect(screen.getByPlaceholderText('stripe')).toBeInTheDocument()
  })

  // ── Edit mode ───────────────────────────────────────────────────────────────

  it('shows "Edit Company" title in edit mode', () => {
    const company: Company = {
      id: 'acme',
      name: 'Acme Corp',
      url: 'https://acme.com',
      adapter_type: 'greenhouse',
      active: true,
      created_at: '2024-01-01T00:00:00Z',
      last_scan_at: null,
      last_scan_status: null,
    }
    render(<AddCompanyModal {...baseProps} initialData={company} />)
    expect(screen.getByText('Edit Company')).toBeInTheDocument()
  })

  // ── Validation ──────────────────────────────────────────────────────────────

  it('shows validation error when submitting with empty name', async () => {
    render(<AddCompanyModal {...baseProps} />)

    // Fill in the identifier but leave name empty
    const idInput = screen.getByPlaceholderText('stripe')
    fireEvent.change(idInput, { target: { value: 'acme' } })

    // Clear the name field (it's empty by default already but let's be explicit)
    const nameInput = screen.getByPlaceholderText('Stripe')
    fireEvent.change(nameInput, { target: { value: '' } })

    const submitBtn = screen.getByRole('button', { name: /add company/i })
    fireEvent.click(submitBtn)

    await waitFor(() => {
      // Error message about required fields
      expect(
        screen.getByText(/identifier.*name.*url are required|identifier and name are required/i)
      ).toBeInTheDocument()
    })
  })

  it('shows validation error when identifier is empty', async () => {
    render(<AddCompanyModal {...baseProps} />)

    // Fill name but no id
    const nameInput = screen.getByPlaceholderText('Stripe')
    fireEvent.change(nameInput, { target: { value: 'Acme Corp' } })

    const submitBtn = screen.getByRole('button', { name: /add company/i })
    fireEvent.click(submitBtn)

    await waitFor(() => {
      expect(screen.getByText(/required/i)).toBeInTheDocument()
    })
  })

  // ── Cancel ──────────────────────────────────────────────────────────────────

  it('calls onClose when Cancel is clicked', async () => {
    const onClose = vi.fn()
    render(<AddCompanyModal {...baseProps} onClose={onClose} />)

    const cancelBtn = screen.getByRole('button', { name: /cancel/i })
    fireEvent.click(cancelBtn)

    expect(onClose).toHaveBeenCalled()
  })

  // ── Successful submit ───────────────────────────────────────────────────────

  it('calls onCreated and closes on successful create', async () => {
    const user = userEvent.setup()
    const onCreated = vi.fn()
    const onClose = vi.fn()
    vi.mocked(adminApi.createAdminCompany).mockResolvedValue(mockAdminCompanyResponse)

    render(<AddCompanyModal {...baseProps} onCreated={onCreated} onClose={onClose} />)

    // Fill identifier (lowercase, no spaces)
    const idInput = screen.getByPlaceholderText('stripe')
    await user.type(idInput, 'acme')

    // Fill name
    const nameInput = screen.getByPlaceholderText('Stripe')
    await user.type(nameInput, 'Acme Corp')

    // For greenhouse adapter, URL is auto-filled from identifier
    // The URL field should be auto-filled; just submit
    const submitBtn = screen.getByRole('button', { name: /add company/i })
    await user.click(submitBtn)

    await waitFor(() => {
      expect(adminApi.createAdminCompany).toHaveBeenCalled()
      expect(onCreated).toHaveBeenCalled()
      expect(onClose).toHaveBeenCalled()
    })
  })

  it('calls onUpdated and closes on successful update', async () => {
    const user = userEvent.setup()
    const onUpdated = vi.fn()
    const onClose = vi.fn()
    vi.mocked(adminApi.updateAdminCompany).mockResolvedValue(undefined)
    vi.mocked(adminApi.updateAdminCompanyScanSettings).mockResolvedValue(undefined)

    const company: Company = {
      id: 'acme',
      name: 'Acme Corp',
      url: 'https://acme.com/jobs',
      adapter_type: 'greenhouse',
      active: true,
      created_at: '2024-01-01T00:00:00Z',
      last_scan_at: null,
      last_scan_status: null,
    }

    render(
      <AddCompanyModal
        {...baseProps}
        initialData={company}
        onUpdated={onUpdated}
        onClose={onClose}
      />
    )

    // Change the name
    const nameInput = screen.getByDisplayValue('Acme Corp')
    await user.clear(nameInput)
    await user.type(nameInput, 'Acme Updated')

    const submitBtn = screen.getByRole('button', { name: /save changes/i })
    await user.click(submitBtn)

    await waitFor(() => {
      expect(adminApi.updateAdminCompany).toHaveBeenCalled()
      expect(onUpdated).toHaveBeenCalled()
      expect(onClose).toHaveBeenCalled()
    })
  })

  // ── URL auto-fill ───────────────────────────────────────────────────────────

  it('auto-fills URL when adapter has a template and identifier is typed', async () => {
    const user = userEvent.setup()
    render(<AddCompanyModal {...baseProps} />)

    const idInput = screen.getByPlaceholderText('stripe')
    await user.type(idInput, 'mycompany')

    // Greenhouse template: https://boards-api.greenhouse.io/v1/boards/{id}/jobs
    const urlInput = screen.getByPlaceholderText(/auto-filled|https/i)
    expect((urlInput as HTMLInputElement).value).toContain('mycompany')
  })

  // ── Error display ───────────────────────────────────────────────────────────

  it('shows global error when API call fails with a non-validation error', async () => {
    const user = userEvent.setup()
    vi.mocked(adminApi.createAdminCompany).mockRejectedValue(new Error('Internal server error'))

    render(<AddCompanyModal {...baseProps} />)

    const idInput = screen.getByPlaceholderText('stripe')
    await user.type(idInput, 'acme')

    const nameInput = screen.getByPlaceholderText('Stripe')
    await user.type(nameInput, 'Acme Corp')

    const submitBtn = screen.getByRole('button', { name: /add company/i })
    await user.click(submitBtn)

    await waitFor(() => {
      expect(screen.getByText(/internal server error/i)).toBeInTheDocument()
    })
  })
})
