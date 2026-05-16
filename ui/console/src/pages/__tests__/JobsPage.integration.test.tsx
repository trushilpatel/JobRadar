/**
 * Cross-cutting regression tests for the jobs-page audit fixes.
 *
 * Each test is the simplest possible reproduction of a bug the audit found.
 * They live at the page level (rather than co-located with the hook/component
 * they exercise) because the bug only manifests in the interaction *between*
 * the hook and the page wiring — a passing unit test on either side alone
 * would not catch them.
 */
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, fireEvent, waitFor, within } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { MemoryRouter } from 'react-router-dom'
import { NuqsTestingAdapter } from 'nuqs/adapters/testing'
import type { ReactNode } from 'react'

// ── Module mocks ─────────────────────────────────────────────────────────────
vi.mock('@/api/jobs')
vi.mock('sonner', () => {
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
vi.mock('@/features/auth/store', () => ({
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
vi.mock('@/lib/use-media-query', () => ({
  useIsMobile: () => false,
  useIsDesktop: () => true,
}))

import * as jobsApi from '@/api/jobs'
import { JobsPage } from '@/pages/JobsPage'
import { JOB_STATUS } from '@/constants/jobStatus'
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
  vi.clearAllMocks()
  // Default API responses.
  vi.mocked(jobsApi.fetchJobs).mockResolvedValue({
    items: [makeJob('a'), makeJob('b'), makeJob('c', { status: JOB_STATUS.SAVED })],
    next_cursor: '',
  })
  vi.mocked(jobsApi.fetchJobCounts).mockResolvedValue({
    new: 3,
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
  vi.mocked(jobsApi.bulkDismissJobs).mockResolvedValue({ dismissed: 0 })
})

describe('JobsPage — regression coverage for audit fixes', () => {
  it('REGRESSION: clicking row 3 then pressing `s` saves row 3, not row 0', async () => {
    render(wrap(<JobsPage />))
    // Wait for the rows to render.
    const row = await screen.findByRole('button', { name: /Title c at Acme/i })
    fireEvent.click(row)

    // Press `s` to save — should target the clicked job ('c'), not jobs[0].
    fireEvent.keyDown(document.body, { key: 's', code: 'KeyS' })

    await waitFor(() => {
      expect(jobsApi.setJobStatus).toHaveBeenCalledWith('c', JOB_STATUS.SAVED)
    })
    // And NOT row 0.
    const calls = vi.mocked(jobsApi.setJobStatus).mock.calls.map((c) => c[0])
    expect(calls).not.toContain('a')
  })

  it('REGRESSION: keyboard rows expose accessible names for screen readers', async () => {
    render(wrap(<JobsPage />))
    const row = await screen.findByRole('button', { name: /Title a at Acme, status new/i })
    expect(row).toBeInTheDocument()
  })

  it('REGRESSION: clicking bulk dismiss opens the modal (no longer silent)', async () => {
    render(wrap(<JobsPage />))
    // Wait for rows then check one.
    await screen.findByRole('button', { name: /Title a at Acme/i })
    const headerCheckbox = screen.getByLabelText('Select all jobs on this page')
    fireEvent.click(headerCheckbox)

    // The bulk action bar should now show "3 selected" + a "Mark not relevant" button.
    expect(await screen.findByText(/3 selected/)).toBeInTheDocument()

    const dismissBtn = screen.getByRole('button', {
      name: /Dismiss 3 selected jobs/i,
    })
    fireEvent.click(dismissBtn)

    // The DismissModal should open in bulk mode.
    expect(await screen.findByText('Dismiss 3 jobs?')).toBeInTheDocument()

    // Confirming should call bulkDismissJobs (not silently — the modal must
    // have opened first). Pick the modal's confirm button (exact text)
    // rather than the bar's button (aria-label "Dismiss 3 selected jobs").
    const dialog = await screen.findByRole('dialog')
    fireEvent.click(within(dialog).getByRole('button', { name: 'Dismiss 3' }))
    await waitFor(() => expect(jobsApi.bulkDismissJobs).toHaveBeenCalledWith(['a', 'b', 'c']))
  })

  it('REGRESSION: bulk save and mark-applied buttons exist in the bulk bar', async () => {
    render(wrap(<JobsPage />))
    await screen.findByRole('button', { name: /Title a at Acme/i })
    fireEvent.click(screen.getByLabelText('Select all jobs on this page'))

    const bar = await screen.findByRole('status')
    expect(within(bar).getByRole('button', { name: /Save 3 selected jobs/ })).toBeInTheDocument()
    expect(
      within(bar).getByRole('button', { name: /Mark 3 selected jobs as applied/ })
    ).toBeInTheDocument()
  })

  it('REGRESSION: bulk action bar is a live region (role=status)', async () => {
    render(wrap(<JobsPage />))
    await screen.findByRole('button', { name: /Title a at Acme/i })
    fireEvent.click(screen.getByLabelText('Select all jobs on this page'))
    expect(await screen.findByRole('status')).toBeInTheDocument()
  })
})
