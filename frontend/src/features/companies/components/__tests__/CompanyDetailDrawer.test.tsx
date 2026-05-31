import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, fireEvent } from '@testing-library/react'
import type { Company } from '@/types'
import { useCompanyScanStats } from '@/features/companies/hooks/useCompanyScanStats'
import { useCompanyAnalytics } from '@/features/companies/hooks/useCompanyAnalytics'
import { CompanyDetailDrawer } from '../CompanyDetailDrawer'

vi.mock('@/features/companies/hooks/useCompanyScanStats')
vi.mock('@/features/companies/hooks/useCompanyAnalytics')

const company: Company = {
  id: 'stripe',
  name: 'Stripe',
  url: 'https://stripe.com',
  adapter_type: 'greenhouse',
  active: true,
  created_at: '2024-01-01T00:00:00Z',
  last_scan_at: null,
  last_scan_status: null,
}

const baseProps = {
  company,
  onClose: vi.fn(),
  onScan: vi.fn(),
  onEdit: vi.fn(),
  onDelete: vi.fn(),
}

beforeEach(() => {
  vi.clearAllMocks()
  vi.mocked(useCompanyScanStats).mockReturnValue({
    data: { total_new: 5, new_today: 1, new_this_month: 3, recent_runs: [] },
    isLoading: false,
  } as unknown as ReturnType<typeof useCompanyScanStats>)
  vi.mocked(useCompanyAnalytics).mockReturnValue({
    data: undefined,
    isError: false,
  } as unknown as ReturnType<typeof useCompanyAnalytics>)
})

describe('CompanyDetailDrawer — View new jobs navigation', () => {
  it('calls onViewJobs with the company name when the button is clicked', () => {
    const onViewJobs = vi.fn()
    render(<CompanyDetailDrawer {...baseProps} onViewJobs={onViewJobs} />)

    fireEvent.click(screen.getByRole('button', { name: /view 5 new jobs/i }))

    expect(onViewJobs).toHaveBeenCalledOnce()
    expect(onViewJobs).toHaveBeenCalledWith('Stripe')
  })

  it('does not throw when onViewJobs is not provided', () => {
    render(<CompanyDetailDrawer {...baseProps} />)

    // Button is only shown when total_new > 0; clicking it should be a no-op
    expect(() =>
      fireEvent.click(screen.getByRole('button', { name: /view 5 new jobs/i }))
    ).not.toThrow()
  })
})
