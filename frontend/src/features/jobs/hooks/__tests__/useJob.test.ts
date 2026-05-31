import { describe, it, expect, vi, beforeEach } from 'vitest'
import { renderHook, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { createElement } from 'react'
import * as jobsApi from '@/features/jobs/api/jobs.api'
import { useJob } from '../useJob'
import { JOB_STATUS } from '@/config/constants/jobStatus'
import type { Job } from '@/types'

vi.mock('@/features/jobs/api/jobs.api')

const mockJob: Job = {
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
}

function makeWrapper() {
  const qc = new QueryClient({
    defaultOptions: { queries: { retry: 0, gcTime: 0 } },
  })
  return ({ children }: { children: React.ReactNode }) =>
    createElement(QueryClientProvider, { client: qc }, children)
}

describe('useJob', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('fetches and returns the job when id is provided', async () => {
    vi.mocked(jobsApi.fetchJob).mockResolvedValue(mockJob)
    const { result } = renderHook(() => useJob('j1'), { wrapper: makeWrapper() })
    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expect(result.current.data?.id).toBe('j1')
    expect(jobsApi.fetchJob).toHaveBeenCalledWith('j1')
  })

  it('is disabled (no fetch) when id is undefined', () => {
    const { result } = renderHook(() => useJob(undefined), { wrapper: makeWrapper() })
    expect(result.current.fetchStatus).toBe('idle')
    expect(result.current.data).toBeUndefined()
    expect(jobsApi.fetchJob).not.toHaveBeenCalled()
  })

  it('is disabled when id is an empty string', () => {
    const { result } = renderHook(() => useJob(''), { wrapper: makeWrapper() })
    expect(result.current.fetchStatus).toBe('idle')
    expect(jobsApi.fetchJob).not.toHaveBeenCalled()
  })

  it('propagates an error when the fetch rejects', async () => {
    vi.mocked(jobsApi.fetchJob).mockRejectedValue(new Error('boom'))
    const { result } = renderHook(() => useJob('j1'), { wrapper: makeWrapper() })
    await waitFor(() => expect(result.current.isError).toBe(true))
    expect(result.current.error).toBeInstanceOf(Error)
    expect((result.current.error as Error).message).toBe('boom')
  })
})
