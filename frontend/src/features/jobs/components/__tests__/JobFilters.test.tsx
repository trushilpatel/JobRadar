import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, cleanup, fireEvent, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { createRef } from 'react'
import type { DateFilter, LocationFilter } from '../../hooks/useJobs'
import type { VisaFilter } from '../../hooks/useJobUrlState'

// useIsMobile drives the desktop/mobile branch split. Controlled per-test via
// a mutable flag so we can render both layouts without remounting modules.
let mockIsMobile = false
vi.mock('@/lib/hooks/use-media-query', () => ({
  useIsMobile: () => mockIsMobile,
  useIsDesktop: () => !mockIsMobile,
}))

// MobileFilterDrawer wraps vaul, which crashes in jsdom. The drawer has its own
// test; here we only assert JobFilters wires the right props into it.
vi.mock('../MobileFilterDrawer', () => ({
  MobileFilterDrawer: ({
    open,
    onReset,
    visaFilter,
    onVisaChange,
  }: {
    open?: boolean
    onReset?: () => void
    visaFilter?: string
    onVisaChange?: (v: string) => void
  }) =>
    open ? (
      <div data-testid="mobile-drawer">
        <button type="button" onClick={onReset}>
          drawer-reset
        </button>
        <span data-testid="drawer-visa-filter">{visaFilter}</span>
        <button type="button" onClick={() => onVisaChange?.('true')}>
          drawer-set-visa
        </button>
      </div>
    ) : null,
}))

import { JobFilters } from '../JobFilters'

interface Overrides {
  searchQuery?: string
  companies?: string[]
  selectedCompanies?: string[]
  locationFilter?: LocationFilter
  expFilter?: string[]
  dateFilter?: DateFilter
  visaFilter?: VisaFilter
  hasActiveFilters?: boolean
  mobileOpen?: boolean
  activeFilterCount?: number
}

function renderFilters(overrides: Overrides = {}) {
  const onSearchChange = vi.fn()
  const onCompaniesChange = vi.fn()
  const onLocationChange = vi.fn()
  const onExpChange = vi.fn()
  const onDateChange = vi.fn()
  const onVisaChange = vi.fn()
  const onReset = vi.fn()
  const onMobileOpenChange = vi.fn()
  const ref = createRef<HTMLInputElement>()

  render(
    <JobFilters
      ref={ref}
      searchQuery={overrides.searchQuery ?? ''}
      onSearchChange={onSearchChange}
      companies={overrides.companies ?? ['Acme', 'Stripe']}
      selectedCompanies={overrides.selectedCompanies ?? []}
      onCompaniesChange={onCompaniesChange}
      locationFilter={overrides.locationFilter ?? 'any'}
      onLocationChange={onLocationChange}
      expFilter={overrides.expFilter ?? []}
      onExpChange={onExpChange}
      dateFilter={overrides.dateFilter ?? 'all'}
      onDateChange={onDateChange}
      visaFilter={overrides.visaFilter ?? 'any'}
      onVisaChange={onVisaChange}
      onReset={onReset}
      hasActiveFilters={overrides.hasActiveFilters ?? false}
      mobileOpen={overrides.mobileOpen ?? false}
      onMobileOpenChange={onMobileOpenChange}
      activeFilterCount={overrides.activeFilterCount ?? 0}
    />
  )

  return {
    onSearchChange,
    onCompaniesChange,
    onLocationChange,
    onExpChange,
    onDateChange,
    onVisaChange,
    onReset,
    onMobileOpenChange,
    ref,
  }
}

beforeEach(() => {
  cleanup()
  vi.clearAllMocks()
  mockIsMobile = false
})

