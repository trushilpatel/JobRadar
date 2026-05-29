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
import { render, screen, fireEvent, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { MemoryRouter } from 'react-router-dom'
import { NuqsTestingAdapter } from 'nuqs/adapters/testing'
import type { ReactNode } from 'react'

// ── Module mocks ─────────────────────────────────────────────────────────────
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
vi.mock('@/lib/hooks/use-media-query', () => ({
  useIsMobile: () => false,
  useIsDesktop: () => true,
}))

import * as jobsApi from '@/features/jobs'
import { toast } from 'sonner'
import { JobsPage } from '@/pages/JobsPage'
import { JOB_STATUS } from '@/config/constants/jobStatus'
import { hiddenItems } from '@/lib/hooks/hidden-items'
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
  localStorage.clear()
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
})

describe('JobsPage — regression coverage for audit fixes', () => {
  it('REGRESSION: clicking row 1 then pressing `s` saves row 1, not row 0', async () => {
    render(wrap(<JobsPage />))
    // Click row 'b' so the keyboard `s` shortcut targets it. Under the sparse
    // model, `s` toggles the orthogonal saved flag (setJobSaved), not the
    // workflow status. The regression we guard against here is that the
    // shortcut targets the CLICKED row, not jobs[0].
    const row = await screen.findByRole('button', { name: /Title b at Acme/i })
    fireEvent.click(row)

    fireEvent.keyDown(document.body, { key: 's', code: 'KeyS' })

    await waitFor(() => {
      expect(jobsApi.setJobSaved).toHaveBeenCalledWith('b', true)
    })
    // And NOT row 0.
    const calls = vi.mocked(jobsApi.setJobSaved).mock.calls.map((c) => c[0])
    expect(calls).not.toContain('a')
  })

  it('REGRESSION: keyboard rows expose accessible names for screen readers', async () => {
    render(wrap(<JobsPage />))
    const row = await screen.findByRole('button', { name: /Title a at Acme, status new/i })
    expect(row).toBeInTheDocument()
  })
})

describe('JobsPage — bookmark and dismiss flows', () => {
  it('bookmark button on an unsaved row calls setJobSaved(true) — orthogonal to status', async () => {
    render(wrap(<JobsPage />))
    // Row "a" is unsaved (saved_at = null) → bookmark says "Save Title a"
    const btn = await screen.findByRole('button', { name: /Save Title a/i })
    fireEvent.click(btn)

    await waitFor(() => {
      expect(jobsApi.setJobSaved).toHaveBeenCalledWith('a', true)
    })
    // Workflow status must remain untouched.
    expect(jobsApi.setJobStatus).not.toHaveBeenCalled()
  })

  it('bookmark button on a saved row toggles saved_at to null', async () => {
    // Re-mock so job 'c' has saved_at populated. Status stays as 'new' —
    // saved is orthogonal.
    vi.mocked(jobsApi.fetchJobs).mockResolvedValue({
      items: [
        makeJob('a'),
        makeJob('b'),
        makeJob('c', {
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
      ],
      next_cursor: '',
    })

    render(wrap(<JobsPage />))
    const btn = await screen.findByRole('button', { name: /Remove Title c from saved/i })
    fireEvent.click(btn)

    await waitFor(() => {
      expect(jobsApi.setJobSaved).toHaveBeenCalledWith('c', false)
    })
  })

  it('dismiss button calls dismissJob with default reason and shows DismissStripe toast', async () => {
    render(wrap(<JobsPage />))
    const dismissBtn = await screen.findByRole('button', { name: /Dismiss Title a/i })
    fireEvent.click(dismissBtn)

    await waitFor(() => {
      expect(jobsApi.dismissJob).toHaveBeenCalledWith('a', 'user_dismissed', undefined)
    })

    // The dismiss stripe is rendered via toast.custom — assert it was invoked.
    expect(vi.mocked(toast).custom).toHaveBeenCalled()
  })

  it('hides jobs whose company is in the hidden-items list', async () => {
    hiddenItems.add('companies', 'Acme')
    render(wrap(<JobsPage />))

    // All three jobs were from "Acme" — none should render
    await waitFor(() => {
      expect(screen.queryByRole('button', { name: /Title a at Acme/i })).not.toBeInTheDocument()
      expect(screen.queryByRole('button', { name: /Title b at Acme/i })).not.toBeInTheDocument()
      expect(screen.queryByRole('button', { name: /Title c at Acme/i })).not.toBeInTheDocument()
    })
  })

  it('renders normally when no companies are hidden', async () => {
    render(wrap(<JobsPage />))
    expect(await screen.findByRole('button', { name: /Title a at Acme/i })).toBeInTheDocument()
    expect(await screen.findByRole('button', { name: /Title b at Acme/i })).toBeInTheDocument()
  })
})
