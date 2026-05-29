import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, fireEvent } from '@testing-library/react'
import type { Company } from '@/types'
import { useCompanyScanStats } from '@/features/companies/hooks/useCompanyScanStats'
import { useCompanyAnalytics } from '@/features/companies/hooks/useCompanyAnalytics'
import { CompanyDetailDrawer } from '../CompanyDetailDrawer'

const mockNavigate = vi.fn()

vi.mock('react-router-dom', async (importOriginal) => {
  const actual = await importOriginal<typeof import('react-router-dom')>()
  return { ...actual, useNavigate: () => mockNavigate }
})

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
  it('navigates to the jobs page with the company filter in the URL query string', () => {
    render(<CompanyDetailDrawer {...baseProps} />)

    fireEvent.click(screen.getByRole('button', { name: /view 5 new jobs/i }))

    expect(mockNavigate).toHaveBeenCalledTimes(1)
    const target = mockNavigate.mock.calls[0][0]
    expect(typeof target).toBe('string')
    const params = new URL(`http://x${target as string}`).searchParams
    expect(params.getAll('companies')).toEqual(['Stripe'])
  })

  it('does NOT pass the filter via router state (the bug that rendered an empty jobs page)', () => {
    render(<CompanyDetailDrawer {...baseProps} />)

    fireEvent.click(screen.getByRole('button', { name: /view 5 new jobs/i }))

    // Regression guard: the jobs page reads filters only from the URL, never
    // from location.state. A state payload here is silently dropped → empty page.
    expect(mockNavigate.mock.calls[0][1]).toBeUndefined()
  })
})
