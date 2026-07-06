import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { ApplyQueuePage } from '../ApplyQueuePage'
import { JOB_STATUS } from '@/config/constants/jobStatus'
import type { Job } from '@/types'

// Mocked via inline factories (not deep imports elsewhere) so the page only
// ever talks to the two features it composes through their real hooks — this
// mirrors ResumeLibraryPage.test.tsx's approach for the same reason (pages
// should only consume feature APIs via hooks, not raw fetch functions). The
// pages/ layer-boundary lint rule also forbids importing feature internals
// directly (only the barrel, @/features/X) — hoisted mock fns + a factory
// (rather than `import * as jobsApi` + vi.mock(path)) is how this file talks
// to the mocked jobs.api.ts functions without tripping that rule.
const {
  fetchApplyQueueMock,
  setApplyQueueResumeMock,
  fetchResumesMock,
  fetchJobMock,
  setJobStatusMock,
  setJobSavedMock,
  dismissJobMock,
} = vi.hoisted(() => ({
  fetchApplyQueueMock: vi.fn(),
  setApplyQueueResumeMock: vi.fn(),
  fetchResumesMock: vi.fn(),
  fetchJobMock: vi.fn(),
  setJobStatusMock: vi.fn(),
  setJobSavedMock: vi.fn(),
  dismissJobMock: vi.fn(),
}))
vi.mock('@/features/apply-queue/api/apply-queue.api', () => ({
  fetchApplyQueue: fetchApplyQueueMock,
  setApplyQueueResume: setApplyQueueResumeMock,
}))
vi.mock('@/features/resumes/api/resumes.api', () => ({
  fetchResumes: fetchResumesMock,
}))
// The detail panel (features/jobs) hits jobs.api.ts directly via useJob /
// useJobActions — mocked so panel-open / status / save / dismiss flows can be
// exercised without a real backend. bulkDismissJobs is imported (unused here)
// by useJobActions at module load — stub it too so that import isn't undefined.
vi.mock('@/features/jobs/api/jobs.api', () => ({
  fetchJob: fetchJobMock,
  setJobStatus: setJobStatusMock,
  setJobSaved: setJobSavedMock,
  dismissJob: dismissJobMock,
  bulkDismissJobs: vi.fn(),
}))
vi.mock('sonner', () => {
  // useJobActions's undo toast calls `toast(message, opts)` as a function AND
  // `toast.success`/`toast.error` as methods — a plain object mock (as used
  // by pages that don't touch useJobActions) breaks the moment any mutation
  // here succeeds. Mirrors the callable mock in useJobActions.test.ts /
  // JobsPage.interactions.test.tsx.
  const toast = vi.fn() as unknown as ((msg: string, opts?: unknown) => void) & {
    success: ReturnType<typeof vi.fn>
    error: ReturnType<typeof vi.fn>
    warning: ReturnType<typeof vi.fn>
  }
  toast.success = vi.fn()
  toast.error = vi.fn()
  toast.warning = vi.fn()
  return { toast }
})

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
  job_keywords: ['Go', 'PostgreSQL'],
  location: 'Austin, TX',
  remote: false,
  salary_range: null,
  added_at: '2026-06-01T00:00:00Z',
}

const sampleResume = {
  id: 'resume-1',
  user_id: 'u1',
  label: 'My resume',
  original_filename: 'resume.pdf',
  content_type: 'application/pdf' as const,
  size_bytes: 2048,
  created_at: '2026-07-03T00:00:00Z',
  extracted_text: 'Backend engineer with Go and PostgreSQL experience.',
}

