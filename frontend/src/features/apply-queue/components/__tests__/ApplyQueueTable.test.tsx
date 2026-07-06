import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, fireEvent, cleanup } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { ApplyQueueTable } from '../ApplyQueueTable'
import type { ApplyQueueItem } from '../../api/apply-queue.api'
import * as applyQueueApi from '@/features/apply-queue/api/apply-queue.api'

vi.mock('@/features/apply-queue/api/apply-queue.api')
vi.mock('sonner', () => ({ toast: { success: vi.fn(), error: vi.fn() } }))

const item: ApplyQueueItem = {
  job_id: 'job-1',
  job_title: 'Senior Engineer',
  company: 'Acme',
  job_url: 'https://acme.example/jobs/1',
  status: 'saved',
  resume_id: null,
  resume_label: null,
  job_keywords: [],
  location: null,
  remote: false,
  salary_range: null,
  added_at: '2024-01-01T00:00:00Z',
}

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

const baseResumeProps = {
  getEffectiveResume: () => null,
  getMatchSummary: () => null,
  getResumeMatchOptions: () => [],
}

function renderTable(ui: React.ReactElement) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: 0, gcTime: 0 } } })
  return render(<QueryClientProvider client={qc}>{ui}</QueryClientProvider>)
}

describe('ApplyQueueTable', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    cleanup()
  })

  it('renders a loading skeleton', () => {
    const { container } = renderTable(
      <ApplyQueueTable items={[]} loading {...baseResumeProps} {...basePaginationProps} />
    )
    expect(container.querySelector('[aria-busy="true"]')).not.toBeNull()
    expect(screen.getByRole('table')).toBeInTheDocument()
  })

  it('renders the error state with a working retry button', async () => {
    const user = userEvent.setup()
    const onRetry = vi.fn()
    renderTable(
      <ApplyQueueTable
        items={[]}
        loading={false}
        isError
        onRetry={onRetry}
        {...baseResumeProps}
        {...basePaginationProps}
      />
    )
    expect(screen.getByRole('alert')).toHaveTextContent("Couldn't load your apply queue")
    await user.click(screen.getByRole('button', { name: /try again/i }))
    expect(onRetry).toHaveBeenCalled()
  })

  it('renders the empty state message', () => {
    renderTable(
      <ApplyQueueTable
        items={[]}
        loading={false}
        emptyMessage="No jobs in your apply queue yet"
        {...baseResumeProps}
        {...basePaginationProps}
      />
    )
    expect(screen.getByText('No jobs in your apply queue yet')).toBeInTheDocument()
  })

  it('renders job title, company, status pill, and resume trigger for a populated row', () => {
    renderTable(
      <ApplyQueueTable
        items={[item]}
        loading={false}
        {...baseResumeProps}
        {...basePaginationProps}
      />
    )
    expect(screen.getByRole('link', { name: 'Senior Engineer' })).toHaveAttribute(
      'href',
      'https://acme.example/jobs/1'
    )
    expect(screen.getByText('Acme')).toBeInTheDocument()
    expect(screen.getByText('Saved')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /select resume/i })).toBeInTheDocument()
  })

  it('wires the pagination controls to the provided handlers', async () => {
    const user = userEvent.setup()
    const onNext = vi.fn()
    renderTable(
      <ApplyQueueTable
        items={[item]}
        loading={false}
        {...baseResumeProps}
        {...basePaginationProps}
        hasNextPage
        onNext={onNext}
      />
    )
    await user.click(screen.getByRole('button', { name: /next page/i }))
    expect(onNext).toHaveBeenCalled()
  })

  describe('resume-picker dialog wiring', () => {
    it('opens the dialog for the clicked row and shows that job in the title', async () => {
      const user = userEvent.setup()
      renderTable(
        <ApplyQueueTable
          items={[item]}
          loading={false}
          {...baseResumeProps}
          {...basePaginationProps}
        />
      )
      await user.click(screen.getByRole('button', { name: /select resume/i }))
      expect(
        screen.getByRole('heading', { name: /select resume for senior engineer/i })
      ).toBeInTheDocument()
    })

    it('renders only one dialog instance regardless of row count', async () => {
      const user = userEvent.setup()
      const items = [item, { ...item, job_id: 'job-2', job_title: 'Backend Engineer' }]
      renderTable(
        <ApplyQueueTable
          items={items}
          loading={false}
          {...baseResumeProps}
          {...basePaginationProps}
        />
      )
      await user.click(screen.getAllByRole('button', { name: /select resume/i })[0]!)
      expect(screen.getAllByRole('dialog')).toHaveLength(1)
    })

    it("shows job B's data (not stale job A data) when switching active job", async () => {
      const user = userEvent.setup()
      const items = [item, { ...item, job_id: 'job-2', job_title: 'Backend Engineer' }]
      renderTable(
        <ApplyQueueTable
          items={items}
          loading={false}
          {...baseResumeProps}
          {...basePaginationProps}
        />
      )
      const triggers = screen.getAllByRole('button', { name: /select resume/i })
      await user.click(triggers[0]!)
      expect(
        screen.getByRole('heading', { name: /select resume for senior engineer/i })
      ).toBeInTheDocument()
      await user.keyboard('{Escape}')
      await user.click(triggers[1]!)
      expect(
        screen.getByRole('heading', { name: /select resume for backend engineer/i })
      ).toBeInTheDocument()
    })

    it('confirms via the mutation and closes the dialog on success', async () => {
      vi.mocked(applyQueueApi.setApplyQueueResume).mockResolvedValue(undefined)
      const user = userEvent.setup()
      renderTable(
        <ApplyQueueTable
          items={[item]}
          loading={false}
          getEffectiveResume={() => null}
          getMatchSummary={() => null}
          getResumeMatchOptions={() => [
            {
              id: 'resume-1',
              label: 'My Resume',
              matched: 0,
              total: 0,
              matchedKeywords: [],
              missingKeywords: [],
              hasExtractedText: true,
            },
          ]}
          {...basePaginationProps}
        />
      )
      await user.click(screen.getByRole('button', { name: /select resume/i }))
      await user.click(screen.getByText('My Resume'))
      await user.click(screen.getByRole('button', { name: /use this resume/i }))
      expect(applyQueueApi.setApplyQueueResume).toHaveBeenCalledWith('job-1', 'resume-1')
      await screen.findByRole('button', { name: /select resume/i })
      expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
    })
  })

  describe('row click wiring', () => {
    it('does not make rows focusable/clickable when onRowClick is absent', () => {
      renderTable(
        <ApplyQueueTable
          items={[item]}
          loading={false}
          {...baseResumeProps}
          {...basePaginationProps}
        />
      )
      expect(
        screen.queryByRole('button', { name: /Senior Engineer at Acme/i })
      ).not.toBeInTheDocument()
    })

    it('calls onRowClick with the clicked item when a row is clicked', async () => {
      const user = userEvent.setup()
      const onRowClick = vi.fn()
      renderTable(
        <ApplyQueueTable
          items={[item]}
          loading={false}
          {...baseResumeProps}
          onRowClick={onRowClick}
          {...basePaginationProps}
        />
      )
      await user.click(
        screen.getByRole('button', { name: /Senior Engineer at Acme, status saved/i })
      )
      expect(onRowClick).toHaveBeenCalledWith(item)
    })

    it('calls onRowClick on Enter and Space keydown', async () => {
      const onRowClick = vi.fn()
      renderTable(
        <ApplyQueueTable
          items={[item]}
          loading={false}
          {...baseResumeProps}
          onRowClick={onRowClick}
          {...basePaginationProps}
        />
      )
      const row = screen.getByRole('button', { name: /Senior Engineer at Acme, status saved/i })
      fireEvent.keyDown(row, { key: 'Enter' })
      expect(onRowClick).toHaveBeenCalledWith(item)
      fireEvent.keyDown(row, { key: ' ' })
      expect(onRowClick).toHaveBeenCalledTimes(2)
    })

    it('marks the selected row aria-selected when selectedJobId matches', () => {
      renderTable(
        <ApplyQueueTable
          items={[item]}
          loading={false}
          {...baseResumeProps}
          onRowClick={vi.fn()}
          selectedJobId="job-1"
          {...basePaginationProps}
        />
      )
      const row = screen.getByRole('button', { name: /Senior Engineer at Acme, status saved/i })
      expect(row).toHaveAttribute('aria-selected', 'true')
    })
  })
})
