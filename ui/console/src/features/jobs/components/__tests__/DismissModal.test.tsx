import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, fireEvent } from '@testing-library/react'
import { DismissModal } from '../DismissModal'

// ── Tests ─────────────────────────────────────────────────────────────────────

describe('DismissModal', () => {
  const onConfirm = vi.fn()
  const onCancel = vi.fn()

  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('renders when open=true', () => {
    render(
      <DismissModal
        open={true}
        jobTitle="Software Engineer at Acme"
        onConfirm={onConfirm}
        onCancel={onCancel}
      />
    )
    expect(screen.getByText('Dismiss job')).toBeInTheDocument()
  })

  it('shows the job title', () => {
    render(
      <DismissModal
        open={true}
        jobTitle="Software Engineer at Acme"
        onConfirm={onConfirm}
        onCancel={onCancel}
      />
    )
    expect(screen.getByText('"Software Engineer at Acme"')).toBeInTheDocument()
  })

  it('does not render when open=false', () => {
    render(
      <DismissModal
        open={false}
        jobTitle="Software Engineer at Acme"
        onConfirm={onConfirm}
        onCancel={onCancel}
      />
    )
    expect(screen.queryByText('Dismiss job')).not.toBeInTheDocument()
  })

  it('renders all dismiss reason options', () => {
    render(
      <DismissModal
        open={true}
        jobTitle="Software Engineer"
        onConfirm={onConfirm}
        onCancel={onCancel}
      />
    )
    expect(screen.getByText('Not relevant')).toBeInTheDocument()
    expect(screen.getByText('Company not a fit')).toBeInTheDocument()
    expect(screen.getByText('Other')).toBeInTheDocument()
  })

  it('renders Cancel button', () => {
    render(
      <DismissModal
        open={true}
        jobTitle="Software Engineer"
        onConfirm={onConfirm}
        onCancel={onCancel}
      />
    )
    expect(screen.getByText('Cancel')).toBeInTheDocument()
  })

  it('renders Dismiss button', () => {
    render(
      <DismissModal
        open={true}
        jobTitle="Software Engineer"
        onConfirm={onConfirm}
        onCancel={onCancel}
      />
    )
    expect(screen.getByText('Dismiss')).toBeInTheDocument()
  })

  it('calls onCancel when Cancel button is clicked', () => {
    render(
      <DismissModal
        open={true}
        jobTitle="Software Engineer"
        onConfirm={onConfirm}
        onCancel={onCancel}
      />
    )
    fireEvent.click(screen.getByText('Cancel'))
    expect(onCancel).toHaveBeenCalledTimes(1)
  })

  it('calls onConfirm with a reason value when Dismiss is clicked', () => {
    render(
      <DismissModal
        open={true}
        jobTitle="Software Engineer"
        onConfirm={onConfirm}
        onCancel={onCancel}
      />
    )
    // First option ("Not relevant") is selected by default (index 0)
    fireEvent.click(screen.getByText('Dismiss'))
    expect(onConfirm).toHaveBeenCalledTimes(1)
    // The first option has value 'user_dismissed'
    expect(onConfirm).toHaveBeenCalledWith('user_dismissed')
  })

  it('calls onConfirm with the selected reason when a different option is clicked', () => {
    render(
      <DismissModal
        open={true}
        jobTitle="Software Engineer"
        onConfirm={onConfirm}
        onCancel={onCancel}
      />
    )
    // Click "Company not a fit" (5th option, value='user_dismissed')
    fireEvent.click(screen.getByText('Company not a fit'))
    fireEvent.click(screen.getByText('Dismiss'))
    expect(onConfirm).toHaveBeenCalledWith('user_dismissed')
  })

  it('calls onConfirm with user_not_relevant when "Too junior / senior" is selected', () => {
    render(
      <DismissModal
        open={true}
        jobTitle="Software Engineer"
        onConfirm={onConfirm}
        onCancel={onCancel}
      />
    )
    fireEvent.click(screen.getByText('Too junior / senior'))
    fireEvent.click(screen.getByText('Dismiss'))
    expect(onConfirm).toHaveBeenCalledWith('user_not_relevant')
  })

  it('does not call onConfirm when Cancel is clicked', () => {
    render(
      <DismissModal
        open={true}
        jobTitle="Software Engineer"
        onConfirm={onConfirm}
        onCancel={onCancel}
      />
    )
    fireEvent.click(screen.getByText('Cancel'))
    expect(onConfirm).not.toHaveBeenCalled()
  })

  it('does not call onCancel when Dismiss is clicked', () => {
    render(
      <DismissModal
        open={true}
        jobTitle="Software Engineer"
        onConfirm={onConfirm}
        onCancel={onCancel}
      />
    )
    fireEvent.click(screen.getByText('Dismiss'))
    expect(onCancel).not.toHaveBeenCalled()
  })

  it('first reason option is visually selected by default', () => {
    render(
      <DismissModal
        open={true}
        jobTitle="Software Engineer"
        onConfirm={onConfirm}
        onCancel={onCancel}
      />
    )
    // The first option button should have the selected styling class
    const notRelevantBtn = screen.getByText('Not relevant').closest('button')
    expect(notRelevantBtn?.className).toContain('bg-violet-600/20')
  })

  it('selecting a reason updates the visual selection', () => {
    render(
      <DismissModal
        open={true}
        jobTitle="Software Engineer"
        onConfirm={onConfirm}
        onCancel={onCancel}
      />
    )
    // Click "Other"
    fireEvent.click(screen.getByText('Other'))
    const otherBtn = screen.getByText('Other').closest('button')
    expect(otherBtn?.className).toContain('bg-violet-600/20')
  })
})
