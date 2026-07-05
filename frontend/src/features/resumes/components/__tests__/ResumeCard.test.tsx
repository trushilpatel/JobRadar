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
}

function mockActions(overrides: Partial<ReturnType<typeof actionsHook.useResumeActions>> = {}) {
  vi.mocked(actionsHook.useResumeActions).mockReturnValue({
    upload: { mutateAsync: vi.fn(), isPending: false } as unknown as ReturnType<
      typeof actionsHook.useResumeActions
    >['upload'],
    download: {
      mutateAsync: vi.fn().mockResolvedValue('https://example.test/signed'),
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
    Object.defineProperty(window, 'location', { value: { assign: vi.fn() }, writable: true })
  })

  it('renders label and filename', () => {
    mockActions()
    render(<ResumeCard resume={sampleResume} />)
    expect(screen.getByText('My resume')).toBeInTheDocument()
    expect(screen.getByText('resume.pdf')).toBeInTheDocument()
  })

  it('download button fetches a URL and navigates to it', async () => {
    mockActions()
    render(<ResumeCard resume={sampleResume} />)
    fireEvent.click(screen.getByLabelText('Download'))
    await waitFor(() =>
      expect(window.location.assign).toHaveBeenCalledWith('https://example.test/signed')
    )
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
})
