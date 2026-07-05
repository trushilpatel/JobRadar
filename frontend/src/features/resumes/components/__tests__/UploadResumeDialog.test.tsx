import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, fireEvent, waitFor } from '@testing-library/react'
import { UploadResumeDialog } from '../UploadResumeDialog'
import * as actionsHook from '../../hooks/useResumeActions'

vi.mock('../../hooks/useResumeActions')

function mockUpload(mutateAsync = vi.fn(), isPending = false) {
  vi.mocked(actionsHook.useResumeActions).mockReturnValue({
    upload: { mutateAsync, isPending } as unknown as ReturnType<
      typeof actionsHook.useResumeActions
    >['upload'],
  } as ReturnType<typeof actionsHook.useResumeActions>)
}

describe('UploadResumeDialog', () => {
  beforeEach(() => vi.clearAllMocks())

  it('shows a validation error and does not upload when file exceeds 2MB', () => {
    mockUpload()
    render(<UploadResumeDialog open onOpenChange={vi.fn()} />)
    const bigFile = new File([new Uint8Array(2 * 1024 * 1024 + 1)], 'big.pdf', {
      type: 'application/pdf',
    })
    fireEvent.change(screen.getByLabelText(/File/i), { target: { files: [bigFile] } })
    expect(screen.getByText('File exceeds the 2MB limit')).toBeInTheDocument()
  })

  it('requires a file before submitting', () => {
    mockUpload()
    render(<UploadResumeDialog open onOpenChange={vi.fn()} />)
    fireEvent.click(screen.getByText('Upload'))
    expect(screen.getByText('Choose a PDF or DOCX file')).toBeInTheDocument()
  })

  it('submits the file and label, then closes the dialog on success', async () => {
    const mutateAsync = vi.fn().mockResolvedValue({})
    mockUpload(mutateAsync)
    const onOpenChange = vi.fn()
    render(<UploadResumeDialog open onOpenChange={onOpenChange} />)
    const file = new File(['data'], 'resume.pdf', { type: 'application/pdf' })
    fireEvent.change(screen.getByLabelText(/File/i), { target: { files: [file] } })
    fireEvent.click(screen.getByText('Upload'))
    await waitFor(() => expect(mutateAsync).toHaveBeenCalled())
    await waitFor(() => expect(onOpenChange).toHaveBeenCalledWith(false))
  })

  it('label defaults to the filename minus extension when the file is chosen', () => {
    mockUpload()
    render(<UploadResumeDialog open onOpenChange={vi.fn()} />)
    const file = new File(['data'], 'backend-resume.pdf', { type: 'application/pdf' })
    fireEvent.change(screen.getByLabelText(/File/i), { target: { files: [file] } })
    expect(screen.getByDisplayValue('backend-resume')).toBeInTheDocument()
  })

  it('keeps the dialog open when the upload fails', async () => {
    const mutateAsync = vi.fn().mockRejectedValue(new Error('upload failed'))
    mockUpload(mutateAsync)
    const onOpenChange = vi.fn()
    render(<UploadResumeDialog open onOpenChange={onOpenChange} />)
    const file = new File(['data'], 'resume.pdf', { type: 'application/pdf' })
    fireEvent.change(screen.getByLabelText(/File/i), { target: { files: [file] } })
    fireEvent.click(screen.getByText('Upload'))
    await waitFor(() => expect(mutateAsync).toHaveBeenCalled())
    expect(onOpenChange).not.toHaveBeenCalledWith(false)
  })
})
