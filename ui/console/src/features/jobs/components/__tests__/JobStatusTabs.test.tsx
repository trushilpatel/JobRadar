import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, cleanup } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { JobStatusTabs } from '../JobStatusTabs'
import { JOB_STATUS } from '@/constants/jobStatus'

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

  it('renders all 6 tabs on desktop', () => {
    setMatchMedia(false)
    render(
      <JobStatusTabs
        tab={JOB_STATUS.NEW}
        onChange={() => {}}
        counts={{ new: 1, saved: 2, applied: 3, interviewing: 4, offer: 5, rejected: 6 }}
      />
    )
    expect(screen.getByTestId('job-status-tabs-desktop')).toBeInTheDocument()
    expect(screen.getAllByRole('tab')).toHaveLength(6)
  })

  it('clicking a tab on desktop calls onChange with the right key', async () => {
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
})