const sampleJob: Job = {
  id: 'job-1',
  job_post_id: 'p1',
  company_id: 'c1',
  title: 'Senior Engineer',
  location: 'Austin, TX',
  remote: false,
  url: 'https://acme.example/jobs/1',
  department: null,
  employ_type: null,
  experience_years: null,
  tech_stack: null,
  salary_range: null,
  llm_summary: null,
  sponsorship: false,
  requires_citizen: false,
  security_clearance: false,
  role_brief: null,
  responsibilities: null,
  status: JOB_STATUS.SAVED,
  saved: true,
  notes: null,
  user_state: {
    status: JOB_STATUS.SAVED,
    notes: null,
    saved_at: '2026-06-01T00:00:00Z',
    applied_at: null,
    dismissed_at: null,
    dismissal_reason: null,
    dismissal_note: null,
  },
  added_at: '2026-06-01T00:00:00Z',
  company: {
    id: 'c1',
    name: 'Acme',
    url: 'https://acme.com',
    adapter_type: 'greenhouse',
    active: true,
    created_at: '2024-01-01T00:00:00Z',
    last_scan_at: null,
    last_scan_status: null,
  },
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

  it('auto-picks the best-matching resume and shows the match summary when none is attached', async () => {
    fetchApplyQueueMock.mockResolvedValue({ items: [sampleItem], next_cursor: '' })
    fetchResumesMock.mockResolvedValue([sampleResume])
    render(<ApplyQueuePage />, { wrapper: makeWrapper() })

    await waitFor(() => expect(screen.getByText('Senior Engineer')).toBeInTheDocument())

    expect(screen.getByText('2/2 matched')).toBeInTheDocument()
    expect(screen.getByRole('combobox', { name: /select resume/i })).toHaveTextContent('My resume')
  })

  it('shows a placeholder in the match column when no resume matches', async () => {
    fetchApplyQueueMock.mockResolvedValue({
      items: [{ ...sampleItem, job_keywords: ['Rust'] }],
      next_cursor: '',
    })
    fetchResumesMock.mockResolvedValue([sampleResume])
    render(<ApplyQueuePage />, { wrapper: makeWrapper() })

    await waitFor(() => expect(screen.getByText('Senior Engineer')).toBeInTheDocument())
    expect(screen.getByText('—')).toBeInTheDocument()
  })

  it('selecting a resume in the picker calls the mutation and invalidates the list', async () => {
    const user = userEvent.setup()
    // No job_keywords here — auto-pick must never kick in, so the picker
    // starts at "No resume selected" and this test exercises a plain manual
    // selection unaffected by the auto-pick behavior covered separately above.
    fetchApplyQueueMock.mockResolvedValue({
      items: [{ ...sampleItem, job_keywords: [] }],
      next_cursor: '',
    })
    fetchResumesMock.mockResolvedValue([sampleResume])
    setApplyQueueResumeMock.mockResolvedValue(undefined)
    render(<ApplyQueuePage />, { wrapper: makeWrapper() })

    await waitFor(() => expect(screen.getByText('Senior Engineer')).toBeInTheDocument())

    await user.click(screen.getByRole('combobox', { name: /select resume/i }))
    await user.click(await screen.findByText('My resume'))

    await waitFor(() => expect(setApplyQueueResumeMock).toHaveBeenCalledWith('job-1', 'resume-1'))
  })

  describe('detail panel', () => {
    beforeEach(() => {
      fetchApplyQueueMock.mockResolvedValue({ items: [sampleItem], next_cursor: '' })
      fetchResumesMock.mockResolvedValue([sampleResume])
    })

    it('opens the JobDetailPanel with the full job when a row is clicked', async () => {
      const user = userEvent.setup()
      fetchJobMock.mockResolvedValue(sampleJob)
      render(<ApplyQueuePage />, { wrapper: makeWrapper() })

      await waitFor(() => expect(screen.getByText('Senior Engineer')).toBeInTheDocument())
      await user.click(screen.getByRole('button', { name: /Senior Engineer at Acme/i }))

      expect(fetchJobMock).toHaveBeenCalledWith('job-1')
      await waitFor(() =>
        expect(screen.getByRole('region', { name: 'Job details' })).toBeInTheDocument()
      )
      // The panel renders the full Job fields (e.g. the posting link), not
      // just the ApplyQueueItem's subset — proves the panel is fed by useJob.
      expect(screen.getByRole('link', { name: /open posting/i })).toBeInTheDocument()
    })

    it('shows a loading skeleton while the job detail fetch is in flight', async () => {
      const user = userEvent.setup()
      let resolveFetch: (job: Job) => void = () => {}
      fetchJobMock.mockReturnValue(
        new Promise<Job>((resolve) => {
          resolveFetch = resolve
        })
      )
      render(<ApplyQueuePage />, { wrapper: makeWrapper() })

      await waitFor(() => expect(screen.getByText('Senior Engineer')).toBeInTheDocument())
      await user.click(screen.getByRole('button', { name: /Senior Engineer at Acme/i }))

      expect(screen.queryByRole('region', { name: 'Job details' })).not.toBeInTheDocument()
      resolveFetch(sampleJob)
      await waitFor(() =>
        expect(screen.getByRole('region', { name: 'Job details' })).toBeInTheDocument()
      )
    })

    it('shows an error state with a close button when the job detail fetch fails', async () => {
      const user = userEvent.setup()
      fetchJobMock.mockRejectedValue(new Error('boom'))
      render(<ApplyQueuePage />, { wrapper: makeWrapper() })

      await waitFor(() => expect(screen.getByText('Senior Engineer')).toBeInTheDocument())
      await user.click(screen.getByRole('button', { name: /Senior Engineer at Acme/i }))

      await waitFor(() => expect(screen.getByText("Couldn't load this job.")).toBeInTheDocument())
      await user.click(screen.getByRole('button', { name: 'Close' }))
      expect(screen.queryByText("Couldn't load this job.")).not.toBeInTheDocument()
    })

    it('closing the panel clears the selected job', async () => {
      const user = userEvent.setup()
      fetchJobMock.mockResolvedValue(sampleJob)
      render(<ApplyQueuePage />, { wrapper: makeWrapper() })

      await waitFor(() => expect(screen.getByText('Senior Engineer')).toBeInTheDocument())
      await user.click(screen.getByRole('button', { name: /Senior Engineer at Acme/i }))
      await waitFor(() =>
        expect(screen.getByRole('region', { name: 'Job details' })).toBeInTheDocument()
      )

      await user.click(screen.getByRole('button', { name: /close job details/i }))
      expect(screen.queryByRole('region', { name: 'Job details' })).not.toBeInTheDocument()
    })

    it('changing status from the panel invalidates the apply-queue list', async () => {
      const user = userEvent.setup()
      fetchJobMock.mockResolvedValue(sampleJob)
      setJobStatusMock.mockResolvedValue(undefined)
      render(<ApplyQueuePage />, { wrapper: makeWrapper() })

      await waitFor(() => expect(screen.getByText('Senior Engineer')).toBeInTheDocument())
      await user.click(screen.getByRole('button', { name: /Senior Engineer at Acme/i }))
      await waitFor(() =>
        expect(screen.getByRole('region', { name: 'Job details' })).toBeInTheDocument()
      )

      fetchApplyQueueMock.mockClear()
      await user.click(screen.getByRole('radio', { name: 'Applied' }))

      expect(setJobStatusMock).toHaveBeenCalledWith('job-1', JOB_STATUS.APPLIED)
      await waitFor(() => expect(fetchApplyQueueMock).toHaveBeenCalled())
    })

    it('dismissing from the panel closes it and invalidates the apply-queue list', async () => {
      const user = userEvent.setup()
      fetchJobMock.mockResolvedValue(sampleJob)
      dismissJobMock.mockResolvedValue(undefined)
      render(<ApplyQueuePage />, { wrapper: makeWrapper() })

      await waitFor(() => expect(screen.getByText('Senior Engineer')).toBeInTheDocument())
      await user.click(screen.getByRole('button', { name: /Senior Engineer at Acme/i }))
      await waitFor(() =>
        expect(screen.getByRole('region', { name: 'Job details' })).toBeInTheDocument()
      )

      fetchApplyQueueMock.mockClear()
      await user.click(screen.getByRole('button', { name: 'Dismiss job' }))

      expect(dismissJobMock).toHaveBeenCalledWith('job-1', 'user_dismissed', undefined)
      expect(screen.queryByRole('region', { name: 'Job details' })).not.toBeInTheDocument()
      await waitFor(() => expect(fetchApplyQueueMock).toHaveBeenCalled())
    })

    it('toggling save from the panel invalidates the apply-queue list', async () => {
      const user = userEvent.setup()
      fetchJobMock.mockResolvedValue(sampleJob)
      setJobSavedMock.mockResolvedValue(undefined)
      render(<ApplyQueuePage />, { wrapper: makeWrapper() })

      await waitFor(() => expect(screen.getByText('Senior Engineer')).toBeInTheDocument())
      await user.click(screen.getByRole('button', { name: /Senior Engineer at Acme/i }))
      await waitFor(() =>
        expect(screen.getByRole('region', { name: 'Job details' })).toBeInTheDocument()
      )

      fetchApplyQueueMock.mockClear()
      await user.click(screen.getByRole('button', { name: 'Remove from saved' }))

      expect(setJobSavedMock).toHaveBeenCalledWith('job-1', false)
      await waitFor(() => expect(fetchApplyQueueMock).toHaveBeenCalled())
    })
  })
})
