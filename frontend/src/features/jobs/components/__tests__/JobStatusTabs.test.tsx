import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, cleanup, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { JobStatusTabs } from '../JobStatusTabs'
import { JOB_STATUS } from '@/config/constants/jobStatus'

function setMatchMedia(matches: boolean) {
  Object.defineProperty(window, 'matchMedia', {
    writable: true,
    configurable: true,
    value: vi.fn().mockImplementation((q: string) => ({
      matches,
      media: q,
      addEventListener: vi.fn(),
      removeEventListener: vi.fn(),
      onchange: null,
      dispatchEvent: vi.fn(),
      addListener: vi.fn(),
      removeListener: vi.fn(),
    })),
  })
}

describe('JobStatusTabs', () => {
  beforeEach(() => {
    cleanup()
  })

  it('renders New, Saved, and Tracking on desktop', () => {
    setMatchMedia(false)
    render(
      <JobStatusTabs
        tab={JOB_STATUS.NEW}
        onChange={() => {}}
        counts={{ new: 1, saved: 2, applied: 3, interviewing: 4, offer: 5, rejected: 6 }}
      />
    )
    expect(screen.getByTestId('job-status-tabs-desktop')).toBeInTheDocument()
    const tabs = screen.getAllByRole('tab')
    expect(tabs).toHaveLength(3)
    expect(screen.getByRole('tab', { name: /New/ })).toBeInTheDocument()
    expect(screen.getByRole('tab', { name: /Saved/ })).toBeInTheDocument()
    expect(screen.getByRole('tab', { name: /Tracking/ })).toBeInTheDocument()
  })

  it('clicking New or Saved on desktop calls onChange with the right key', async () => {
    setMatchMedia(false)
    const onChange = vi.fn()
    render(<JobStatusTabs tab={JOB_STATUS.NEW} onChange={onChange} counts={{}} />)
    await userEvent.click(screen.getByRole('tab', { name: /Saved/i }))
    expect(onChange).toHaveBeenCalledWith(JOB_STATUS.SAVED)
  })

  it('renders mobile select trigger below the breakpoint', () => {
    setMatchMedia(true)
    render(<JobStatusTabs tab={JOB_STATUS.NEW} onChange={() => {}} counts={{ new: 5 }} />)
    expect(screen.getByTestId('job-status-tabs-mobile-trigger')).toBeInTheDocument()
    expect(screen.queryByTestId('job-status-tabs-desktop')).not.toBeInTheDocument()
  })

  it('renders count badges when counts provided', () => {
    setMatchMedia(false)
    render(<JobStatusTabs tab={JOB_STATUS.NEW} onChange={() => {}} counts={{ new: 42 }} />)
    expect(screen.getByText('42')).toBeInTheDocument()
  })

  it('renders left and right slots', () => {
    setMatchMedia(false)
    render(
      <JobStatusTabs
        tab={JOB_STATUS.NEW}
        onChange={() => {}}
        counts={{}}
        leftSlot={<span>LEFT</span>}
        rightSlot={<span>RIGHT</span>}
      />
    )
    expect(screen.getByText('LEFT')).toBeInTheDocument()
    expect(screen.getByText('RIGHT')).toBeInTheDocument()
  })

  // ── Tracking sub-menu ────────────────────────────────────────────────────────

  it('clicking the Tracking segment calls onChange with "tracking"', async () => {
    setMatchMedia(false)
    const onChange = vi.fn()
    render(<JobStatusTabs tab={JOB_STATUS.NEW} onChange={onChange} counts={{}} />)
    await userEvent.click(screen.getByRole('tab', { name: /Tracking/ }))
    expect(onChange).toHaveBeenCalledWith('tracking')
  })

  it('shows the combined Tracking count badge from counts.tracking', () => {
    setMatchMedia(false)
    render(
      <JobStatusTabs
        tab={JOB_STATUS.NEW}
        onChange={() => {}}
        counts={{ applied: 3, interviewing: 4, offer: 5, rejected: 6, tracking: 18 }}
      />
    )
    const trackingTab = screen.getByRole('tab', { name: /Tracking/ })
    expect(within(trackingTab).getByText('18')).toBeInTheDocument()
  })

  it('opens a checkbox sub-menu with all 4 stages and their live counts', async () => {
    setMatchMedia(false)
    render(
      <JobStatusTabs
        tab="tracking"
        onChange={() => {}}
        counts={{ applied: 3, interviewing: 4, offer: 5, rejected: 6 }}
        trackingStages={['applied', 'interviewing', 'offer', 'rejected']}
        onTrackingStagesChange={() => {}}
      />
    )
    await userEvent.click(screen.getByRole('tab', { name: /Tracking/ }))
    expect(screen.getByText('Applied')).toBeInTheDocument()
    expect(screen.getByText('Interviewing')).toBeInTheDocument()
    expect(screen.getByText('Offer')).toBeInTheDocument()
    expect(screen.getByText('Rejected')).toBeInTheDocument()
    expect(screen.getByText('3')).toBeInTheDocument()
  })

  it('defaults to all 4 stages checked when trackingStages is not provided', async () => {
    setMatchMedia(false)
    render(<JobStatusTabs tab="tracking" onChange={() => {}} counts={{}} />)
    await userEvent.click(screen.getByRole('tab', { name: /Tracking/ }))
    const checkboxes = screen.getAllByRole('checkbox')
    expect(checkboxes).toHaveLength(4)
    checkboxes.forEach((cb) => expect(cb).toHaveAttribute('data-state', 'checked'))
  })

  it('toggling a stage checkbox calls onTrackingStagesChange with the updated set', async () => {
    setMatchMedia(false)
    const onTrackingStagesChange = vi.fn()
    render(
      <JobStatusTabs
        tab="tracking"
        onChange={() => {}}
        counts={{}}
        trackingStages={['applied', 'interviewing', 'offer', 'rejected']}
        onTrackingStagesChange={onTrackingStagesChange}
      />
    )
    await userEvent.click(screen.getByRole('tab', { name: /Tracking/ }))
    await userEvent.click(screen.getByText('Applied'))
    expect(onTrackingStagesChange).toHaveBeenCalledWith(['interviewing', 'offer', 'rejected'])
  })

  it('checking an unchecked stage adds it back to the set', async () => {
    setMatchMedia(false)
    const onTrackingStagesChange = vi.fn()
    render(
      <JobStatusTabs
        tab="tracking"
        onChange={() => {}}
        counts={{}}
        trackingStages={['applied']}
        onTrackingStagesChange={onTrackingStagesChange}
      />
    )
    await userEvent.click(screen.getByRole('tab', { name: /Tracking/ }))
    await userEvent.click(screen.getByText('Offer'))
    expect(onTrackingStagesChange).toHaveBeenCalledWith(['applied', 'offer'])
  })

  it('includes Tracking as a selectable option on mobile', () => {
    setMatchMedia(true)
    render(<JobStatusTabs tab="tracking" onChange={() => {}} counts={{ tracking: 9 }} />)
    expect(screen.getByTestId('job-status-tabs-mobile-trigger')).toHaveTextContent('Tracking')
  })
})
