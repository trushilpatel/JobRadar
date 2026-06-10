import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, fireEvent, within } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { MemoryRouter } from 'react-router-dom'
import { TooltipProvider } from '@/components/ui/tooltip'
import type { Company } from '@/types'
// Mock the hook source modules; reference the mocked fns through the feature
// barrel (pages may only import from @/features/X, never internals).
import {
  useCompanies,
  useCompanyActions,
  useCompanyScanStats,
  useCompanyAnalytics,
} from '@/features/companies'
import { CompaniesPage } from '@/pages/CompaniesPage'

vi.mock('@/features/companies/hooks/useCompanies')
vi.mock('@/features/companies/hooks/useCompanyActions')
// usePipelineStats + useRetryTier feed the child PipelineStrip/RetryStage; mock
// them to no-op fetches. We register the mocks but drive defaults in the factory.
vi.mock('@/features/companies/hooks/usePipelineStats', () => ({
  usePipelineStats: () => ({ data: undefined, isLoading: true, isError: false }),
}))
vi.mock('@/features/companies/hooks/useRetryTier', () => ({
  useRetryTier: () => ({ mutate: vi.fn(), isPending: false }),
}))
vi.mock('@/features/companies/hooks/useCompanyScanStats')
vi.mock('@/features/companies/hooks/useCompanyAnalytics')
vi.mock('@/features/ats', () => ({
  useAts: vi.fn(() => ({ data: undefined, isLoading: true, isError: false, refetch: vi.fn() })),
  useAtsActions: vi.fn(),
  atsQueryKeys: { list: () => ['ats'], detail: (name: string) => ['ats', name] },
  getAts: vi.fn(),
  updateAts: vi.fn(),
}))

const navigate = vi.fn()
vi.mock('react-router-dom', async () => {
  const actual = await vi.importActual<typeof import('react-router-dom')>('react-router-dom')
  return { ...actual, useNavigate: () => navigate }
})

const companyA: Company = {
  id: 'stripe',
  name: 'Stripe',
  url: 'https://stripe.com',
  adapter_type: 'greenhouse',
  active: true,
  created_at: '2024-01-01T00:00:00Z',
  last_scan_at: null,
  last_scan_status: null,
}
const companyB: Company = { ...companyA, id: 'lever', name: 'Lever', active: false }

type CompaniesReturn = ReturnType<typeof useCompanies>
type ActionsReturn = ReturnType<typeof useCompanyActions>

const actionFns = {
  scan: vi.fn(),
  remove: vi.fn(),
  toggleActive: vi.fn(),
  scanAll: vi.fn(),
  bulkToggleActive: vi.fn(),
  handleCompanyAdded: vi.fn(),
  handleCompanyUpdated: vi.fn(),
}

function mockCompanies(over: Partial<CompaniesReturn> = {}) {
  vi.mocked(useCompanies).mockReturnValue({
    companies: [companyA, companyB],
    total: 2,
    totalActive: 1,
    isLoading: false,
    isFetching: false,
    refetch: vi.fn(),
    search: '',
    handleSearchChange: vi.fn(),
    adapters: [],
    setAdapters: vi.fn(),
    activeFilter: 'all',
    setActiveFilter: vi.fn(),
    scanStatus: 'all',
    setScanStatus: vi.fn(),
    sorting: [],
    handleSortingChange: vi.fn(),
    limit: 20,
    setLimit: vi.fn(),
    nextCursor: null,
    hasPrev: false,
    goToNextPage: vi.fn(),
    goToPrevPage: vi.fn(),
    goToFirstPage: vi.fn(),
    page: 1,
    itemsOnPage: 2,
    itemsBeforePage: 0,
    hasActiveFilters: false,
    resetFilters: vi.fn(),
    autoRefresh: false,
    toggleAutoRefresh: vi.fn(),
    ...over,
  } as unknown as CompaniesReturn)
}

function mockActions(over: Partial<ActionsReturn> = {}) {
  vi.mocked(useCompanyActions).mockReturnValue({
    scanning: new Set<string>(),
    bulkToggling: false,
    runningAll: false,
    ...actionFns,
    ...over,
  } as unknown as ActionsReturn)
}

