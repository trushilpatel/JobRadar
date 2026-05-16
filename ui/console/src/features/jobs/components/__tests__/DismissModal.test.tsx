import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, fireEvent } from '@testing-library/react'
import { DismissModal } from '../DismissModal'

describe('DismissModal', () => {
  const onConfirm = vi.fn()
  const onCancel = vi.fn()

  beforeEach(() => {
    vi.clearAllMocks()
  })

  function renderModal(props: Partial<React.ComponentProps<typeof DismissModal>> = {}) {
    return render(
      <DismissModal
        open
        jobTitle="Software Engineer at Acme"
        onConfirm={onConfirm}
        onCancel={onCancel}
        {...props}
      />
    )
  }

  describe('single mode (count=1 default)', () => {
    it('renders the title and job subtitle', () => {
      renderModal()
      expect(screen.getByText('Dismiss job')).toBeInTheDocument()
      expect(screen.getByText('"Software Engineer at Acme"')).toBeInTheDocument()
    })

    it('renders all six dismiss reason options', () => {
      renderModal()
      expect(screen.getByText('Not relevant')).toBeInTheDocument()
      expect(screen.getByText('Too junior / senior')).toBeInTheDocument()
      expect(screen.getByText('Bad location')).toBeInTheDocument()
      expect(screen.getByText('Compensation too low')).toBeInTheDocument()
      expect(screen.getByText('Company not a fit')).toBeInTheDocument()
      expect(screen.getByText('Other')).toBeInTheDocument()
    })

    it('renders Cancel and Dismiss buttons', () => {
      renderModal()
      expect(screen.getByText('Cancel')).toBeInTheDocument()
      expect(screen.getByText('Dismiss')).toBeInTheDocument()
    })

    it('Dismiss button text in single mode is "Dismiss"', () => {
      renderModal()
      expect(screen.getByRole('button', { name: 'Dismiss' })).toBeInTheDocument()
    })
  })

  describe('open=false', () => {
    it('does not render the dialog content', () => {
      renderModal({ open: false })
      expect(screen.queryByText('Dismiss job')).not.toBeInTheDocument()
    })
  })

  describe('bulk mode (count > 1)', () => {
    it('shows the bulk count in the title', () => {
      renderModal({ count: 5 })
      expect(screen.getByText('Dismiss 5 jobs?')).toBeInTheDocument()
    })

    it('shows the bulk-apply subtitle and hides the job title', () => {
      renderModal({ count: 5 })
      expect(screen.getByText('Apply this reason to all 5 selected.')).toBeInTheDocument()
      expect(screen.queryByText('"Software Engineer at Acme"')).not.toBeInTheDocument()
    })

    it('Dismiss button text reflects the count', () => {
      renderModal({ count: 5 })
      expect(screen.getByRole('button', { name: 'Dismiss 5' })).toBeInTheDocument()
    })
  })

  describe('a11y / radiogroup', () => {
    it('wraps options in a role="radiogroup" with role="radio" + aria-checked', () => {
      renderModal()
      const group = screen.getByRole('radiogroup')
      expect(group).toBeInTheDocument()
      const radios = screen.getAllByRole('radio')
      expect(radios).toHaveLength(6)
      // First option is selected by default.
      expect(radios[0]).toHaveAttribute('aria-checked', 'true')
      expect(radios[1]).toHaveAttribute('aria-checked', 'false')
    })

    it('arrow keys move selection across radios', () => {
      renderModal()
      const group = screen.getByRole('radiogroup')
      fireEvent.keyDown(group, { key: 'ArrowDown' })
      const radios = screen.getAllByRole('radio')
      expect(radios[1]).toHaveAttribute('aria-checked', 'true')
      expect(radios[0]).toHaveAttribute('aria-checked', 'false')
      fireEvent.keyDown(group, { key: 'ArrowUp' })
      expect(radios[0]).toHaveAttribute('aria-checked', 'true')
    })

    it('Home / End jump to first / last radio', () => {
      renderModal()
      const group = screen.getByRole('radiogroup')
      fireEvent.keyDown(group, { key: 'End' })
      const radios = screen.getAllByRole('radio')
      expect(radios[5]).toHaveAttribute('aria-checked', 'true')
      fireEvent.keyDown(group, { key: 'Home' })
      expect(radios[0]).toHaveAttribute('aria-checked', 'true')
    })

    it('note textarea has an accessible label', () => {
      renderModal()
      expect(screen.getByLabelText('Note (optional)')).toBeInTheDocument()
    })
  })

  describe('onConfirm signature: { reason, note }', () => {
    it('emits { reason, note } with the auto-filled label when the note is empty', () => {
      renderModal()
      // First option selected by default → label "Not relevant", value "user_dismissed"
      fireEvent.click(screen.getByText('Dismiss'))
      expect(onConfirm).toHaveBeenCalledTimes(1)
      expect(onConfirm).toHaveBeenCalledWith({
        reason: 'user_dismissed',
        note: 'Not relevant',
      })
    })

    it('emits the visible label as note for "Bad location" when note is empty', () => {
      renderModal()
      fireEvent.click(screen.getByText('Bad location'))
      fireEvent.click(screen.getByText('Dismiss'))
      expect(onConfirm).toHaveBeenCalledWith({
        reason: 'user_not_relevant',
        note: 'Bad location',
      })
    })

    it('user-typed note overrides the auto-filled label', () => {
      renderModal()
      fireEvent.click(screen.getByText('Bad location'))
      fireEvent.change(screen.getByLabelText('Note (optional)'), {
        target: { value: 'remote required' },
      })
      fireEvent.click(screen.getByText('Dismiss'))
      expect(onConfirm).toHaveBeenCalledWith({
        reason: 'user_not_relevant',
        note: 'remote required',
      })
    })

    it('truncates the note at 500 characters', () => {
      renderModal()
      const textarea = screen.getByLabelText('Note (optional)') as HTMLTextAreaElement
      fireEvent.change(textarea, { target: { value: 'x'.repeat(600) } })
      expect(textarea.value.length).toBe(500)
    })

    it('a note of pure whitespace falls back to the auto-fill label', () => {
      renderModal()
      fireEvent.change(screen.getByLabelText('Note (optional)'), {
        target: { value: '   ' },
      })
      fireEvent.click(screen.getByText('Dismiss'))
      expect(onConfirm).toHaveBeenCalledWith({
        reason: 'user_dismissed',
        note: 'Not relevant',
      })
    })
  })

  describe('Cancel', () => {
    it('Cancel button calls onCancel and not onConfirm', () => {
      renderModal()
      fireEvent.click(screen.getByText('Cancel'))
      expect(onCancel).toHaveBeenCalledTimes(1)
      expect(onConfirm).not.toHaveBeenCalled()
    })

    it('Dismiss button does not call onCancel', () => {
      renderModal()
      fireEvent.click(screen.getByText('Dismiss'))
      expect(onCancel).not.toHaveBeenCalled()
    })
  })

  describe('visual selection', () => {
    it('first option is selected on initial render', () => {
      renderModal()
      const notRelevantBtn = screen.getByText('Not relevant').closest('button')
      expect(notRelevantBtn?.className).toContain('bg-violet-600/20')
    })

    it('clicking another option updates the visual selection', () => {
      renderModal()
      fireEvent.click(screen.getByText('Other'))
      const otherBtn = screen.getByText('Other').closest('button')
      expect(otherBtn?.className).toContain('bg-violet-600/20')
    })
  })
})
