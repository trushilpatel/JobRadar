import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, cleanup, fireEvent } from '@testing-library/react'
import { MemoryRouter, Routes, Route } from 'react-router-dom'
import type { Job, JobStatus } from '@/types'
import { JOB_STATUS } from '@/config/constants/jobStatus'

// ── Mocks ─────────────────────────────────────────────────────────────────────
// useJob result is controlled per-test; useJobActions exposes mutate spies; the
// detail panel is stubbed so we assert the route's branching + handler wiring,
// not the panel internals (covered by its own test).

const navigateSpy = vi.fn()
vi.mock('react-router-dom', async (orig) => {
  const actual = await orig<typeof import('react-router-dom')>()
  return { ...actual, useNavigate: () => navigateSpy }
})

type UseJobResult = {
  data?: Job
  isLoading: boolean
  isError: boolean
  error?: unknown
  refetch: ReturnType<typeof vi.fn>
}
let jobResult: UseJobResult
const refetchSpy = vi.fn()
vi.mock('../../hooks/useJob', () => ({
  useJob: () => jobResult,
}))

const setStatusMutate = vi.fn()
const setSavedMutate = vi.fn()
vi.mock('../../hooks/useJobActions', () => ({
  useJobActions: () => ({
    setStatus: { mutate: setStatusMutate },
    setSaved: { mutate: setSavedMutate },
  }),
}))

vi.mock('../JobDetailPanel', () => ({
  JobDetailPanel: ({
    job,
    onClose,
    onStatusChange,
    onToggleSave,
  }: {
    job: Job
    onClose: () => void
    onStatusChange: (j: Job, s: Exclude<JobStatus, 'dismissed' | 'saved'>) => void
    onToggleSave: (j: Job) => void
  }) => (
    <div data-testid="detail-panel">
      <span>{job.title}</span>
      <button type="button" onClick={onClose}>
        close
      </button>
      <button type="button" onClick={() => onStatusChange(job, JOB_STATUS.APPLIED)}>
        apply
      </button>
      <button type="button" onClick={() => onToggleSave(job)}>
        toggle-save
      </button>
    </div>
  ),
}))

import { MobileJobDetailRoute } from '../MobileJobDetailRoute'

function makeJob(overrides: Partial<Job> = {}): Job {
  return {
    id: 'j1',
    job_post_id: 'p1',
    company_id: 'c1',
    title: 'Software Engineer',
    location: 'New York',
    remote: false,
    url: 'https://example.com/job/1',
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
    status: JOB_STATUS.NEW,
    saved: false,
    notes: null,
    user_state: {
      status: JOB_STATUS.NEW,
      notes: null,
      saved_at: null,
      applied_at: null,
      dismissed_at: null,
      dismissal_reason: null,
      dismissal_note: null,
    },
    added_at: '2024-01-01T00:00:00Z',
    company: {
      id: 'c1',
      name: 'Acme Corp',
      url: 'https://acme.com',
      adapter_type: 'greenhouse',
      active: true,
      created_at: '2024-01-01T00:00:00Z',
      last_scan_at: null,
      last_scan_status: null,
    },
    ...overrides,
  } as Job
}

function renderRoute(initialState?: { idx?: number }) {
  return render(
    <MemoryRouter initialEntries={[{ pathname: '/jobs/j1', state: initialState ?? null }]}>
      <Routes>
        <Route path="/jobs/:id" element={<MobileJobDetailRoute />} />
        <Route path="/jobs" element={<div>jobs list</div>} />
      </Routes>
    </MemoryRouter>
  )
}

beforeEach(() => {
  cleanup()
  vi.clearAllMocks()
  jobResult = {
    data: makeJob(),
    isLoading: false,
    isError: false,
    error: undefined,
    refetch: refetchSpy,
  }
})

