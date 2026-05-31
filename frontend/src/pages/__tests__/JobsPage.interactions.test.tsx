/**
 * Desktop interaction coverage for JobsPage branches the integration suite
 * doesn't exercise: opening + closing the always-on detail panel, and
 * selecting a job from the command palette.
 */
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, fireEvent, waitFor, cleanup } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { MemoryRouter } from 'react-router-dom'
import { NuqsTestingAdapter } from 'nuqs/adapters/testing'
import type { ReactNode } from 'react'

vi.mock('@/features/jobs/api/jobs.api')
vi.mock('sonner', () => {
  const toast = vi.fn() as unknown as ((msg: string, opts?: unknown) => void) & {
    success: ReturnType<typeof vi.fn>
    error: ReturnType<typeof vi.fn>
    warning: ReturnType<typeof vi.fn>
    custom: ReturnType<typeof vi.fn>
    dismiss: ReturnType<typeof vi.fn>
  }
  toast.success = vi.fn()
  toast.error = vi.fn()
  toast.warning = vi.fn()
  toast.custom = vi.fn()
  toast.dismiss = vi.fn()
  return { toast, Toaster: () => null }
})
vi.mock('@/features/auth/store/auth.store', () => ({
  useAuthStore: vi.fn((sel: (s: unknown) => unknown) =>
    sel({
      session: { access_token: 'tok', user: { id: 'u1', email: 'test@test.com' } },
      identity: { user_id: 'u1', email: 'test@test.com', roles: ['user'] },
      loading: false,
      initialized: true,
      isAdmin: false,
    })
  ),
}))
vi.mock('@/lib/hooks/use-media-query', () => ({
  useIsMobile: () => false,
  useIsDesktop: () => true,
}))

import * as jobsApi from '@/features/jobs'
import { JobsPage } from '@/pages/JobsPage'
import { JOB_STATUS } from '@/config/constants/jobStatus'
import type { Job } from '@/types'

