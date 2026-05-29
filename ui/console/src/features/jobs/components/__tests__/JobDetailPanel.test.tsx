import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, cleanup, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { MemoryRouter } from 'react-router-dom'
import { createElement, type ReactNode } from 'react'
import { JobDetailPanel } from '../JobDetailPanel'
import type { Job } from '@/types'
import { JOB_STATUS } from '@/config/constants/jobStatus'
import * as jobsApi from '@/features/jobs/api/jobs.api'

vi.mock('@/features/jobs/api/jobs.api')
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
import { toast } from 'sonner'

const SAMPLE_JOB: Job = {
  id: 'j1',
  title: 'Senior Engineer',
  company_id: 'c1',
  company: { id: 'c1', name: 'Anthropic', slug: 'anthropic', adapter: 'manual' },
  url: 'https://example.com/jobs/1',
  remote: true,
  location: null,
  added_at: new Date().toISOString(),
  salary_range: null,
  experience_years: 5,
  employ_type: 'full_time',
  status: JOB_STATUS.NEW,
  tech_stack: ['python'],
  responsibilities: ['Build systems'],
  role_brief: 'Inference work',
  llm_summary: null,
  notes: null,
} as unknown as Job

function wrap(ui: ReactNode) {
  const qc = new QueryClient({
    defaultOptions: { queries: { retry: 0 }, mutations: { retry: 0 } },
  })
  // Wrap in MemoryRouter — any injected resumeSection slot may use
  // react-router hooks (Link).
  return createElement(MemoryRouter, null, createElement(QueryClientProvider, { client: qc }, ui))
}

beforeEach(() => {
  cleanup()
  vi.clearAllMocks()
})

