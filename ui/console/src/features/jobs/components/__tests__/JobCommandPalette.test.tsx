import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, cleanup } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { JobCommandPalette } from '../JobCommandPalette'
import { JOB_STATUS } from '@/constants/jobStatus'

beforeEach(() => {
  cleanup()
})

describe('JobCommandPalette', () => {
  it('renders nothing when open is false', () => {
    render(<JobCommandPalette open={false} onOpenChange={() => {}} />)
    expect(screen.queryByPlaceholderText(/type a command/i)).not.toBeInTheDocument()
  })

  it('renders search input when open', () => {
    render(<JobCommandPalette open={true} onOpenChange={() => {}} />)
    expect(screen.getByPlaceholderText(/type a command/i)).toBeInTheDocument()
  })

  it('renders Navigate group with 6 tab entries', () => {
    render(<JobCommandPalette open={true} onOpenChange={() => {}} />)
    expect(screen.getByText('Go to New')).toBeInTheDocument()
    expect(screen.getByText('Go to Saved')).toBeInTheDocument()
    expect(screen.getByText('Go to Applied')).toBeInTheDocument()
    expect(screen.getByText('Go to Interviewing')).toBeInTheDocument()
    expect(screen.getByText('Go to Offer')).toBeInTheDocument()
    expect(screen.getByText('Go to Rejected')).toBeInTheDocument()
  })

  it('selecting Go to Saved calls onGoToTab and closes', async () => {
    const onGoToTab = vi.fn()
    const onOpenChange = vi.fn()
    render(<JobCommandPalette open={true} onOpenChange={onOpenChange} onGoToTab={onGoToTab} />)
    await userEvent.click(screen.getByText('Go to Saved'))
    expect(onGoToTab).toHaveBeenCalledWith(JOB_STATUS.SAVED)
    expect(onOpenChange).toHaveBeenCalledWith(false)
  })

  it('selecting Reset filters calls onResetFilters and closes', async () => {
    const onResetFilters = vi.fn()
    const onOpenChange = vi.fn()
    render(
      <JobCommandPalette open={true} onOpenChange={onOpenChange} onResetFilters={onResetFilters} />
    )
    await userEvent.click(screen.getByText('Reset filters'))
    expect(onResetFilters).toHaveBeenCalled()
    expect(onOpenChange).toHaveBeenCalledWith(false)
  })

  it('renders recent jobs when provided', () => {
    const jobs = [
      {
        id: 'j1',
        title: 'Senior Engineer',
        company: { name: 'Anthropic' },
      } as unknown as import('@/types').Job,
    ]
    render(<JobCommandPalette open={true} onOpenChange={() => {}} recentJobs={jobs} />)
    expect(screen.getByText('Jobs in view')).toBeInTheDocument()
    expect(screen.getByText('Senior Engineer')).toBeInTheDocument()
  })
})
