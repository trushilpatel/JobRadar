import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, fireEvent, waitFor } from '@testing-library/react'
import { ResumeCard } from '../ResumeCard'
import * as actionsHook from '../../hooks/useResumeActions'

vi.mock('../../hooks/useResumeActions')

const sampleResume = {
  id: 'r1',
  user_id: 'u1',
  label: 'My resume',
  original_filename: 'resume.pdf',
  content_type: 'application/pdf' as const,
  size_bytes: 2048,
  created_at: '2026-07-03T00:00:00Z',
  extracted_text: 'Backend engineer with Go experience.',
}

const sampleBlob = new Blob(['%PDF-1.4'], { type: 'application/pdf' })

function mockActions(overrides: Partial<ReturnType<typeof actionsHook.useResumeActions>> = {}) {
  vi.mocked(actionsHook.useResumeActions).mockReturnValue({
    upload: { mutateAsync: vi.fn(), isPending: false } as unknown as ReturnType<
      typeof actionsHook.useResumeActions
    >['upload'],
    download: {
      mutateAsync: vi.fn().mockResolvedValue(sampleBlob),
    } as unknown as ReturnType<typeof actionsHook.useResumeActions>['download'],
    rename: { mutateAsync: vi.fn() } as unknown as ReturnType<
      typeof actionsHook.useResumeActions
    >['rename'],
    remove: { mutate: vi.fn() } as unknown as ReturnType<
      typeof actionsHook.useResumeActions
    >['remove'],
    ...overrides,
  })
}

describe('ResumeCard', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    // jsdom has no createObjectURL/revokeObjectURL implementation, and
    // clicking a real <a href="blob:..."> would log a jsdom "not implemented:
    // navigation" error — stub all three so the save-blob flow runs cleanly.
    URL.createObjectURL = vi.fn().mockReturnValue('blob:mock-url')
    URL.revokeObjectURL = vi.fn()
    vi.spyOn(HTMLAnchorElement.prototype, 'click').mockImplementation(() => {})
  })

  it('renders label and filename', () => {
    mockActions()
    render(<ResumeCard resume={sampleResume} />)
    expect(screen.getByText('My resume')).toBeInTheDocument()
    expect(screen.getByText('resume.pdf')).toBeInTheDocument()
  })

  it('download button fetches the file blob and triggers a save', async () => {
    mockActions()
    render(<ResumeCard resume={sampleResume} />)
    fireEvent.click(screen.getByLabelText('Download'))
    await waitFor(() => expect(URL.createObjectURL).toHaveBeenCalledWith(sampleBlob))
    expect(HTMLAnchorElement.prototype.click).toHaveBeenCalled()
  })

  it('shows a spinner and disables the button while the download is pending', () => {
    mockActions({
      download: {
        mutateAsync: vi.fn().mockResolvedValue(sampleBlob),
        isPending: true,
      } as unknown as ReturnType<typeof actionsHook.useResumeActions>['download'],
    })
    render(<ResumeCard resume={sampleResume} />)
    const button = screen.getByLabelText('Downloading')
    expect(button).toBeDisabled()
    expect(button.querySelector('svg')).toHaveClass('animate-spin')
  })

  it('re-enables the download button and hides the spinner once the download settles', () => {
    mockActions()
    render(<ResumeCard resume={sampleResume} />)
    const button = screen.getByLabelText('Download')
    expect(button).not.toBeDisabled()
    expect(button.querySelector('svg')).not.toHaveClass('animate-spin')
  })

  it('delete button calls remove.mutate with the resume id', () => {
    const removeMutate = vi.fn()
    mockActions({
      remove: { mutate: removeMutate } as unknown as ReturnType<
        typeof actionsHook.useResumeActions
      >['remove'],
    })
    render(<ResumeCard resume={sampleResume} />)
    fireEvent.click(screen.getByLabelText('Delete'))
    expect(removeMutate).toHaveBeenCalledWith('r1')
  })

  it('rename: clicking the pencil then blurring the input submits the new label', async () => {
    const renameMutateAsync = vi.fn()
    mockActions({
      rename: { mutateAsync: renameMutateAsync } as unknown as ReturnType<
        typeof actionsHook.useResumeActions
      >['rename'],
    })
    render(<ResumeCard resume={sampleResume} />)
    fireEvent.click(screen.getByLabelText('Rename'))
    const input = screen.getByDisplayValue('My resume')
    fireEvent.change(input, { target: { value: 'New label' } })
    fireEvent.blur(input)
    await waitFor(() =>
      expect(renameMutateAsync).toHaveBeenCalledWith({ id: 'r1', label: 'New label' })
    )
  })

  it('rename is skipped when the label is unchanged', async () => {
    const renameMutateAsync = vi.fn()
    mockActions({
      rename: { mutateAsync: renameMutateAsync } as unknown as ReturnType<
        typeof actionsHook.useResumeActions
      >['rename'],
    })
    render(<ResumeCard resume={sampleResume} />)
    fireEvent.click(screen.getByLabelText('Rename'))
    const input = screen.getByDisplayValue('My resume')
    fireEvent.blur(input)
    await waitFor(() => expect(screen.queryByDisplayValue('My resume')).not.toBeInTheDocument())
    expect(renameMutateAsync).not.toHaveBeenCalled()
  })

  it('does not render keyword badges or an extract-keywords action', () => {
    mockActions()
    render(<ResumeCard resume={sampleResume} />)
    expect(screen.queryByLabelText('Extract keywords')).not.toBeInTheDocument()
    expect(screen.queryByText(/more$/)).not.toBeInTheDocument()
  })
})
