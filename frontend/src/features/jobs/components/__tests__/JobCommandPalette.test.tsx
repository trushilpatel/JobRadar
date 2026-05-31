import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, cleanup } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { JobCommandPalette } from '../JobCommandPalette'
import { JOB_COMMANDS } from '../../commands/jobCommands'
import { JOB_STATUS } from '@/config/constants/jobStatus'

beforeEach(() => {
  cleanup()
})

describe('JobCommandPalette', () => {
  it('renders nothing when open is false', () => {
    render(<JobCommandPalette open={false} onOpenChange={() => {}} />)
    expect(screen.queryByPlaceholderText(/type a command/i)).not.toBeInTheDocument()
  })

  it('renders search input when open', () => {
    render(<JobCommandPalette open onOpenChange={() => {}} />)
    expect(screen.getByPlaceholderText(/type a command/i)).toBeInTheDocument()
  })

  it('renders all six tab commands using their registry labels', () => {
    render(<JobCommandPalette open onOpenChange={() => {}} />)
    expect(screen.getByText(JOB_COMMANDS['jobs.tab.go-new'].label)).toBeInTheDocument()
    expect(screen.getByText(JOB_COMMANDS['jobs.tab.go-saved'].label)).toBeInTheDocument()
    expect(screen.getByText(JOB_COMMANDS['jobs.tab.go-applied'].label)).toBeInTheDocument()
    expect(screen.getByText(JOB_COMMANDS['jobs.tab.go-interviewing'].label)).toBeInTheDocument()
    expect(screen.getByText(JOB_COMMANDS['jobs.tab.go-offer'].label)).toBeInTheDocument()
    expect(screen.getByText(JOB_COMMANDS['jobs.tab.go-rejected'].label)).toBeInTheDocument()
  })

  it('renders the registry-derived utility command labels (not hardcoded)', () => {
    render(<JobCommandPalette open onOpenChange={() => {}} />)
    expect(screen.getByText(JOB_COMMANDS['jobs.nav.reset-filters'].label)).toBeInTheDocument()
    expect(screen.getByText(JOB_COMMANDS['jobs.nav.open-help'].label)).toBeInTheDocument()
  })

  it('does not render the old hardcoded "Show keyboard shortcuts" label', () => {
    render(<JobCommandPalette open onOpenChange={() => {}} />)
    expect(screen.queryByText('Show keyboard shortcuts')).not.toBeInTheDocument()
  })

  it('selecting Go to Saved calls onGoToTab and closes', async () => {
    const onGoToTab = vi.fn()
    const onOpenChange = vi.fn()
    render(<JobCommandPalette open onOpenChange={onOpenChange} onGoToTab={onGoToTab} />)
    await userEvent.click(screen.getByText(JOB_COMMANDS['jobs.tab.go-saved'].label))
    expect(onGoToTab).toHaveBeenCalledWith(JOB_STATUS.SAVED)
    expect(onOpenChange).toHaveBeenCalledWith(false)
  })

  it('selecting Reset filters calls onResetFilters and closes', async () => {
    const onResetFilters = vi.fn()
    const onOpenChange = vi.fn()
    render(<JobCommandPalette open onOpenChange={onOpenChange} onResetFilters={onResetFilters} />)
    await userEvent.click(screen.getByText(JOB_COMMANDS['jobs.nav.reset-filters'].label))
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
    render(<JobCommandPalette open onOpenChange={() => {}} recentJobs={jobs} />)
    expect(screen.getByText('Jobs in view')).toBeInTheDocument()
    expect(screen.getByText('Senior Engineer')).toBeInTheDocument()
  })
})
