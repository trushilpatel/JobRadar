import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { CompaniesFilterBar } from '../CompaniesFilterBar'
import { SCAN_STATUS_FILTER, ACTIVE_FILTER } from '../../constants'

type Props = Parameters<typeof CompaniesFilterBar>[0]

function makeProps(overrides: Partial<Props> = {}): Props {
  return {
    search: '',
    onSearchChange: vi.fn(),
    adapters: [],
    onAdaptersChange: vi.fn(),
    adapterOptions: ['greenhouse', 'lever'],
    onAddAdapter: vi.fn(),
    activeFilter: ACTIVE_FILTER.ALL,
    onActiveFilterChange: vi.fn(),
    scanStatus: SCAN_STATUS_FILTER.ALL,
    onScanStatusChange: vi.fn(),
    bulkToggling: false,
    bulkAllActive: false,
    onBulkToggle: vi.fn(),
    runningAll: false,
    onRunAll: vi.fn(),
    hasActiveFilters: false,
    onReset: vi.fn(),
    onAddClick: vi.fn(),
    ...overrides,
  }
}

beforeEach(() => {
  vi.clearAllMocks()
})

describe('CompaniesFilterBar — bulk toggle', () => {
  it('opens a confirm dialog when deactivating all and fires onBulkToggle only after confirming', async () => {
    const user = userEvent.setup()
    const onBulkToggle = vi.fn()
    render(<CompaniesFilterBar {...makeProps({ bulkAllActive: true, onBulkToggle })} />)

    await user.click(screen.getByRole('button', { name: /deactivate all/i }))

    // Dialog title is distinct from the trigger button label.
    expect(screen.getByText('Deactivate all companies?')).toBeInTheDocument()
    // No toggle yet — confirmation pending.
    expect(onBulkToggle).not.toHaveBeenCalled()

    // The confirm action button inside the dialog.
    const confirmButton = screen
      .getAllByRole('button', { name: /deactivate all/i })
      .find((b) => b.className.includes('bg-red-600'))
    expect(confirmButton).toBeDefined()
    await user.click(confirmButton as HTMLElement)

    expect(onBulkToggle).toHaveBeenCalledTimes(1)
  })

  it('does NOT fire onBulkToggle when the deactivate dialog is cancelled', async () => {
    const user = userEvent.setup()
    const onBulkToggle = vi.fn()
    render(<CompaniesFilterBar {...makeProps({ bulkAllActive: true, onBulkToggle })} />)

    await user.click(screen.getByRole('button', { name: /deactivate all/i }))
    expect(screen.getByText('Deactivate all companies?')).toBeInTheDocument()

    await user.click(screen.getByRole('button', { name: /cancel/i }))

    expect(onBulkToggle).not.toHaveBeenCalled()
  })

  it('fires onBulkToggle immediately with no dialog when activating all', async () => {
    const user = userEvent.setup()
    const onBulkToggle = vi.fn()
    render(<CompaniesFilterBar {...makeProps({ bulkAllActive: false, onBulkToggle })} />)

    await user.click(screen.getByRole('button', { name: /activate all/i }))

    expect(onBulkToggle).toHaveBeenCalledTimes(1)
    expect(screen.queryByText('Deactivate all companies?')).not.toBeInTheDocument()
  })
})

describe('CompaniesFilterBar — run all', () => {
  it('opens a confirm dialog and fires onRunAll only on confirm', async () => {
    const user = userEvent.setup()
    const onRunAll = vi.fn()
    render(<CompaniesFilterBar {...makeProps({ onRunAll })} />)

    await user.click(screen.getByRole('button', { name: /run all/i }))

    expect(screen.getByText('Queue all companies for scan?')).toBeInTheDocument()
    expect(onRunAll).not.toHaveBeenCalled()

    await user.click(screen.getByRole('button', { name: /queue all/i }))

    expect(onRunAll).toHaveBeenCalledTimes(1)
  })

  it('does NOT fire onRunAll when the run-all dialog is cancelled', async () => {
    const user = userEvent.setup()
    const onRunAll = vi.fn()
    render(<CompaniesFilterBar {...makeProps({ onRunAll })} />)

    await user.click(screen.getByRole('button', { name: /run all/i }))
    expect(screen.getByText('Queue all companies for scan?')).toBeInTheDocument()

    await user.click(screen.getByRole('button', { name: /cancel/i }))

    expect(onRunAll).not.toHaveBeenCalled()
  })
})

