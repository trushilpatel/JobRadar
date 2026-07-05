import { describe, it, expect, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { ApplyQueueTable } from '../ApplyQueueTable'
import type { ApplyQueueItem } from '../../api/apply-queue.api'

const item: ApplyQueueItem = {
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

const resumeOptions = [{ id: 'resume-1', label: 'Software Engineer Resume' }]

const basePaginationProps = {
  page: 1,
  itemsOnPage: 1,
  itemsBeforePage: 0,
  limit: 20,
  onLimitChange: vi.fn(),
  hasNextPage: false,
  hasPrevPage: false,
  onNext: vi.fn(),
  onPrev: vi.fn(),
}

describe('ApplyQueueTable', () => {
  it('renders a loading skeleton', () => {
    const { container } = render(
      <ApplyQueueTable
        items={[]}
        loading
        resumeOptions={resumeOptions}
        onResumeChange={vi.fn()}
        {...basePaginationProps}
      />
    )
    expect(container.querySelector('[aria-busy="true"]')).not.toBeNull()
    expect(screen.getByRole('table')).toBeInTheDocument()
  })

  it('renders the error state with a working retry button', async () => {
    const user = userEvent.setup()
    const onRetry = vi.fn()
    render(
      <ApplyQueueTable
        items={[]}
        loading={false}
        isError
        onRetry={onRetry}
        resumeOptions={resumeOptions}
        onResumeChange={vi.fn()}
        {...basePaginationProps}
      />
    )
    expect(screen.getByRole('alert')).toHaveTextContent("Couldn't load your apply queue")
    await user.click(screen.getByRole('button', { name: /try again/i }))
    expect(onRetry).toHaveBeenCalled()
  })

  it('renders the empty state message', () => {
    render(
      <ApplyQueueTable
        items={[]}
        loading={false}
        emptyMessage="No jobs in your apply queue yet"
        resumeOptions={resumeOptions}
        onResumeChange={vi.fn()}
        {...basePaginationProps}
      />
    )
    expect(screen.getByText('No jobs in your apply queue yet')).toBeInTheDocument()
  })

  it('renders job title, company, status pill, and resume picker for a populated row', () => {
    render(
      <ApplyQueueTable
        items={[item]}
        loading={false}
        resumeOptions={resumeOptions}
        onResumeChange={vi.fn()}
        {...basePaginationProps}
      />
    )
    expect(screen.getByRole('link', { name: 'Senior Engineer' })).toHaveAttribute(
      'href',
      'https://acme.example/jobs/1'
    )
    expect(screen.getByText('Acme')).toBeInTheDocument()
    expect(screen.getByText('Saved')).toBeInTheDocument()
    // The Pagination "Rows" page-size control is also a native <select>
    // (role="combobox"), so disambiguate via the resume picker's aria-label.
    expect(screen.getByRole('combobox', { name: /select resume/i })).toBeInTheDocument()
  })

  it('wires the pagination controls to the provided handlers', async () => {
    const user = userEvent.setup()
    const onNext = vi.fn()
    render(
      <ApplyQueueTable
        items={[item]}
        loading={false}
        resumeOptions={resumeOptions}
        onResumeChange={vi.fn()}
        {...basePaginationProps}
        hasNextPage
        onNext={onNext}
      />
    )
    await user.click(screen.getByRole('button', { name: /next page/i }))
    expect(onNext).toHaveBeenCalled()
  })
})