describe('JobFilters — desktop', () => {
  it('renders the search input with placeholder', () => {
    renderFilters()
    expect(screen.getByPlaceholderText('Search jobs…')).toBeInTheDocument()
  })

  it('calls onSearchChange when typing in the search field', () => {
    const { onSearchChange } = renderFilters()
    fireEvent.change(screen.getByPlaceholderText('Search jobs…'), { target: { value: 'react' } })
    expect(onSearchChange).toHaveBeenCalledWith('react')
  })

  it('forwards the ref to the search input', () => {
    const { ref } = renderFilters()
    expect(ref.current).toBeInstanceOf(HTMLInputElement)
  })

  // ── Company popover ─────────────────────────────────────────────────────────

  it('renders the Company popover with searchable options and wires onCompaniesChange', async () => {
    const { onCompaniesChange } = renderFilters({ companies: ['Acme', 'Stripe'] })
    await userEvent.click(screen.getByRole('button', { name: 'Company' }))
    expect(screen.getByPlaceholderText('Search…')).toBeInTheDocument()
    await userEvent.click(screen.getByText('Acme'))
    expect(onCompaniesChange).toHaveBeenCalledWith(['Acme'])
  })

  it('shows "No companies" when a Company search matches nothing', async () => {
    renderFilters({ companies: ['Acme', 'Stripe'] })
    await userEvent.click(screen.getByRole('button', { name: 'Company' }))
    await userEvent.type(screen.getByPlaceholderText('Search…'), 'zzz')
    expect(screen.getByText('No companies')).toBeInTheDocument()
  })

  it('shows the selection count on the Company trigger', () => {
    renderFilters({ selectedCompanies: ['Acme', 'Stripe'] })
    expect(screen.getByRole('button', { name: 'Company (2)' })).toBeInTheDocument()
  })

  // ── Location popover ────────────────────────────────────────────────────────

  it('switches locationFilter directly when clicking an option in the Location popover', async () => {
    const { onLocationChange } = renderFilters({ locationFilter: 'any' })
    await userEvent.click(screen.getByRole('button', { name: 'Location' }))
    await userEvent.click(screen.getByText('Remote'))
    expect(onLocationChange).toHaveBeenCalledWith('remote')
  })

  it('clears locationFilter back to "any" when clicking the already-active option', async () => {
    const { onLocationChange } = renderFilters({ locationFilter: 'remote' })
    await userEvent.click(screen.getByRole('button', { name: 'Location' }))
    await userEvent.click(within(screen.getByRole('dialog')).getByText('Remote'))
    expect(onLocationChange).toHaveBeenCalledWith('any')
  })

  // ── Experience popover ──────────────────────────────────────────────────────

  it('passes selections through directly to onExpChange (already multi-select)', async () => {
    const { onExpChange } = renderFilters({ expFilter: [] })
    await userEvent.click(screen.getByRole('button', { name: 'Experience' }))
    await userEvent.click(screen.getByText('0-2 yrs'))
    expect(onExpChange).toHaveBeenCalledWith(['0-2'])
  })

  it('removes a selection when clicking an already-selected experience option', async () => {
    const { onExpChange } = renderFilters({ expFilter: ['0-2', '2-5'] })
    await userEvent.click(screen.getByRole('button', { name: /Experience/ }))
    await userEvent.click(within(screen.getByRole('dialog')).getByText('0-2 yrs'))
    expect(onExpChange).toHaveBeenCalledWith(['2-5'])
  })

  // ── Posted (date) popover ───────────────────────────────────────────────────

  it('switches dateFilter directly when clicking a Posted option', async () => {
    const { onDateChange } = renderFilters({ dateFilter: 'all' })
    await userEvent.click(screen.getByRole('button', { name: 'Posted' }))
    await userEvent.click(screen.getByText('24h'))
    expect(onDateChange).toHaveBeenCalledWith('24h')
  })

  it('clears dateFilter back to "all" when clicking the already-active Posted option', async () => {
    const { onDateChange } = renderFilters({ dateFilter: '7d' })
    await userEvent.click(screen.getByRole('button', { name: 'Posted' }))
    await userEvent.click(within(screen.getByRole('dialog')).getByText('7d'))
    expect(onDateChange).toHaveBeenCalledWith('all')
  })

  // ── Visa popover ────────────────────────────────────────────────────────────

  it('renders the Visa popover and switches visaFilter directly on selection', async () => {
    const { onVisaChange } = renderFilters({ visaFilter: 'any' })
    await userEvent.click(screen.getByRole('button', { name: 'Visa' }))
    await userEvent.click(screen.getByText('Sponsorship offered'))
    expect(onVisaChange).toHaveBeenCalledWith('true')
  })

  it('clears visaFilter back to "any" when clicking the already-active Visa option', async () => {
    const { onVisaChange } = renderFilters({ visaFilter: 'true' })
    await userEvent.click(screen.getByRole('button', { name: 'Visa' }))
    await userEvent.click(within(screen.getByRole('dialog')).getByText('Sponsorship offered'))
    expect(onVisaChange).toHaveBeenCalledWith('any')
  })

  // ── Reset button (unchanged behavior) ──────────────────────────────────────

  it('does not render the Reset button when no active filters', () => {
    renderFilters({ hasActiveFilters: false })
    expect(screen.queryByRole('button', { name: /Reset filters/i })).not.toBeInTheDocument()
  })

  it('renders and fires the Reset button when filters are active', async () => {
    const { onReset } = renderFilters({ hasActiveFilters: true })
    await userEvent.click(screen.getByRole('button', { name: /Reset filters/i }))
    expect(onReset).toHaveBeenCalled()
  })

  // ── Active-filter chip row ──────────────────────────────────────────────────

  it('renders no chip row when no filters are active', () => {
    renderFilters()
    expect(screen.queryByLabelText(/^Remove /)).not.toBeInTheDocument()
    expect(screen.queryByText('Clear all')).not.toBeInTheDocument()
  })

  it('renders one chip per selected company', () => {
    renderFilters({ selectedCompanies: ['Acme', 'Stripe'] })
    expect(screen.getByLabelText('Remove Acme')).toBeInTheDocument()
    expect(screen.getByLabelText('Remove Stripe')).toBeInTheDocument()
  })

  it('removing a company chip removes only that company', async () => {
    const { onCompaniesChange } = renderFilters({ selectedCompanies: ['Acme', 'Stripe'] })
    await userEvent.click(screen.getByLabelText('Remove Acme'))
    expect(onCompaniesChange).toHaveBeenCalledWith(['Stripe'])
  })

  it('renders a location chip with its label and clears it back to "any" on remove', async () => {
    const { onLocationChange } = renderFilters({ locationFilter: 'remote' })
    expect(screen.getByLabelText('Remove Remote')).toBeInTheDocument()
    await userEvent.click(screen.getByLabelText('Remove Remote'))
    expect(onLocationChange).toHaveBeenCalledWith('any')
  })

  it('renders an experience chip per selected range and removes only that one', async () => {
    const { onExpChange } = renderFilters({ expFilter: ['0-2', '2-5'] })
    expect(screen.getByLabelText('Remove 0-2 yrs')).toBeInTheDocument()
    expect(screen.getByLabelText('Remove 2-5 yrs')).toBeInTheDocument()
    await userEvent.click(screen.getByLabelText('Remove 0-2 yrs'))
    expect(onExpChange).toHaveBeenCalledWith(['2-5'])
  })

  it('renders a date chip and clears it back to "all" on remove', async () => {
    const { onDateChange } = renderFilters({ dateFilter: '7d' })
    expect(screen.getByLabelText('Remove 7d')).toBeInTheDocument()
    await userEvent.click(screen.getByLabelText('Remove 7d'))
    expect(onDateChange).toHaveBeenCalledWith('all')
  })

  it('renders a visa chip and clears it back to "any" on remove', async () => {
    const { onVisaChange } = renderFilters({ visaFilter: 'false' })
    expect(screen.getByLabelText('Remove No sponsorship')).toBeInTheDocument()
    await userEvent.click(screen.getByLabelText('Remove No sponsorship'))
    expect(onVisaChange).toHaveBeenCalledWith('any')
  })

  it('does not render "Clear all" when exactly one chip is active', () => {
    renderFilters({ locationFilter: 'remote' })
    expect(screen.queryByText('Clear all')).not.toBeInTheDocument()
  })

  it('renders "Clear all" when 2+ chips are active and it calls onReset', async () => {
    const { onReset } = renderFilters({ locationFilter: 'remote', dateFilter: '7d' })
    const clearAll = screen.getByText('Clear all')
    expect(clearAll).toBeInTheDocument()
    await userEvent.click(clearAll)
    expect(onReset).toHaveBeenCalled()
  })
})

