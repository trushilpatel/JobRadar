import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, cleanup } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { JobDetailPanel } from '../JobDetailPanel'
import type { Job } from '@/types'
import { JOB_STATUS } from '@/constants/jobStatus'

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
} as unknown as Job

beforeEach(() => {
  cleanup()
})

describe('JobDetailPanel', () => {
  it('renders title, company, and tech stack in pane mode', () => {
    render(
      <JobDetailPanel
        job={SAMPLE_JOB}
        onClose={() => {}}
        onStatusChange={() => {}}
        onDismiss={() => {}}
      />
    )
    expect(screen.getByText('Senior Engineer')).toBeInTheDocument()
    expect(screen.getByText('Anthropic')).toBeInTheDocument()
    expect(screen.getByText('python')).toBeInTheDocument()
  })

  it('renders X close button in pane mode and calls onClose', async () => {
    const onClose = vi.fn()
    render(
      <JobDetailPanel
        job={SAMPLE_JOB}
        onClose={onClose}
        onStatusChange={() => {}}
        onDismiss={() => {}}
        mode="pane"
      />
    )
    await userEvent.click(screen.getByRole('button', { name: /close/i }))
    expect(onClose).toHaveBeenCalled()
  })

  it('renders Back button in route mode and calls onClose', async () => {
    const onClose = vi.fn()
    render(
      <JobDetailPanel
        job={SAMPLE_JOB}
        onClose={onClose}
        onStatusChange={() => {}}
        onDismiss={() => {}}
        mode="route"
      />
    )
    await userEvent.click(screen.getByRole('button', { name: /back/i }))
    expect(onClose).toHaveBeenCalled()
  })

  it('shows sticky action bar in route mode with Save, Applied, Dismiss', () => {
    render(
      <JobDetailPanel
        job={SAMPLE_JOB}
        onClose={() => {}}
        onStatusChange={() => {}}
        onDismiss={() => {}}
        mode="route"
      />
    )
    expect(screen.getByRole('button', { name: /^save$/i })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /^applied$/i })).toBeInTheDocument()
    // Two Dismiss buttons in route mode: header icon + footer; just ensure at least one
    expect(screen.getAllByRole('button', { name: /dismiss/i }).length).toBeGreaterThan(0)
  })

  it('clicking Save in route mode calls onStatusChange with saved', async () => {
    const onStatusChange = vi.fn()
    render(
      <JobDetailPanel
        job={SAMPLE_JOB}
        onClose={() => {}}
        onStatusChange={onStatusChange}
        onDismiss={() => {}}
        mode="route"
      />
    )
    await userEvent.click(screen.getByRole('button', { name: /^save$/i }))
    expect(onStatusChange).toHaveBeenCalledWith(SAMPLE_JOB, JOB_STATUS.SAVED)
  })
})
