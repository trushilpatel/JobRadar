import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, fireEvent, waitFor } from '@testing-library/react'
import { ResumeCard } from '../ResumeCard'
import * as actionsHook from '../../hooks/useResumeActions'
import * as extractKeywordsHook from '../../hooks/useExtractResumeKeywords'

vi.mock('../../hooks/useResumeActions')
vi.mock('../../hooks/useExtractResumeKeywords')

const sampleResume = {
  id: 'r1',
  user_id: 'u1',
  label: 'My resume',
  original_filename: 'resume.pdf',
  content_type: 'application/pdf' as const,
  size_bytes: 2048,
  created_at: '2026-07-03T00:00:00Z',
  keywords: [] as string[],
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

function mockExtractKeywords(
  overrides: Partial<ReturnType<typeof extractKeywordsHook.useExtractResumeKeywords>> = {}
) {
  vi.mocked(extractKeywordsHook.useExtractResumeKeywords).mockReturnValue({
    mutate: vi.fn(),
    isPending: false,
    ...overrides,
  } as unknown as ReturnType<typeof extractKeywordsHook.useExtractResumeKeywords>)
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
    mockExtractKeywords()
    render(<ResumeCard resume={sampleResume} />)
    expect(screen.getByText('My resume')).toBeInTheDocument()
    expect(screen.getByText('resume.pdf')).toBeInTheDocument()
  })

  it('download button fetches the file blob and triggers a save', async () => {
    mockActions()
    mockExtractKeywords()
    render(<ResumeCard resume={sampleResume} />)
    fireEvent.click(screen.getByLabelText('Download'))
    await waitFor(() => expect(URL.createObjectURL).toHaveBeenCalledWith(sampleBlob))
    expect(HTMLAnchorElement.prototype.click).toHaveBeenCalled()
  })

  it('delete button calls remove.mutate with the resume id', () => {
    const removeMutate = vi.fn()
    mockActions({
      remove: { mutate: removeMutate } as unknown as ReturnType<
        typeof actionsHook.useResumeActions
      >['remove'],
    })
    mockExtractKeywords()
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
    mockExtractKeywords()
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
    mockExtractKeywords()
    render(<ResumeCard resume={sampleResume} />)
    fireEvent.click(screen.getByLabelText('Rename'))
    const input = screen.getByDisplayValue('My resume')
    fireEvent.blur(input)
    await waitFor(() => expect(screen.queryByDisplayValue('My resume')).not.toBeInTheDocument())
    expect(renameMutateAsync).not.toHaveBeenCalled()
  })

  it('renders no keyword badges and no "+N more" when keywords is empty', () => {
    mockActions()
    mockExtractKeywords()
    render(<ResumeCard resume={sampleResume} />)
    expect(screen.queryByText(/more$/)).not.toBeInTheDocument()
  })

  it('renders a badge per keyword when under the visible limit', () => {
    mockActions()
    mockExtractKeywords()
    render(<ResumeCard resume={{ ...sampleResume, keywords: ['python', 'react', 'sql'] }} />)
    expect(screen.getByText('python')).toBeInTheDocument()
    expect(screen.getByText('react')).toBeInTheDocument()
    expect(screen.getByText('sql')).toBeInTheDocument()
    expect(screen.queryByText(/more$/)).not.toBeInTheDocument()
  })

  it('caps visible keyword badges and shows a "+N more" affordance beyond the limit', () => {
    mockActions()
    mockExtractKeywords()
    const keywords = ['a', 'b', 'c', 'd', 'e', 'f', 'g', 'h']
    render(<ResumeCard resume={{ ...sampleResume, keywords }} />)
    expect(screen.getByText('a')).toBeInTheDocument()
    expect(screen.getByText('f')).toBeInTheDocument()
    expect(screen.queryByText('g')).not.toBeInTheDocument()
    expect(screen.getByText('+2 more')).toBeInTheDocument()
  })

  it('shows the "Extract keywords" action when keywords is empty', () => {
    mockActions()
    mockExtractKeywords()
    render(<ResumeCard resume={sampleResume} />)
    expect(screen.getByLabelText('Extract keywords')).toBeInTheDocument()
  })

  it('hides the "Extract keywords" action once keywords are present', () => {
    mockActions()
    mockExtractKeywords()
    render(<ResumeCard resume={{ ...sampleResume, keywords: ['python'] }} />)
    expect(screen.queryByLabelText('Extract keywords')).not.toBeInTheDocument()
  })

  it('clicking "Extract keywords" calls the mutation with the resume id', () => {
    const mutate = vi.fn()
    mockActions()
    mockExtractKeywords({ mutate })
    render(<ResumeCard resume={sampleResume} />)
    fireEvent.click(screen.getByLabelText('Extract keywords'))
    expect(mutate).toHaveBeenCalledWith('r1')
  })

  it('disables the "Extract keywords" action while pending', () => {
    mockActions()
    mockExtractKeywords({ isPending: true })
    render(<ResumeCard resume={sampleResume} />)
    expect(screen.getByLabelText('Extract keywords')).toBeDisabled()
  })
})
