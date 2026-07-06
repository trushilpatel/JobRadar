import { describe, it, expect, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { ResumePickerDialog } from '../ResumePickerDialog'
import type { ResumeMatchOption } from '../apply-queue-columns'

function makeOption(overrides: Partial<ResumeMatchOption>): ResumeMatchOption {
  return {
    id: 'r1',
    label: 'My Resume',
    matched: 2,
    total: 3,
    matchedKeywords: ['Go', 'PostgreSQL'],
    missingKeywords: ['Kubernetes'],
    hasExtractedText: true,
    ...overrides,
  }
}

const baseProps = {
  open: true,
  onOpenChange: vi.fn(),
  jobTitle: 'Senior Engineer',
  jobHasKeywords: true,
  currentResumeId: null as string | null,
  options: [] as ResumeMatchOption[],
  onConfirm: vi.fn(),
  onClear: vi.fn(),
  isPending: false,
}

describe('ResumePickerDialog', () => {
  it('shows an empty state when there are no resumes', () => {
    render(<ResumePickerDialog {...baseProps} options={[]} />)
    expect(screen.getByText(/no resumes uploaded yet/i)).toBeInTheDocument()
    expect(screen.queryByRole('table')).not.toBeInTheDocument()
  })

  it('shows an orphaned-resume banner when the attached resume is not in options', () => {
    render(
      <ResumePickerDialog
        {...baseProps}
        currentResumeId="deleted-resume"
        options={[makeOption({ id: 'r1' })]}
      />
    )
    expect(screen.getByText(/previously attached resume was deleted/i)).toBeInTheDocument()
  })

  it('does not show the orphaned banner when nothing is attached', () => {
    render(<ResumePickerDialog {...baseProps} currentResumeId={null} options={[makeOption({})]} />)
    expect(screen.queryByText(/previously attached resume was deleted/i)).not.toBeInTheDocument()
  })

  it('disables confirm until a row is selected', () => {
    render(<ResumePickerDialog {...baseProps} currentResumeId={null} options={[makeOption({})]} />)
    expect(screen.getByRole('button', { name: /use this resume/i })).toBeDisabled()
  })

  it('enables confirm and calls onConfirm with the clicked resume id', async () => {
    const user = userEvent.setup()
    const onConfirm = vi.fn()
    render(
      <ResumePickerDialog
        {...baseProps}
        currentResumeId={null}
        options={[makeOption({ id: 'r1', label: 'My Resume' })]}
        onConfirm={onConfirm}
      />
    )
    await user.click(screen.getByText('My Resume'))
    const confirmButton = screen.getByRole('button', { name: /use this resume/i })
    expect(confirmButton).toBeEnabled()
    await user.click(confirmButton)
    expect(onConfirm).toHaveBeenCalledWith('r1')
  })

  it('pre-highlights the currently committed resume, not an auto-pick', async () => {
    const user = userEvent.setup()
    const onConfirm = vi.fn()
    render(
      <ResumePickerDialog
        {...baseProps}
        currentResumeId="r1"
        options={[makeOption({ id: 'r1', label: 'My Resume' })]}
        onConfirm={onConfirm}
      />
    )
    // Already highlighted on open — confirm works without clicking a row first.
    await user.click(screen.getByRole('button', { name: /use this resume/i }))
    expect(onConfirm).toHaveBeenCalledWith('r1')
  })

  it('confirms the selected row on Enter keydown', async () => {
    const user = userEvent.setup()
    const onConfirm = vi.fn()
    render(
      <ResumePickerDialog
        {...baseProps}
        currentResumeId={null}
        options={[makeOption({ id: 'r1', label: 'My Resume' })]}
        onConfirm={onConfirm}
      />
    )
    await user.click(screen.getByText('My Resume'))
    await user.keyboard('{Enter}')
    expect(onConfirm).toHaveBeenCalledWith('r1')
  })

  it('calls onOpenChange(false) when Cancel is clicked', async () => {
    const user = userEvent.setup()
    const onOpenChange = vi.fn()
    render(
      <ResumePickerDialog {...baseProps} options={[makeOption({})]} onOpenChange={onOpenChange} />
    )
    await user.click(screen.getByRole('button', { name: /cancel/i }))
    expect(onOpenChange).toHaveBeenCalledWith(false)
  })

  it('shows Clear selection only when a resume is currently attached', () => {
    const { rerender } = render(
      <ResumePickerDialog {...baseProps} currentResumeId={null} options={[makeOption({})]} />
    )
    expect(screen.queryByRole('button', { name: /clear selection/i })).not.toBeInTheDocument()

    rerender(
      <ResumePickerDialog
        {...baseProps}
        currentResumeId="r1"
        options={[makeOption({ id: 'r1' })]}
      />
    )
    expect(screen.getByRole('button', { name: /clear selection/i })).toBeInTheDocument()
  })

  it('calls onClear when Clear selection is clicked', async () => {
    const user = userEvent.setup()
    const onClear = vi.fn()
    render(
      <ResumePickerDialog
        {...baseProps}
        currentResumeId="r1"
        options={[makeOption({ id: 'r1' })]}
        onClear={onClear}
      />
    )
    await user.click(screen.getByRole('button', { name: /clear selection/i }))
    expect(onClear).toHaveBeenCalled()
  })

  it('disables Cancel, Clear, and Confirm while a mutation is pending, and shows Saving…', () => {
    render(
      <ResumePickerDialog
        {...baseProps}
        currentResumeId="r1"
        options={[makeOption({ id: 'r1' })]}
        isPending
      />
    )
    expect(screen.getByRole('button', { name: /cancel/i })).toBeDisabled()
    expect(screen.getByRole('button', { name: /clear selection/i })).toBeDisabled()
    expect(screen.getByRole('button', { name: /saving/i })).toBeDisabled()
  })

  it('expands a row to show matched and missing keywords', async () => {
    const user = userEvent.setup()
    render(
      <ResumePickerDialog
        {...baseProps}
        options={[
          makeOption({
            id: 'r1',
            label: 'My Resume',
            matchedKeywords: ['Go', 'PostgreSQL'],
            missingKeywords: ['Kubernetes'],
          }),
        ]}
      />
    )
    expect(screen.queryByText(/matched:/)).not.toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: /expand my resume keyword detail/i }))
    expect(screen.getByText(/Go, PostgreSQL/)).toBeInTheDocument()
    expect(screen.getByText(/Kubernetes/)).toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: /collapse my resume keyword detail/i }))
    expect(screen.queryByText(/matched:/)).not.toBeInTheDocument()
  })

  it('truncates very long keyword lists with a "+N more" suffix', async () => {
    const user = userEvent.setup()
    const many = Array.from({ length: 25 }, (_, i) => `Keyword${i}`)
    render(
      <ResumePickerDialog
        {...baseProps}
        options={[
          makeOption({
            id: 'r1',
            matchedKeywords: many,
            missingKeywords: [],
            total: 25,
            matched: 25,
          }),
        ]}
      />
    )
    await user.click(screen.getByRole('button', { name: /expand/i }))
    expect(screen.getByText(/\+5 more/)).toBeInTheDocument()
  })

  it('shows "no keywords to match" instead of a misleading 0/0 when the job has none', () => {
    render(
      <ResumePickerDialog
        {...baseProps}
        jobHasKeywords={false}
        options={[makeOption({ total: 0, matched: 0 })]}
      />
    )
    expect(screen.getByText(/no keywords to match/i)).toBeInTheDocument()
    expect(screen.queryByText('0/0')).not.toBeInTheDocument()
  })

  it('shows "no extracted text" distinctly from a genuine 0/N poor match', () => {
    render(
      <ResumePickerDialog
        {...baseProps}
        options={[makeOption({ hasExtractedText: false, matched: 0 })]}
      />
    )
    expect(screen.getByText(/no extracted text/i)).toBeInTheDocument()
  })

  it('does not allow expanding a row with no breakdown to show', () => {
    render(
      <ResumePickerDialog {...baseProps} options={[makeOption({ hasExtractedText: false })]} />
    )
    expect(screen.getByRole('button', { name: /expand my resume keyword detail/i })).toBeDisabled()
  })
})