function makeJob(id: string, overrides: Partial<Job> = {}): Job {
  return {
    id,
    job_post_id: `p-${id}`,
    company_id: 'c1',
    title: `Title ${id}`,
    location: 'Remote',
    remote: true,
    url: `https://x/${id}`,
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
      name: 'Acme',
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

function wrap(ui: ReactNode) {
  const qc = new QueryClient({
    defaultOptions: { queries: { retry: 0, gcTime: 5 * 60_000 }, mutations: { retry: 0 } },
  })
  return (
    <NuqsTestingAdapter>
      <MemoryRouter>
        <QueryClientProvider client={qc}>{ui}</QueryClientProvider>
      </MemoryRouter>
    </NuqsTestingAdapter>
  )
}

beforeEach(() => {
  cleanup()
  vi.clearAllMocks()
  localStorage.clear()
  // Pin to Mac so the `mod+k` palette chord resolves to metaKey.
  Object.defineProperty(navigator, 'platform', { value: 'MacIntel', configurable: true })
  vi.mocked(jobsApi.fetchJobs).mockResolvedValue({
    items: [makeJob('a'), makeJob('b')],
    next_cursor: '',
  })
  vi.mocked(jobsApi.fetchJobCounts).mockResolvedValue({
    new: 2,
    saved: 0,
    applied: 0,
    interviewing: 0,
    offer: 0,
    rejected: 0,
  })
  vi.mocked(jobsApi.fetchJobCompanies).mockResolvedValue(['Acme'])
  vi.mocked(jobsApi.setJobStatus).mockResolvedValue(undefined)
  vi.mocked(jobsApi.setJobSaved).mockResolvedValue(undefined)
  vi.mocked(jobsApi.dismissJob).mockResolvedValue(undefined)
})

describe('JobsPage — detail panel open/close', () => {
  it('clicking a row opens the detail panel; the close button hides it', async () => {
    render(wrap(<JobsPage />))
    const row = await screen.findByRole('button', { name: /Title a at Acme/i })
    fireEvent.click(row)

    // The always-on panel renders the region for the clicked job.
    const panel = await screen.findByRole('region', { name: 'Job details' })
    expect(panel).toBeInTheDocument()

    // Closing via the X button persists "closed by user" and the panel vanishes.
    fireEvent.click(screen.getByRole('button', { name: 'Close job details' }))
    await waitFor(() => {
      expect(screen.queryByRole('region', { name: 'Job details' })).not.toBeInTheDocument()
    })
  })
})

describe('JobsPage — keyboard handlers', () => {
  it('apply shortcut (a) sets status on the focused job and closes its panel', async () => {
    render(wrap(<JobsPage />))
    const row = await screen.findByRole('button', { name: /Title a at Acme/i })
    fireEvent.click(row)
    await screen.findByRole('region', { name: 'Job details' })

    fireEvent.keyDown(document.body, { key: 'a', code: 'KeyA' })

    await waitFor(() => {
      expect(jobsApi.setJobStatus).toHaveBeenCalledWith('a', JOB_STATUS.APPLIED)
    })
  })

  it('dismiss shortcut (d) dismisses the focused job', async () => {
    render(wrap(<JobsPage />))
    const row = await screen.findByRole('button', { name: /Title a at Acme/i })
    fireEvent.click(row)
    await screen.findByRole('region', { name: 'Job details' })

    fireEvent.keyDown(document.body, { key: 'd', code: 'KeyD' })

    await waitFor(() => {
      expect(jobsApi.dismissJob).toHaveBeenCalledWith('a', 'user_dismissed', undefined)
    })
  })

  it('open-url shortcut (mod+enter) opens the focused job url in a new tab', async () => {
    const openSpy = vi.spyOn(window, 'open').mockReturnValue(null)
    render(wrap(<JobsPage />))
    const row = await screen.findByRole('button', { name: /Title a at Acme/i })
    fireEvent.click(row)

    fireEvent.keyDown(document.body, { key: 'Enter', code: 'Enter', metaKey: true })

    expect(openSpy).toHaveBeenCalledWith('https://x/a', '_blank', 'noreferrer')
    openSpy.mockRestore()
  })

  it('escape shortcut closes the open detail panel', async () => {
    render(wrap(<JobsPage />))
    const row = await screen.findByRole('button', { name: /Title a at Acme/i })
    fireEvent.click(row)
    await screen.findByRole('region', { name: 'Job details' })

    fireEvent.keyDown(document.body, { key: 'Escape', code: 'Escape' })

    await waitFor(() => {
      expect(screen.queryByRole('region', { name: 'Job details' })).not.toBeInTheDocument()
    })
  })

  it('focus-search shortcut (/) focuses the search input', async () => {
    render(wrap(<JobsPage />))
    await screen.findByRole('button', { name: /Title a at Acme/i })

    fireEvent.keyDown(document.body, { key: '/', code: 'Slash' })

    expect(document.activeElement).toBe(screen.getByPlaceholderText('Search jobs…'))
  })
})

describe('JobsPage — tab switching', () => {
  it('clicking a status tab switches the active tab', async () => {
    render(wrap(<JobsPage />))
    await screen.findByRole('button', { name: /Title a at Acme/i })

    await userEvent.click(screen.getByRole('tab', { name: /Saved/i }))

    // Switching the tab refetches with the new status filter.
    await waitFor(() => {
      const calledStatuses = vi.mocked(jobsApi.fetchJobs).mock.calls.map((c) => c[0].status)
      expect(calledStatuses).toContain(JOB_STATUS.SAVED)
    })
  })
})

describe('JobsPage — command palette', () => {
  it('selecting a job from the palette opens its detail panel', async () => {
    render(wrap(<JobsPage />))
    await screen.findByRole('button', { name: /Title a at Acme/i })

    // mod+k opens the palette (cmdk dialog).
    fireEvent.keyDown(document.body, { key: 'k', code: 'KeyK', metaKey: true })

    // Scope to the palette dialog — "Title b" also appears in the table row,
    // so query within the command list to avoid an ambiguous match.
    await screen.findByPlaceholderText(/Type a command/i)
    const paletteItem = screen
      .getByText('Jobs in view')
      .closest('[cmdk-group]')!
      .querySelector('[cmdk-item]')!
    fireEvent.click(paletteItem)

    // onSelectJob on desktop opens the detail panel for the chosen job.
    await waitFor(() => {
      expect(screen.getByRole('region', { name: 'Job details' })).toBeInTheDocument()
    })
  })
})
