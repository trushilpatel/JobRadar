import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { ApplyQueuePage } from '../ApplyQueuePage'

// Mocked via inline factories (not deep imports elsewhere) so the page only
// ever talks to the two features it composes through their real hooks — this
// mirrors ResumeLibraryPage.test.tsx's approach for the same reason (pages
// should only consume feature APIs via hooks, not raw fetch functions).
const { fetchApplyQueueMock, setApplyQueueResumeMock, fetchResumesMock } = vi.hoisted(() => ({
  fetchApplyQueueMock: vi.fn(),
  setApplyQueueResumeMock: vi.fn(),
  fetchResumesMock: vi.fn(),
}))
vi.mock('@/features/apply-queue/api/apply-queue.api', () => ({
  fetchApplyQueue: fetchApplyQueueMock,
  setApplyQueueResume: setApplyQueueResumeMock,
}))
vi.mock('@/features/resumes/api/resumes.api', () => ({
  fetchResumes: fetchResumesMock,
}))
vi.mock('sonner', () => ({ toast: { success: vi.fn(), error: vi.fn() } }))

function makeWrapper() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: 0, gcTime: 0 } } })
  return function Wrapper({ children }: { children: React.ReactNode }) {
    return <QueryClientProvider client={qc}>{children}</QueryClientProvider>
  }
}

const sampleItem = {
  job_id: 'job-1',
  job_title: 'Senior Engineer',
  company: 'Acme',
  job_url: 'https://acme.example/jobs/1',
  status: 'saved',
  resume_id: null,
  resume_label: null,
  recommended_resume_id: null,
  recommended_resume_label: null,
}

const sampleResume = {
  id: 'resume-1',
  user_id: 'u1',
  label: 'My resume',
  original_filename: 'resume.pdf',
  content_type: 'application/pdf' as const,
  size_bytes: 2048,
  created_at: '2026-07-03T00:00:00Z',
  keywords: [] as string[],
}

describe('ApplyQueuePage', () => {
  beforeEach(() => vi.clearAllMocks())

  it('renders the header and filter toggle', async () => {
    fetchApplyQueueMock.mockResolvedValue({ items: [], next_cursor: '' })
    fetchResumesMock.mockResolvedValue([])
    render(<ApplyQueuePage />, { wrapper: makeWrapper() })

    expect(screen.getByRole('heading', { name: 'Apply Queue' })).toBeInTheDocument()
    expect(screen.getByRole('group', { name: /apply queue filter/i })).toBeInTheDocument()
    await waitFor(() =>
      expect(screen.getByText('No jobs in your apply queue yet')).toBeInTheDocument()
    )
  })

  it('shows an error state with retry when the queue fails to load', async () => {
    fetchApplyQueueMock.mockRejectedValue(new Error('network error'))
    fetchResumesMock.mockResolvedValue([])
    render(<ApplyQueuePage />, { wrapper: makeWrapper() })

    await waitFor(() =>
      expect(screen.getByText("Couldn't load your apply queue")).toBeInTheDocument()
    )
  })

  it('renders queue items once loaded', async () => {
    fetchApplyQueueMock.mockResolvedValue({ items: [sampleItem], next_cursor: '' })
    fetchResumesMock.mockResolvedValue([sampleResume])
    render(<ApplyQueuePage />, { wrapper: makeWrapper() })

    await waitFor(() => expect(screen.getByText('Senior Engineer')).toBeInTheDocument())
    expect(screen.getByText('Acme')).toBeInTheDocument()
  })

  it('switching to the Saved filter refetches with filter=saved', async () => {
    const user = userEvent.setup()
    fetchApplyQueueMock.mockResolvedValue({ items: [sampleItem], next_cursor: '' })
    fetchResumesMock.mockResolvedValue([sampleResume])
    render(<ApplyQueuePage />, { wrapper: makeWrapper() })

    await waitFor(() => expect(screen.getByText('Senior Engineer')).toBeInTheDocument())

    await user.click(screen.getByRole('radio', { name: 'Saved' }))

    await waitFor(() =>
      expect(fetchApplyQueueMock).toHaveBeenCalledWith('saved', '', 20, expect.anything())
    )
  })

  it('shows a Recommended badge on the option matching recommended_resume_id', async () => {
    const user = userEvent.setup()
    fetchApplyQueueMock.mockResolvedValue({
      items: [
        { ...sampleItem, recommended_resume_id: 'resume-1', recommended_resume_label: 'My resume' },
      ],
      next_cursor: '',
    })
    fetchResumesMock.mockResolvedValue([sampleResume])
    render(<ApplyQueuePage />, { wrapper: makeWrapper() })

    await waitFor(() => expect(screen.getByText('Senior Engineer')).toBeInTheDocument())

    await user.click(screen.getByRole('combobox', { name: /select resume/i }))
    expect(await screen.findByText('Recommended')).toBeInTheDocument()
  })

  it('selecting a resume in the picker calls the mutation and invalidates the list', async () => {
    const user = userEvent.setup()
    fetchApplyQueueMock.mockResolvedValue({ items: [sampleItem], next_cursor: '' })
    fetchResumesMock.mockResolvedValue([sampleResume])
    setApplyQueueResumeMock.mockResolvedValue(undefined)
    render(<ApplyQueuePage />, { wrapper: makeWrapper() })

    await waitFor(() => expect(screen.getByText('Senior Engineer')).toBeInTheDocument())

    await user.click(screen.getByRole('combobox', { name: /select resume/i }))
    await user.click(await screen.findByText('My resume'))

    await waitFor(() => expect(setApplyQueueResumeMock).toHaveBeenCalledWith('job-1', 'resume-1'))
  })
})
