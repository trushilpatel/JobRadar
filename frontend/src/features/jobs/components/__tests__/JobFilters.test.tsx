import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, cleanup, fireEvent } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { createRef } from 'react'
import type { DateFilter, LocationFilter } from '../../hooks/useJobs'

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
  MobileFilterDrawer: ({ open, onReset }: { open?: boolean; onReset?: () => void }) =>
    open ? (
      <div data-testid="mobile-drawer">
        <button type="button" onClick={onReset}>
          drawer-reset
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

  it('toggles an experience pill ON when not selected', async () => {
    const { onExpChange } = renderFilters({ expFilter: [] })
    await userEvent.click(screen.getByText('0-2 yrs'))
    expect(onExpChange).toHaveBeenCalledWith(['0-2'])
  })

  it('toggles an experience pill OFF when already selected', async () => {
    const { onExpChange } = renderFilters({ expFilter: ['0-2', '2-5'] })
    await userEvent.click(screen.getByText('0-2 yrs'))
    expect(onExpChange).toHaveBeenCalledWith(['2-5'])
  })

  it('renders the "5+ yrs" label for the 5+ range', () => {
    renderFilters()
    expect(screen.getByText('5+ yrs')).toBeInTheDocument()
  })

  it('toggles a date pill ON when inactive', async () => {
    const { onDateChange } = renderFilters({ dateFilter: 'all' })
    await userEvent.click(screen.getByText('24h'))
    expect(onDateChange).toHaveBeenCalledWith('24h')
  })

  it('toggles a date pill OFF (back to "all") when already active', async () => {
    const { onDateChange } = renderFilters({ dateFilter: '7d' })
    await userEvent.click(screen.getByText('7d'))
    expect(onDateChange).toHaveBeenCalledWith('all')
  })

  it('toggles a location pill ON when inactive', async () => {
    const { onLocationChange } = renderFilters({ locationFilter: 'any' })
    await userEvent.click(screen.getByText('Remote'))
    expect(onLocationChange).toHaveBeenCalledWith('remote')
  })

  it('toggles a location pill OFF (back to "any") when already active', async () => {
    const { onLocationChange } = renderFilters({ locationFilter: 'remote' })
    await userEvent.click(screen.getByText('Remote'))
    expect(onLocationChange).toHaveBeenCalledWith('any')
  })

  it('does not render the Reset button when no active filters', () => {
    renderFilters({ hasActiveFilters: false })
    expect(screen.queryByRole('button', { name: /Reset filters/i })).not.toBeInTheDocument()
  })

  it('renders and fires the Reset button when filters are active', async () => {
    const { onReset } = renderFilters({ hasActiveFilters: true })
    await userEvent.click(screen.getByRole('button', { name: /Reset filters/i }))
    expect(onReset).toHaveBeenCalled()
  })

  it('reflects a selected experience pill via aria-pressed', () => {
    renderFilters({ expFilter: ['5+'] })
    expect(screen.getByText('5+ yrs').closest('button')).toHaveAttribute('aria-pressed', 'true')
  })

  it('forwards the ref to the search input', () => {
    const { ref } = renderFilters()
    expect(ref.current).toBeInstanceOf(HTMLInputElement)
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
})