describe('JobFilters — mobile', () => {
  beforeEach(() => {
    mockIsMobile = true
  })

  it('renders the mobile search input', () => {
    renderFilters()
    expect(screen.getByPlaceholderText('Search jobs…')).toBeInTheDocument()
  })

  it('calls onSearchChange from the mobile search field', () => {
    const { onSearchChange } = renderFilters()
    fireEvent.change(screen.getByPlaceholderText('Search jobs…'), { target: { value: 'go' } })
    expect(onSearchChange).toHaveBeenCalledWith('go')
  })

  it('opens the filter drawer when the filters button is tapped', async () => {
    const { onMobileOpenChange } = renderFilters()
    await userEvent.click(screen.getByLabelText('Open filters'))
    expect(onMobileOpenChange).toHaveBeenCalledWith(true)
  })

  it('shows the active-filter count badge on the trigger', () => {
    renderFilters({ activeFilterCount: 3, hasActiveFilters: true })
    expect(screen.getByText('3')).toBeInTheDocument()
  })

  it('hides the count badge when there are no active filters', () => {
    renderFilters({ activeFilterCount: 0 })
    expect(screen.queryByText('0')).not.toBeInTheDocument()
  })

  it('renders the drawer when mobileOpen is true and wires onReset', async () => {
    const { onReset } = renderFilters({ mobileOpen: true })
    const drawer = screen.getByTestId('mobile-drawer')
    expect(drawer).toBeInTheDocument()
    await userEvent.click(screen.getByText('drawer-reset'))
    expect(onReset).toHaveBeenCalled()
  })

  it('does not render the drawer when mobileOpen is false', () => {
    renderFilters({ mobileOpen: false })
    expect(screen.queryByTestId('mobile-drawer')).not.toBeInTheDocument()
  })

  it('threads visaFilter and onVisaChange through to MobileFilterDrawer', async () => {
    const { onVisaChange } = renderFilters({ mobileOpen: true, visaFilter: 'true' })
    const drawer = screen.getByTestId('mobile-drawer')
    expect(within(drawer).getByTestId('drawer-visa-filter')).toHaveTextContent('true')
    await userEvent.click(within(drawer).getByText('drawer-set-visa'))
    expect(onVisaChange).toHaveBeenCalledWith('true')
  })
})