function renderPage() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: 0, gcTime: 0 } } })
  return render(
    <TooltipProvider>
      <MemoryRouter>
        <QueryClientProvider client={qc}>
          <CompaniesPage />
        </QueryClientProvider>
      </MemoryRouter>
    </TooltipProvider>
  )
}

beforeEach(() => {
  vi.clearAllMocks()
  mockCompanies()
  mockActions()
  vi.mocked(useCompanyScanStats).mockReturnValue({
    data: { total_new: 0, new_today: 0, new_this_month: 0, recent_runs: [] },
    isLoading: false,
  } as unknown as ReturnType<typeof useCompanyScanStats>)
  vi.mocked(useCompanyAnalytics).mockReturnValue({
    data: undefined,
    isError: false,
  } as unknown as ReturnType<typeof useCompanyAnalytics>)
})

describe('CompaniesPage', () => {
  it('renders the header and the company rows', () => {
    renderPage()
    expect(screen.getByRole('heading', { name: 'Companies' })).toBeInTheDocument()
    expect(screen.getByText('Stripe')).toBeInTheDocument()
    expect(screen.getByText('Lever')).toBeInTheDocument()
  })

  it('opens the detail drawer when a row is clicked', () => {
    renderPage()
    fireEvent.click(screen.getByText('Stripe'))
    expect(screen.getByRole('button', { name: 'Close drawer' })).toBeInTheDocument()
  })

  it('navigates to the jobs page when "View jobs" fires from the drawer', () => {
    vi.mocked(useCompanyScanStats).mockReturnValue({
      data: { total_new: 3, new_today: 1, new_this_month: 2, recent_runs: [] },
      isLoading: false,
    } as unknown as ReturnType<typeof useCompanyScanStats>)
    renderPage()
    fireEvent.click(screen.getByText('Stripe'))
    fireEvent.click(screen.getByRole('button', { name: /view 3 new jobs/i }))
    expect(navigate).toHaveBeenCalledOnce()
    expect(String(navigate.mock.calls[0]![0])).toContain('Stripe')
  })

  it('opens the delete dialog from a row action and removes on confirm', () => {
    renderPage()
    fireEvent.click(screen.getByRole('button', { name: 'Delete Stripe' }))
    const dialog = screen.getByRole('alertdialog')
    fireEvent.click(within(dialog).getByRole('button', { name: 'Delete' }))
    expect(actionFns.remove).toHaveBeenCalledWith('stripe')
  })

  it('cancels the delete dialog without removing', () => {
    renderPage()
    fireEvent.click(screen.getByRole('button', { name: 'Delete Lever' }))
    const dialog = screen.getByRole('alertdialog')
    fireEvent.click(within(dialog).getByRole('button', { name: 'Cancel' }))
    expect(actionFns.remove).not.toHaveBeenCalled()
  })

  it('deleting from the drawer closes it and removes the company', () => {
    renderPage()
    fireEvent.click(screen.getByText('Stripe'))
    // Scope to the drawer panel (the aside containing the Close button).
    const drawer = screen.getByRole('button', { name: 'Close drawer' }).closest('aside')!
    fireEvent.click(within(drawer).getByTitle('Delete'))
    // Confirm in the drawer's own delete dialog.
    fireEvent.click(screen.getByRole('button', { name: 'Delete' }))
    expect(actionFns.remove).toHaveBeenCalledWith('stripe')
  })

  it('shows the filtered empty message when filters are active and no rows', () => {
    mockCompanies({ companies: [], itemsOnPage: 0, total: 0, hasActiveFilters: true })
    renderPage()
    expect(screen.getByText('No companies match the current filters')).toBeInTheDocument()
  })

  it('shows the default empty message when there are no rows and no filters', () => {
    mockCompanies({ companies: [], itemsOnPage: 0, total: 0, hasActiveFilters: false })
    renderPage()
    expect(screen.getByText('No companies found')).toBeInTheDocument()
  })
})