describe('JobDetailPanel', () => {
  it('renders title, company, and tech stack in pane mode', () => {
    render(
      wrap(
        <JobDetailPanel
          job={SAMPLE_JOB}
          onClose={() => {}}
          onStatusChange={() => {}}
          onToggleSave={() => {}}
          onDismiss={() => {}}
        />
      )
    )
    expect(screen.getByText('Senior Engineer')).toBeInTheDocument()
    expect(screen.getByText('Anthropic')).toBeInTheDocument()
    expect(screen.getByText('python')).toBeInTheDocument()
  })

  it('exposes the panel via role="region" with aria-label "Job details"', () => {
    render(
      wrap(
        <JobDetailPanel
          job={SAMPLE_JOB}
          onClose={() => {}}
          onStatusChange={() => {}}
          onToggleSave={() => {}}
          onDismiss={() => {}}
        />
      )
    )
    expect(screen.getByRole('region', { name: 'Job details' })).toBeInTheDocument()
  })

  it('renders X close button in pane mode and calls onClose', async () => {
    const onClose = vi.fn()
    render(
      wrap(
        <JobDetailPanel
          job={SAMPLE_JOB}
          onClose={onClose}
          onStatusChange={() => {}}
          onToggleSave={() => {}}
          onDismiss={() => {}}
          mode="pane"
        />
      )
    )
    await userEvent.click(screen.getByRole('button', { name: /close/i }))
    expect(onClose).toHaveBeenCalled()
  })

  it('renders Back button in route mode and calls onClose', async () => {
    const onClose = vi.fn()
    render(
      wrap(
        <JobDetailPanel
          job={SAMPLE_JOB}
          onClose={onClose}
          onStatusChange={() => {}}
          onToggleSave={() => {}}
          onDismiss={() => {}}
          mode="route"
        />
      )
    )
    await userEvent.click(screen.getByRole('button', { name: /back/i }))
    expect(onClose).toHaveBeenCalled()
  })

  it('shows sticky action bar in route mode with Save, Applied, Dismiss', () => {
    render(
      wrap(
        <JobDetailPanel
          job={SAMPLE_JOB}
          onClose={() => {}}
          onStatusChange={() => {}}
          onToggleSave={() => {}}
          onDismiss={() => {}}
          mode="route"
        />
      )
    )
    expect(screen.getByRole('button', { name: /^save$/i })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /^applied$/i })).toBeInTheDocument()
    expect(screen.getAllByRole('button', { name: /dismiss/i }).length).toBeGreaterThan(0)
  })

  it('clicking Save in route mode calls onToggleSave (orthogonal flag, not status)', async () => {
    const onStatusChange = vi.fn()
    const onToggleSave = vi.fn()
    render(
      wrap(
        <JobDetailPanel
          job={SAMPLE_JOB}
          onClose={() => {}}
          onStatusChange={onStatusChange}
          onToggleSave={onToggleSave}
          onDismiss={() => {}}
          mode="route"
        />
      )
    )
    await userEvent.click(screen.getByRole('button', { name: /^save$/i }))
    expect(onToggleSave).toHaveBeenCalledWith(SAMPLE_JOB)
    // Workflow status untouched — saved is its own concept.
    expect(onStatusChange).not.toHaveBeenCalled()
  })

  describe('mobile route mode reaches all statuses', () => {
    it('renders StatusSelect in route mode (so Interviewing / Offer / Rejected are reachable)', () => {
      render(
        wrap(
          <JobDetailPanel
            job={SAMPLE_JOB}
            onClose={() => {}}
            onStatusChange={() => {}}
            onToggleSave={() => {}}
            onDismiss={() => {}}
            mode="route"
          />
        )
      )
      // Radix Select trigger is a button labelled with the current value.
      // StatusSelect renders a combobox — expect at least one to exist.
      expect(screen.getAllByRole('combobox').length).toBeGreaterThan(0)
    })
  })

  describe('notes UI', () => {
    it('renders the notes textarea seeded from job.notes', () => {
      render(
        wrap(
          <JobDetailPanel
            job={{ ...SAMPLE_JOB, notes: 'pre-existing note' } as Job}
            onClose={() => {}}
            onStatusChange={() => {}}
            onToggleSave={() => {}}
            onDismiss={() => {}}
          />
        )
      )
      expect(screen.getByLabelText('Your notes')).toHaveValue('pre-existing note')
    })

    it('saves on blur via setJobNotes', async () => {
      vi.mocked(jobsApi.setJobNotes).mockResolvedValue(undefined)
      render(
        wrap(
          <JobDetailPanel
            job={SAMPLE_JOB}
            onClose={() => {}}
            onStatusChange={() => {}}
            onToggleSave={() => {}}
            onDismiss={() => {}}
          />
        )
      )
      const ta = screen.getByLabelText('Your notes')
      await userEvent.type(ta, 'follow-up next week')
      ta.blur()
      await waitFor(() =>
        expect(jobsApi.setJobNotes).toHaveBeenCalledWith('j1', 'follow-up next week')
      )
    })

    it('does not save unchanged content', async () => {
      vi.mocked(jobsApi.setJobNotes).mockResolvedValue(undefined)
      render(
        wrap(
          <JobDetailPanel
            job={{ ...SAMPLE_JOB, notes: 'same' } as Job}
            onClose={() => {}}
            onStatusChange={() => {}}
            onToggleSave={() => {}}
            onDismiss={() => {}}
          />
        )
      )
      const ta = screen.getByLabelText('Your notes')
      ta.blur()
      // Nothing changed → no save call.
      expect(jobsApi.setJobNotes).not.toHaveBeenCalled()
    })

    it('shows a toast and announces failure when save errors', async () => {
      vi.mocked(jobsApi.setJobNotes).mockRejectedValue(new Error('500'))
      render(
        wrap(
          <JobDetailPanel
            job={SAMPLE_JOB}
            onClose={() => {}}
            onStatusChange={() => {}}
            onToggleSave={() => {}}
            onDismiss={() => {}}
          />
        )
      )
      const ta = screen.getByLabelText('Your notes')
      await userEvent.type(ta, 'x')
      ta.blur()
      await waitFor(() => expect(toast.error).toHaveBeenCalledWith('Failed to save notes'))
    })
  })

  describe('safe-area on the sticky bar', () => {
    it('applies a safe-area-inset-bottom padding in route mode', () => {
      const { container } = render(
        wrap(
          <JobDetailPanel
            job={SAMPLE_JOB}
            onClose={() => {}}
            onStatusChange={() => {}}
            onToggleSave={() => {}}
            onDismiss={() => {}}
            mode="route"
          />
        )
      )
      const stickyBar = container.querySelector('div.sticky.bottom-0') as HTMLElement | null
      expect(stickyBar).not.toBeNull()
      // Padding is applied via Tailwind class; JSDOM does not compute class-based
      // styles into element.style, so we assert the class attribute directly.
      expect(stickyBar?.className).toContain('safe-area-inset-bottom')
    })
  })
})
