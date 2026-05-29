import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, fireEvent, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import * as adminApi from '@/api/adminCompanies'
import { ApiValidationError } from '@/lib/api-error'
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

  // ── ApiValidationError on a field that isn't in the form ────────────────────
  // Regression: when the backend rejected `slug` (a field the modal doesn't
  // expose), the submit failed silently and the modal looked frozen. The
  // handler should now lift unmapped field errors into the global banner.

  it('surfaces backend ApiValidationError on an unmapped field as the global error', async () => {
    const user = userEvent.setup()
    vi.mocked(adminApi.createAdminCompany).mockRejectedValue(
      new ApiValidationError([{ field: 'slug', code: 'REQUIRED', message: 'slug is required' }])
    )

    render(<AddCompanyModal {...baseProps} />)
    await user.type(screen.getByPlaceholderText('stripe'), 'acme')
    await user.type(screen.getByPlaceholderText('Stripe'), 'Acme Corp')
    await user.click(screen.getByRole('button', { name: /add company/i }))

    await waitFor(() => {
      expect(screen.getByText(/slug.*required/i)).toBeInTheDocument()
    })
  })

  it('keeps field-level errors on mapped fields without duplicating into the global banner', async () => {
    const user = userEvent.setup()
    vi.mocked(adminApi.createAdminCompany).mockRejectedValue(
      new ApiValidationError([{ field: 'name', code: 'REQUIRED', message: 'name is required' }])
    )

    render(<AddCompanyModal {...baseProps} />)
    await user.type(screen.getByPlaceholderText('stripe'), 'acme')
    await user.type(screen.getByPlaceholderText('Stripe'), 'Acme Corp')
    await user.click(screen.getByRole('button', { name: /add company/i }))

    await waitFor(() => {
      expect(adminApi.createAdminCompany).toHaveBeenCalled()
    })
    // The inline field error shows; the global banner stays empty.
    expect(screen.getByText('name is required')).toBeInTheDocument()
  })

  // ── gem adapter URL template + payload ──────────────────────────────────────

  it('auto-fills the careers URL for the gem adapter (jobs.gem.com/{slug})', async () => {
    const user = userEvent.setup()
    render(<AddCompanyModal {...baseProps} />)

    // Switch adapter to gem
    const adapterSelect = screen.getAllByRole('combobox')[0] as HTMLSelectElement
    fireEvent.change(adapterSelect, { target: { value: 'gem' } })

    // Typing the slug should auto-fill the URL via the gem template.
    await user.type(screen.getByPlaceholderText('stripe'), 'inception')

    const urlInput = screen.getByPlaceholderText(/auto-filled|https/i) as HTMLInputElement
    expect(urlInput.value).toBe('https://jobs.gem.com/inception')
  })

  it('submits the gem adapter create payload correctly', async () => {
    const user = userEvent.setup()
    const onCreated = vi.fn()
    vi.mocked(adminApi.createAdminCompany).mockResolvedValue({
      id: 'inception',
      name: 'Inception',
      website: 'https://jobs.gem.com/inception',
      created_at: '2026-05-14T00:00:00Z',
      scan_settings: null,
    })

    render(<AddCompanyModal {...baseProps} onCreated={onCreated} />)
    fireEvent.change(screen.getAllByRole('combobox')[0]!, { target: { value: 'gem' } })
    await user.type(screen.getByPlaceholderText('stripe'), 'inception')
    await user.type(screen.getByPlaceholderText('Stripe'), 'Inception')
    await user.click(screen.getByRole('button', { name: /add company/i }))

    await waitFor(() => {
      expect(adminApi.createAdminCompany).toHaveBeenCalledWith({
        id: 'inception',
        name: 'Inception',
        website: 'https://jobs.gem.com/inception',
        scan_settings: { adapter_type: 'gem', active: true },
      })
    })
  })

  // ── Workday adapter (host + site, no slug-based URL template) ───────────────

  function selectAdapter(value: string) {
    const adapterSelect = screen.getAllByRole('combobox')[0] as HTMLSelectElement
    fireEvent.change(adapterSelect, { target: { value } })
  }

  it('shows host + site inputs and hides slug-based hint when workday is selected', () => {
    render(<AddCompanyModal {...baseProps} />)

    // slug input still exists (id is needed even for workday), but Workday host + site appear
    selectAdapter('workday')

    expect(screen.getByLabelText('Workday host')).toBeInTheDocument()
    expect(screen.getByLabelText('Workday site path')).toBeInTheDocument()

    // careers URL field becomes read-only with the Workday placeholder
    const urlInput = screen.getByPlaceholderText(
      /Auto-filled — enter host \+ site above/i
    ) as HTMLInputElement
    expect(urlInput).toBeInTheDocument()
    expect(urlInput.readOnly).toBe(true)
  })

  it('shows inline error for invalid Workday hosts and clears it when valid', () => {
    render(<AddCompanyModal {...baseProps} />)
    selectAdapter('workday')

    const hostInput = screen.getByLabelText('Workday host') as HTMLInputElement

    // Plainly wrong domain
    fireEvent.change(hostInput, { target: { value: 'foo.com' } })
    expect(screen.getByText(/Host must match/i)).toBeInTheDocument()

    // Missing wd<N> segment
    fireEvent.change(hostInput, { target: { value: 'nvidia.myworkdayjobs.com' } })
    expect(screen.getByText(/Host must match/i)).toBeInTheDocument()

    // Uppercase rejected
    fireEvent.change(hostInput, { target: { value: 'NVIDIA.wd5.myworkdayjobs.com' } })
    expect(screen.getByText(/Host must match/i)).toBeInTheDocument()

    // Valid — error clears
    fireEvent.change(hostInput, { target: { value: 'nvidia.wd5.myworkdayjobs.com' } })
    expect(screen.queryByText(/Host must match/i)).not.toBeInTheDocument()
  })

  it('shows inline error for empty or slash-containing site, accepts alphanumeric+underscore', () => {
    render(<AddCompanyModal {...baseProps} />)
    selectAdapter('workday')

    const siteInput = screen.getByLabelText('Workday site path') as HTMLInputElement

    // Typing then clearing should leave the empty-state error
    fireEvent.change(siteInput, { target: { value: 'temp' } })
    fireEvent.change(siteInput, { target: { value: '' } })
    expect(screen.getByText(/Site path is required/i)).toBeInTheDocument()

    fireEvent.change(siteInput, { target: { value: 'has/slash' } })
    expect(screen.getByText(/must not contain "\/"/i)).toBeInTheDocument()

    fireEvent.change(siteInput, { target: { value: 'NVIDIA_External_Career_Site' } })
    expect(screen.queryByText(/Site path is required/i)).not.toBeInTheDocument()
    expect(screen.queryByText(/must not contain "\/"/i)).not.toBeInTheDocument()
  })

  it('auto-fills careers URL as https://<host>/<site> when both Workday fields are valid', () => {
    render(<AddCompanyModal {...baseProps} />)
    selectAdapter('workday')

    fireEvent.change(screen.getByLabelText('Workday host'), {
      target: { value: 'nvidia.wd5.myworkdayjobs.com' },
    })
    fireEvent.change(screen.getByLabelText('Workday site path'), {
      target: { value: 'NVIDIAExternalCareerSite' },
    })

    const urlInput = screen.getByPlaceholderText(
      /Auto-filled — enter host \+ site above/i
    ) as HTMLInputElement
    expect(urlInput.value).toBe('https://nvidia.wd5.myworkdayjobs.com/NVIDIAExternalCareerSite')
  })

  it('submits the Workday create payload with adapter_config={host, site}', async () => {
    const user = userEvent.setup()
    const onCreated = vi.fn()
    vi.mocked(adminApi.createAdminCompany).mockResolvedValue({
      id: 'nvidia',
      name: 'NVIDIA',
      website: 'https://nvidia.wd5.myworkdayjobs.com/NVIDIAExternalCareerSite',
      created_at: '2026-05-15T00:00:00Z',
      scan_settings: null,
    })

    render(<AddCompanyModal {...baseProps} onCreated={onCreated} />)
    selectAdapter('workday')

    await user.type(screen.getByPlaceholderText('stripe'), 'nvidia')
    await user.type(screen.getByPlaceholderText('Stripe'), 'NVIDIA')
    fireEvent.change(screen.getByLabelText('Workday host'), {
      target: { value: 'nvidia.wd5.myworkdayjobs.com' },
    })
    fireEvent.change(screen.getByLabelText('Workday site path'), {
      target: { value: 'NVIDIAExternalCareerSite' },
    })

    await user.click(screen.getByRole('button', { name: /add company/i }))

    await waitFor(() => {
      expect(adminApi.createAdminCompany).toHaveBeenCalledWith({
        id: 'nvidia',
        name: 'NVIDIA',
        website: 'https://nvidia.wd5.myworkdayjobs.com/NVIDIAExternalCareerSite',
        scan_settings: {
          adapter_type: 'workday',
          active: true,
          adapter_config: {
            host: 'nvidia.wd5.myworkdayjobs.com',
            site: 'NVIDIAExternalCareerSite',
          },
        },
      })
    })
  })

  it('switching adapter back to greenhouse hides workday fields and restores slug auto-fill', async () => {
    const user = userEvent.setup()
    render(<AddCompanyModal {...baseProps} />)

    selectAdapter('workday')
    expect(screen.getByLabelText('Workday host')).toBeInTheDocument()

    selectAdapter('greenhouse')
    expect(screen.queryByLabelText('Workday host')).not.toBeInTheDocument()
    expect(screen.queryByLabelText('Workday site path')).not.toBeInTheDocument()

    // Slug auto-fill works again
    await user.type(screen.getByPlaceholderText('stripe'), 'acme')
    const urlInput = screen.getByPlaceholderText(/auto-filled|https/i) as HTMLInputElement
    expect(urlInput.value).toContain('acme')
    expect(urlInput.value).toContain('greenhouse')
  })

  it('blocks Workday submit when host or site is invalid', async () => {
    const user = userEvent.setup()
    render(<AddCompanyModal {...baseProps} />)
    selectAdapter('workday')

    await user.type(screen.getByPlaceholderText('stripe'), 'nvidia')
    await user.type(screen.getByPlaceholderText('Stripe'), 'NVIDIA')
    fireEvent.change(screen.getByLabelText('Workday host'), {
      target: { value: 'invalid-host' },
    })
    // site left empty

    await user.click(screen.getByRole('button', { name: /add company/i }))

    expect(screen.getByText(/Host must match/i)).toBeInTheDocument()
    expect(screen.getByText(/Site path is required/i)).toBeInTheDocument()
    expect(adminApi.createAdminCompany).not.toHaveBeenCalled()
  })

  // ── Dialog a11y description (Radix warning fix) ─────────────────────────────

  it('renders a Dialog.Description for screen readers', () => {
    render(<AddCompanyModal {...baseProps} />)
    const dialog = screen.getByRole('dialog')
    // Radix wires aria-describedby on DialogContent to the Description id.
    expect(dialog).toHaveAttribute('aria-describedby')
  })
})