describe('MobileJobDetailRoute — render states', () => {
  it('shows the loading skeleton while loading', () => {
    jobResult = { isLoading: true, isError: false, refetch: refetchSpy }
    const { container } = renderRoute()
    expect(container.querySelector('.animate-pulse')).toBeTruthy()
    expect(screen.queryByTestId('detail-panel')).not.toBeInTheDocument()
  })

  it('renders the detail panel when the job is found', () => {
    renderRoute()
    expect(screen.getByTestId('detail-panel')).toBeInTheDocument()
    expect(screen.getByText('Software Engineer')).toBeInTheDocument()
  })

  it('shows a not-found message with "Browse all jobs" on a 404 error', () => {
    jobResult = {
      isLoading: false,
      isError: true,
      error: new Error('404 not found'),
      refetch: refetchSpy,
    }
    renderRoute()
    expect(screen.getByText("We couldn't find that job.")).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Browse all jobs' })).toBeInTheDocument()
  })

  it('shows a forbidden message with no retry on a 403 error', () => {
    jobResult = {
      isLoading: false,
      isError: true,
      error: new Error('403 forbidden'),
      refetch: refetchSpy,
    }
    renderRoute()
    expect(screen.getByText("You don't have access to this job.")).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'Try again' })).not.toBeInTheDocument()
  })

  it('shows a generic error with "Try again" on an unknown error', () => {
    jobResult = {
      isLoading: false,
      isError: true,
      error: new Error('network blew up'),
      refetch: refetchSpy,
    }
    renderRoute()
    expect(screen.getByText("Couldn't load this job.")).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: 'Try again' }))
    expect(refetchSpy).toHaveBeenCalled()
  })

  it('treats a non-Error rejection as a generic error (empty message)', () => {
    jobResult = {
      isLoading: false,
      isError: false,
      data: undefined,
      error: 'string error',
      refetch: refetchSpy,
    }
    renderRoute()
    expect(screen.getByText("Couldn't load this job.")).toBeInTheDocument()
  })
})

describe('MobileJobDetailRoute — navigation', () => {
  it('"Browse all jobs" navigates to /jobs', () => {
    jobResult = {
      isLoading: false,
      isError: true,
      error: new Error('404'),
      refetch: refetchSpy,
    }
    renderRoute()
    fireEvent.click(screen.getByRole('button', { name: 'Browse all jobs' }))
    expect(navigateSpy).toHaveBeenCalledWith('/jobs')
  })

  it('Back button navigates back (-1) when entered via in-app nav', () => {
    jobResult = {
      isLoading: false,
      isError: true,
      error: new Error('boom'),
      refetch: refetchSpy,
    }
    renderRoute({ idx: 2 })
    fireEvent.click(screen.getByRole('button', { name: 'Back' }))
    expect(navigateSpy).toHaveBeenCalledWith(-1)
  })

  it('Back button falls back to /jobs on a deep-link entry (no in-app history)', () => {
    jobResult = {
      isLoading: false,
      isError: true,
      error: new Error('boom'),
      refetch: refetchSpy,
    }
    renderRoute()
    fireEvent.click(screen.getByRole('button', { name: 'Back' }))
    expect(navigateSpy).toHaveBeenCalledWith('/jobs')
  })
})

describe('MobileJobDetailRoute — action handlers', () => {
  it('status change mutates and navigates back', () => {
    renderRoute({ idx: 2 })
    fireEvent.click(screen.getByText('apply'))
    expect(setStatusMutate).toHaveBeenCalledWith({ id: 'j1', status: JOB_STATUS.APPLIED })
    expect(navigateSpy).toHaveBeenCalledWith(-1)
  })

  it('toggle save mutates to saved=true for an unsaved job and stays on the panel', () => {
    renderRoute()
    fireEvent.click(screen.getByText('toggle-save'))
    expect(setSavedMutate).toHaveBeenCalledWith({ id: 'j1', saved: true })
    expect(navigateSpy).not.toHaveBeenCalled()
  })

  it('toggle save mutates to saved=false for an already-saved job', () => {
    jobResult = {
      isLoading: false,
      isError: false,
      data: makeJob({
        user_state: {
          status: JOB_STATUS.NEW,
          notes: null,
          saved_at: '2024-05-01T00:00:00Z',
          applied_at: null,
          dismissed_at: null,
          dismissal_reason: null,
          dismissal_note: null,
        },
      }),
      refetch: refetchSpy,
    }
    renderRoute()
    fireEvent.click(screen.getByText('toggle-save'))
    expect(setSavedMutate).toHaveBeenCalledWith({ id: 'j1', saved: false })
  })

  it('close (onClose) triggers safeBack navigation', () => {
    renderRoute({ idx: 2 })
    fireEvent.click(screen.getByText('close'))
    expect(navigateSpy).toHaveBeenCalledWith(-1)
  })
})