describe('CompaniesFilterBar — scan status pills', () => {
  it('calls onScanStatusChange with the pill value when inactive', async () => {
    const user = userEvent.setup()
    const onScanStatusChange = vi.fn()
    render(<CompaniesFilterBar {...makeProps({ onScanStatusChange })} />)

    await user.click(screen.getByRole('button', { name: /errors/i }))

    expect(onScanStatusChange).toHaveBeenCalledWith(SCAN_STATUS_FILTER.ERROR)
  })

  it('calls onScanStatusChange with ALL when the active pill is clicked again', async () => {
    const user = userEvent.setup()
    const onScanStatusChange = vi.fn()
    render(
      <CompaniesFilterBar
        {...makeProps({ scanStatus: SCAN_STATUS_FILTER.ERROR, onScanStatusChange })}
      />
    )

    await user.click(screen.getByRole('button', { name: /errors/i }))

    expect(onScanStatusChange).toHaveBeenCalledWith(SCAN_STATUS_FILTER.ALL)
  })
})

describe('CompaniesFilterBar — search', () => {
  it('clears the search via the clear button', async () => {
    const user = userEvent.setup()
    const onSearchChange = vi.fn()
    render(<CompaniesFilterBar {...makeProps({ search: 'acme', onSearchChange })} />)

    // The clear button is the only icon-only button next to the search input;
    // it renders only while `search` is non-empty. Locate it by its X-icon SVG
    // sibling within the search wrapper.
    const searchInput = screen.getByPlaceholderText('Search companies…')
    const clearButton = searchInput.parentElement?.querySelector('button')
    expect(clearButton).toBeTruthy()
    await user.click(clearButton as HTMLElement)

    expect(onSearchChange).toHaveBeenCalledWith('')
  })
})

describe('CompaniesFilterBar — add adapter', () => {
  it('fires onAddAdapter from the inline add-adapter input', async () => {
    const user = userEvent.setup()
    const onAddAdapter = vi.fn()
    render(<CompaniesFilterBar {...makeProps({ onAddAdapter })} />)

    // The "Add new adapter…" input is always visible (inline in the filter bar).
    await user.type(screen.getByPlaceholderText('Add new adapter…'), 'customats')
    await user.click(screen.getByRole('button', { name: '+ Add' }))

    expect(onAddAdapter).toHaveBeenCalledWith('customats')
  })
})

describe('CompaniesFilterBar — reset control', () => {
  it('is non-interactive when there are no active filters', () => {
    const onReset = vi.fn()
    render(<CompaniesFilterBar {...makeProps({ hasActiveFilters: false, onReset })} />)

    const resetButton = screen.getByRole('button', { name: /reset/i })
    // Hidden + pointer-events-none — clicks must not reach the handler.
    expect(resetButton.className).toContain('pointer-events-none')
    expect(resetButton.className).toContain('opacity-0')
  })

  it('fires onReset when active filters exist', async () => {
    const user = userEvent.setup()
    const onReset = vi.fn()
    render(<CompaniesFilterBar {...makeProps({ hasActiveFilters: true, onReset })} />)

    const resetButton = screen.getByRole('button', { name: /reset/i })
    expect(resetButton.className).toContain('opacity-100')
    await user.click(resetButton)

    expect(onReset).toHaveBeenCalledTimes(1)
  })
})

describe('CompaniesFilterBar — disabled', () => {
  it('disables the bulk-toggle button while bulkToggling', () => {
    render(<CompaniesFilterBar {...makeProps({ bulkToggling: true })} />)
    expect(screen.getByRole('button', { name: /activate all/i })).toBeDisabled()
  })

  it('disables the run-all button while runningAll', () => {
    render(<CompaniesFilterBar {...makeProps({ runningAll: true })} />)
    expect(screen.getByRole('button', { name: /run all/i })).toBeDisabled()
  })
})
