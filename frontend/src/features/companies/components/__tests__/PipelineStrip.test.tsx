import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import type { PipelineStats } from '@/types'
import { usePipelineStats } from '@/features/companies/hooks/usePipelineStats'
import { useRetryTier } from '@/features/companies/hooks/useRetryTier'
import { PipelineStrip } from '../PipelineStrip'

vi.mock('@/features/companies/hooks/usePipelineStats')
vi.mock('@/features/companies/hooks/useRetryTier')

const stats: PipelineStats = {
  total_companies: 10,
  active_companies: 8,
  queued: 2,
  crawling: 1,
  awaiting_filter: 3,
  filter_retries: { '1m': 1, '10m': 0, '1h': 0, dlq: 0 },
  awaiting_enrichment: 4,
  enricher_retries: { '1m': 0, '10m': 0, '1h': 0, dlq: 2 },
  new_jobs_24h: 12,
  errors_24h: 0,
}

function mockStats(over: Partial<ReturnType<typeof usePipelineStats>> = {}) {
  vi.mocked(usePipelineStats).mockReturnValue({
    data: stats,
    isLoading: false,
    isError: false,
    ...over,
  } as unknown as ReturnType<typeof usePipelineStats>)
}

beforeEach(() => {
  vi.clearAllMocks()
  vi.mocked(useRetryTier).mockReturnValue({
    mutate: vi.fn(),
    isPending: false,
  } as unknown as ReturnType<typeof useRetryTier>)
  mockStats()
})

describe('PipelineStrip', () => {
  it('renders a skeleton while loading', () => {
    mockStats({ data: undefined, isLoading: true })
    const { container } = render(<PipelineStrip />)
    expect(container.querySelector('.animate-pulse')).toBeInTheDocument()
  })

  it('renders an error message when the query errors', () => {
    mockStats({ data: undefined, isLoading: false, isError: true })
    render(<PipelineStrip />)
    expect(screen.getByText(/pipeline stats unavailable/i)).toBeInTheDocument()
  })

  it('renders an error message when data is missing without an error flag', () => {
    mockStats({ data: undefined, isLoading: false, isError: false })
    render(<PipelineStrip />)
    expect(screen.getByText(/pipeline stats unavailable/i)).toBeInTheDocument()
  })

  it('shows the Total stage when total_companies differs from active_companies', () => {
    mockStats()
    render(<PipelineStrip />)
    expect(screen.getByText('Total')).toBeInTheDocument()
    expect(screen.getByText('10')).toBeInTheDocument()
    expect(screen.getByText('Active')).toBeInTheDocument()
  })

  it('hides the Total stage when total equals active', () => {
    mockStats({ data: { ...stats, total_companies: 8, active_companies: 8 } })
    render(<PipelineStrip />)
    expect(screen.queryByText('Total')).not.toBeInTheDocument()
    expect(screen.getByText('Active')).toBeInTheDocument()
  })

  it('renders all named pipeline stages', () => {
    mockStats()
    render(<PipelineStrip />)
    expect(screen.getByText('Queued')).toBeInTheDocument()
    expect(screen.getByText('Crawling')).toBeInTheDocument()
    // "Filter" appears in both "Awaiting Filter" and "Filter Retrying".
    expect(screen.getAllByText('Filter').length).toBeGreaterThanOrEqual(2)
    expect(screen.getByText('Enricher')).toBeInTheDocument()
    // Multi-word labels are split into stacked word spans.
    expect(screen.getAllByText('Awaiting').length).toBeGreaterThanOrEqual(2)
  })
})
