import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, fireEvent } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { usePipelineStats } from '@/features/companies/hooks/usePipelineStats'
import { useRetryTier } from '@/features/companies/hooks/useRetryTier'
import { companyQueryKeys } from '@/features/companies/queryKeys'
import { CompaniesHeader } from '../CompaniesHeader'

vi.mock('@/features/companies/hooks/usePipelineStats')
vi.mock('@/features/companies/hooks/useRetryTier')

function renderHeader(autoRefresh: boolean, onToggle = vi.fn()) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: 0, gcTime: 0 } } })
  const invalidateSpy = vi.spyOn(qc, 'invalidateQueries')
  const utils = render(
    <QueryClientProvider client={qc}>
      <CompaniesHeader autoRefresh={autoRefresh} onToggleAutoRefresh={onToggle} />
    </QueryClientProvider>
  )
  return { ...utils, invalidateSpy, onToggle }
}

beforeEach(() => {
  vi.clearAllMocks()
  // PipelineStrip is rendered as a child; keep it in the loading state to avoid noise.
  vi.mocked(usePipelineStats).mockReturnValue({
    data: undefined,
    isLoading: true,
    isError: false,
  } as unknown as ReturnType<typeof usePipelineStats>)
  vi.mocked(useRetryTier).mockReturnValue({
    mutate: vi.fn(),
    isPending: false,
  } as unknown as ReturnType<typeof useRetryTier>)
})

describe('CompaniesHeader', () => {
  it('renders the title and the auto-refresh + refresh controls', () => {
    renderHeader(false)
    expect(screen.getByRole('heading', { name: 'Companies' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Refresh data' })).toBeInTheDocument()
  })

  it('shows the "off" state when autoRefresh is false', () => {
    renderHeader(false)
    const toggle = screen.getByRole('button', { name: 'Auto-refresh: off' })
    expect(toggle).toHaveAttribute('title', expect.stringContaining('click to enable'))
    expect(screen.getByText('Auto')).toBeInTheDocument()
  })

  it('shows the "on" state when autoRefresh is true', () => {
    renderHeader(true)
    const toggle = screen.getByRole('button', { name: 'Auto-refresh: on' })
    expect(toggle).toHaveAttribute('title', expect.stringContaining('click to disable'))
    expect(toggle.className).toContain('text-emerald-400')
    expect(screen.getByText('Live')).toBeInTheDocument()
  })

  it('invokes onToggleAutoRefresh when the toggle is clicked', () => {
    const { onToggle } = renderHeader(false)
    fireEvent.click(screen.getByRole('button', { name: 'Auto-refresh: off' }))
    expect(onToggle).toHaveBeenCalledOnce()
  })

  it('invalidates company + pipeline queries when Refresh is clicked', () => {
    const { invalidateSpy } = renderHeader(false)
    fireEvent.click(screen.getByRole('button', { name: 'Refresh data' }))

    expect(invalidateSpy).toHaveBeenCalledWith({
      queryKey: companyQueryKeys.adminCompaniesBase(),
    })
    expect(invalidateSpy).toHaveBeenCalledWith({
      queryKey: companyQueryKeys.pipelineStats(),
    })
  })
})
