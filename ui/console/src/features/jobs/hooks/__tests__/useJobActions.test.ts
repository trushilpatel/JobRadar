import { describe, it, expect, vi, beforeEach } from 'vitest'
import { renderHook, waitFor, act } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { createElement } from 'react'
import * as jobsApi from '@/api/jobs'
import { useJobActions } from '../useJobActions'
import { jobQueryKeys } from '@/features/jobs/queryKeys'
import { JOB_STATUS } from '@/constants/jobStatus'
import type { Job } from '@/types'

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

import { toast } from 'sonner'

let lastQc: QueryClient | null = null

function makeWrapper() {
  // gcTime must be > 0 in tests that seed the cache directly — otherwise
  // inactive (un-observed) queries are gc'd the moment TanStack walks the
  // cache, and our `seedListCache` rows vanish before onMutate can read them.
  const qc = new QueryClient({
    defaultOptions: {
      queries: { retry: 0, gcTime: 5 * 60_000 },
      mutations: { retry: 0 },
    },
  })
  lastQc = qc
  return ({ children }: { children: React.ReactNode }) =>
    createElement(QueryClientProvider, { client: qc }, children)
}

function makeJob(id: string, status: Job['status'] = JOB_STATUS.NEW): Job {
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
    status,
    saved: false,
    notes: null,
    user_state: {
      status,
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
  }
}

function seedListCache(jobs: Job[]) {
  const filters = { status: JOB_STATUS.NEW } as jobsApi.JobFilters
  lastQc?.setQueryData(jobQueryKeys.list(filters, 20), {
    pages: [{ items: jobs, next_cursor: '' }],
    pageParams: [''],
  })
}

describe('useJobActions', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  describe('setStatus', () => {
    it('calls setJobStatus with id and status when status is saved', async () => {
      vi.mocked(jobsApi.setJobStatus).mockResolvedValue(undefined)
      const { result } = renderHook(() => useJobActions(), { wrapper: makeWrapper() })
      await act(async () => {
        await result.current.setStatus.mutateAsync({ id: 'j1', status: JOB_STATUS.SAVED })
      })
      expect(jobsApi.setJobStatus).toHaveBeenCalledWith('j1', JOB_STATUS.SAVED)
      expect(jobsApi.setJobSaved).not.toHaveBeenCalled()
    })

    it('calls setJobSaved(false) when status is "new"', async () => {
      vi.mocked(jobsApi.setJobSaved).mockResolvedValue(undefined)
      const { result } = renderHook(() => useJobActions(), { wrapper: makeWrapper() })
      await act(async () => {
        await result.current.setStatus.mutateAsync({ id: 'j1', status: JOB_STATUS.NEW })
      })
      expect(jobsApi.setJobSaved).toHaveBeenCalledWith('j1', false)
      expect(jobsApi.setJobStatus).not.toHaveBeenCalled()
    })

    it('shows "Moved back to New" undo toast when status is new', async () => {
      vi.mocked(jobsApi.setJobSaved).mockResolvedValue(undefined)
      const { result } = renderHook(() => useJobActions(), { wrapper: makeWrapper() })
      await act(async () => {
        await result.current.setStatus.mutateAsync({ id: 'j1', status: JOB_STATUS.NEW })
      })
      expect(toast).toHaveBeenCalledWith(
        'Moved back to New',
        expect.objectContaining({
          action: expect.objectContaining({ label: 'Undo' }),
        })
      )
    })

    it('shows "Moved to saved" undo toast when status is saved', async () => {
      vi.mocked(jobsApi.setJobStatus).mockResolvedValue(undefined)
      const { result } = renderHook(() => useJobActions(), { wrapper: makeWrapper() })
      await act(async () => {
        await result.current.setStatus.mutateAsync({ id: 'j1', status: JOB_STATUS.SAVED })
      })
      expect(toast).toHaveBeenCalledWith('Moved to saved', expect.any(Object))
    })

    it('shows "Moved to applied" undo toast when status is applied', async () => {
      vi.mocked(jobsApi.setJobStatus).mockResolvedValue(undefined)
      const { result } = renderHook(() => useJobActions(), { wrapper: makeWrapper() })
      await act(async () => {
        await result.current.setStatus.mutateAsync({ id: 'j1', status: JOB_STATUS.APPLIED })
      })
      expect(toast).toHaveBeenCalledWith('Moved to applied', expect.any(Object))
    })

    it('shows error toast when setJobStatus fails', async () => {
      vi.mocked(jobsApi.setJobStatus).mockRejectedValue(new Error('Server error'))
      const { result } = renderHook(() => useJobActions(), { wrapper: makeWrapper() })
      await act(async () => {
        try {
          await result.current.setStatus.mutateAsync({ id: 'j1', status: JOB_STATUS.SAVED })
        } catch {
          // expected
        }
      })
      await waitFor(() => expect(toast.error).toHaveBeenCalledWith('Failed to update status'))
    })

    it('shows error toast when setJobSaved fails', async () => {
      vi.mocked(jobsApi.setJobSaved).mockRejectedValue(new Error('Server error'))
      const { result } = renderHook(() => useJobActions(), { wrapper: makeWrapper() })
      await act(async () => {
        try {
          await result.current.setStatus.mutateAsync({ id: 'j1', status: JOB_STATUS.NEW })
        } catch {
          // expected
        }
      })
      await waitFor(() => expect(toast.error).toHaveBeenCalledWith('Failed to update status'))
    })
  })

  describe('dismiss', () => {
    it('calls dismissJob with id, reason, and optional note', async () => {
      vi.mocked(jobsApi.dismissJob).mockResolvedValue(undefined)
      const { result } = renderHook(() => useJobActions(), { wrapper: makeWrapper() })
      await act(async () => {
        await result.current.dismiss.mutateAsync({
          id: 'j1',
          reason: 'user_dismissed',
          note: 'Not relevant',
        })
      })
      expect(jobsApi.dismissJob).toHaveBeenCalledWith('j1', 'user_dismissed', 'Not relevant')
    })

    it('calls dismissJob without note when not provided', async () => {
      vi.mocked(jobsApi.dismissJob).mockResolvedValue(undefined)
      const { result } = renderHook(() => useJobActions(), { wrapper: makeWrapper() })
      await act(async () => {
        await result.current.dismiss.mutateAsync({ id: 'j2', reason: 'user_not_relevant' })
      })
      expect(jobsApi.dismissJob).toHaveBeenCalledWith('j2', 'user_not_relevant', undefined)
    })

    it('shows "Job dismissed" undo toast on success', async () => {
      vi.mocked(jobsApi.dismissJob).mockResolvedValue(undefined)
      const { result } = renderHook(() => useJobActions(), { wrapper: makeWrapper() })
      await act(async () => {
        await result.current.dismiss.mutateAsync({ id: 'j1', reason: 'user_dismissed' })
      })
      expect(toast).toHaveBeenCalledWith('Job dismissed', expect.any(Object))
    })

    it('shows error toast when dismissJob fails', async () => {
      vi.mocked(jobsApi.dismissJob).mockRejectedValue(new Error('Server error'))
      const { result } = renderHook(() => useJobActions(), { wrapper: makeWrapper() })
      await act(async () => {
        try {
          await result.current.dismiss.mutateAsync({ id: 'j1', reason: 'user_dismissed' })
        } catch {
          // expected
        }
      })
      await waitFor(() => expect(toast.error).toHaveBeenCalledWith('Failed to dismiss job'))
    })
  })

  describe('bulkDismiss', () => {
    it('calls bulkDismissJobs with ids array', async () => {
      vi.mocked(jobsApi.bulkDismissJobs).mockResolvedValue({ dismissed: 3 })
      const { result } = renderHook(() => useJobActions(), { wrapper: makeWrapper() })
      await act(async () => {
        await result.current.bulkDismiss.mutateAsync(['j1', 'j2', 'j3'])
      })
      expect(jobsApi.bulkDismissJobs).toHaveBeenCalledWith(['j1', 'j2', 'j3'])
    })

    it('shows plural "jobs dismissed" undo toast when count > 1', async () => {
      vi.mocked(jobsApi.bulkDismissJobs).mockResolvedValue({ dismissed: 3 })
      const { result } = renderHook(() => useJobActions(), { wrapper: makeWrapper() })
      await act(async () => {
        await result.current.bulkDismiss.mutateAsync(['j1', 'j2', 'j3'])
      })
      expect(toast).toHaveBeenCalledWith('3 jobs dismissed', expect.any(Object))
    })

    it('shows singular "job dismissed" undo toast when count is 1', async () => {
      vi.mocked(jobsApi.bulkDismissJobs).mockResolvedValue({ dismissed: 1 })
      const { result } = renderHook(() => useJobActions(), { wrapper: makeWrapper() })
      await act(async () => {
        await result.current.bulkDismiss.mutateAsync(['j1'])
      })
      expect(toast).toHaveBeenCalledWith('1 job dismissed', expect.any(Object))
    })

    it('shows error toast when bulkDismissJobs fails', async () => {
      vi.mocked(jobsApi.bulkDismissJobs).mockRejectedValue(new Error('Server error'))
      const { result } = renderHook(() => useJobActions(), { wrapper: makeWrapper() })
      await act(async () => {
        try {
          await result.current.bulkDismiss.mutateAsync(['j1'])
        } catch {
          // expected
        }
      })
      await waitFor(() => expect(toast.error).toHaveBeenCalledWith('Failed to dismiss jobs'))
    })
  })

  describe('bulkSave', () => {
    it('calls setJobStatus once per id with saved', async () => {
      vi.mocked(jobsApi.setJobStatus).mockResolvedValue(undefined)
      const { result } = renderHook(() => useJobActions(), { wrapper: makeWrapper() })
      await act(async () => {
        await result.current.bulkSave.mutateAsync(['j1', 'j2'])
      })
      expect(jobsApi.setJobStatus).toHaveBeenCalledTimes(2)
      expect(jobsApi.setJobStatus).toHaveBeenCalledWith('j1', JOB_STATUS.SAVED)
      expect(jobsApi.setJobStatus).toHaveBeenCalledWith('j2', JOB_STATUS.SAVED)
    })

    it('shows undo toast reporting all succeeded when no failures', async () => {
      vi.mocked(jobsApi.setJobStatus).mockResolvedValue(undefined)
      const { result } = renderHook(() => useJobActions(), { wrapper: makeWrapper() })
      await act(async () => {
        await result.current.bulkSave.mutateAsync(['j1', 'j2'])
      })
      expect(toast).toHaveBeenCalledWith('2 jobs saved', expect.any(Object))
    })

    it('reports partial failure in the toast', async () => {
      vi.mocked(jobsApi.setJobStatus).mockImplementation((id) => {
        return id === 'j1' ? Promise.resolve() : Promise.reject(new Error('fail'))
      })
      const { result } = renderHook(() => useJobActions(), { wrapper: makeWrapper() })
      await act(async () => {
        await result.current.bulkSave.mutateAsync(['j1', 'j2'])
      })
      expect(toast).toHaveBeenCalledWith('1 of 2 jobs saved (1 failed)', expect.any(Object))
    })
  })

  describe('bulkApplied', () => {
    it('calls setJobStatus once per id with applied', async () => {
      vi.mocked(jobsApi.setJobStatus).mockResolvedValue(undefined)
      const { result } = renderHook(() => useJobActions(), { wrapper: makeWrapper() })
      await act(async () => {
        await result.current.bulkApplied.mutateAsync(['j1', 'j2'])
      })
      expect(jobsApi.setJobStatus).toHaveBeenCalledWith('j1', JOB_STATUS.APPLIED)
      expect(jobsApi.setJobStatus).toHaveBeenCalledWith('j2', JOB_STATUS.APPLIED)
    })
  })

  describe('undoLast', () => {
    it('reverts the most recent status change', async () => {
      vi.mocked(jobsApi.setJobStatus).mockResolvedValue(undefined)
      const { result } = renderHook(() => useJobActions(), { wrapper: makeWrapper() })
      await act(async () => {
        await result.current.setStatus.mutateAsync({ id: 'j1', status: JOB_STATUS.SAVED })
      })
      vi.mocked(jobsApi.setJobStatus).mockClear()
      vi.mocked(jobsApi.setJobSaved).mockClear()
      await act(async () => {
        await result.current.undoLast()
      })
      // Prior status was JOB_STATUS.NEW (default) — undo calls setJobSaved(id, false)
      expect(jobsApi.setJobSaved).toHaveBeenCalledWith('j1', false)
    })

    it('shows a "Nothing to undo" warning when history is empty', async () => {
      const { result } = renderHook(() => useJobActions(), { wrapper: makeWrapper() })
      await act(async () => {
        await result.current.undoLast()
      })
      expect(jobsApi.setJobStatus).not.toHaveBeenCalled()
      expect(jobsApi.setJobSaved).not.toHaveBeenCalled()
      expect(toast.warning).toHaveBeenCalledWith('Nothing to undo')
    })

    it('REGRESSION: undo reverts to the correct prior status when the cache is populated', async () => {
      // Headline regression test for the lookupStatus query-key bug.
      // Old behavior: every undo silently fell back to NEW because the
      // `['jobs', 'list']` predicate never matched the actual write key.
      vi.mocked(jobsApi.setJobStatus).mockResolvedValue(undefined)
      const { result } = renderHook(() => useJobActions(), { wrapper: makeWrapper() })

      // Seed the cache with the job already in `saved` state.
      seedListCache([makeJob('j1', JOB_STATUS.SAVED)])

      // Promote saved → applied.
      await act(async () => {
        await result.current.setStatus.mutateAsync({ id: 'j1', status: JOB_STATUS.APPLIED })
      })

      vi.mocked(jobsApi.setJobStatus).mockClear()
      vi.mocked(jobsApi.setJobSaved).mockClear()

      await act(async () => {
        await result.current.undoLast()
      })

      // MUST revert to 'saved', not to NEW.
      expect(jobsApi.setJobStatus).toHaveBeenCalledWith('j1', JOB_STATUS.SAVED)
      expect(jobsApi.setJobSaved).not.toHaveBeenCalled()
    })
  })

  describe('cache + concurrency', () => {
    it('caps the undo stack at 20 entries', async () => {
      vi.mocked(jobsApi.setJobStatus).mockResolvedValue(undefined)
      const { result } = renderHook(() => useJobActions(), { wrapper: makeWrapper() })
      // Push 25 entries.
      for (let i = 0; i < 25; i++) {
        await act(async () => {
          await result.current.setStatus.mutateAsync({ id: `j${i}`, status: JOB_STATUS.SAVED })
        })
      }
      // Pop them all by undoing 25 times; only 20 should result in calls.
      vi.mocked(jobsApi.setJobSaved).mockClear()
      for (let i = 0; i < 25; i++) {
        await act(async () => {
          await result.current.undoLast()
        })
      }
      // The 20 most recent ids were j5..j24; 25 - 20 = 5 oldest were dropped.
      expect(jobsApi.setJobSaved).toHaveBeenCalledTimes(20)
      // The first undo after the cap pops the most recent (j24), the last pops
      // j5; everything older has been dropped.
      const calls = vi.mocked(jobsApi.setJobSaved).mock.calls
      const calledIds = calls.map((c) => c[0])
      expect(calledIds[0]).toBe('j24')
      expect(calledIds[19]).toBe('j5')
    })

    it('throttles bulk mutations to at most 5 concurrent', async () => {
      let inFlight = 0
      let peak = 0
      vi.mocked(jobsApi.setJobStatus).mockImplementation(async () => {
        inFlight += 1
        if (inFlight > peak) peak = inFlight
        await new Promise((r) => setTimeout(r, 10))
        inFlight -= 1
      })
      const { result } = renderHook(() => useJobActions(), { wrapper: makeWrapper() })
      const ids = Array.from({ length: 20 }, (_, i) => `j${i}`)
      await act(async () => {
        await result.current.bulkSave.mutateAsync(ids)
      })
      expect(peak).toBeLessThanOrEqual(5)
      expect(jobsApi.setJobStatus).toHaveBeenCalledTimes(20)
    })

    it('invalidates dashboard + insights along with jobs on mutation', async () => {
      vi.mocked(jobsApi.setJobStatus).mockResolvedValue(undefined)
      const { result } = renderHook(() => useJobActions(), { wrapper: makeWrapper() })
      const invalidateSpy = vi.spyOn(lastQc!, 'invalidateQueries')
      await act(async () => {
        await result.current.setStatus.mutateAsync({ id: 'j1', status: JOB_STATUS.SAVED })
      })
      const calledKeys = invalidateSpy.mock.calls.map((c) => c[0]?.queryKey)
      expect(calledKeys).toContainEqual(['jobs'])
      expect(calledKeys).toContainEqual(['dashboard'])
      expect(calledKeys).toContainEqual(['insights'])
    })

    it('optimistically patches the list cache before the mutation resolves', async () => {
      let resolveFn: (() => void) | undefined
      vi.mocked(jobsApi.setJobStatus).mockImplementation(
        () => new Promise<void>((res) => (resolveFn = res))
      )
      const { result } = renderHook(() => useJobActions(), { wrapper: makeWrapper() })
      seedListCache([makeJob('j1', JOB_STATUS.NEW)])

      // Kick off mutation; do NOT wait for it (it never resolves until we
      // call resolveFn).
      const mutatePromise = result.current.setStatus.mutateAsync({
        id: 'j1',
        status: JOB_STATUS.SAVED,
      })

      // Wait for the optimistic onMutate hook to finish patching the cache.
      await waitFor(() => {
        const cached = lastQc!.getQueryData<{ pages: jobsApi.JobsPage[] }>(
          jobQueryKeys.list({ status: JOB_STATUS.NEW }, 20)
        )
        expect(cached?.pages[0].items[0].status).toBe(JOB_STATUS.SAVED)
      })

      // Resolve and tidy up so the test exits cleanly.
      await act(async () => {
        resolveFn!()
        await mutatePromise
      })
    })

    it('rolls back optimistic patch when the mutation fails', async () => {
      vi.mocked(jobsApi.setJobStatus).mockRejectedValue(new Error('Server error'))
      const { result } = renderHook(() => useJobActions(), { wrapper: makeWrapper() })
      seedListCache([makeJob('j1', JOB_STATUS.NEW)])

      await act(async () => {
        try {
          await result.current.setStatus.mutateAsync({ id: 'j1', status: JOB_STATUS.SAVED })
        } catch {
          // expected
        }
      })

      // Cache should have rolled back to NEW.
      const cached = lastQc!.getQueryData<{ pages: jobsApi.JobsPage[] }>(
        jobQueryKeys.list({ status: JOB_STATUS.NEW }, 20)
      )
      expect(cached?.pages[0].items[0].status).toBe(JOB_STATUS.NEW)
    })
  })

  describe('return shape', () => {
    it('returns all mutation handlers + undoLast', () => {
      const { result } = renderHook(() => useJobActions(), { wrapper: makeWrapper() })
      expect(result.current.setStatus).toBeDefined()
      expect(result.current.dismiss).toBeDefined()
      expect(result.current.bulkDismiss).toBeDefined()
      expect(result.current.bulkSave).toBeDefined()
      expect(result.current.bulkApplied).toBeDefined()
      expect(typeof result.current.undoLast).toBe('function')
    })

    it('mutations start in idle state', () => {
      const { result } = renderHook(() => useJobActions(), { wrapper: makeWrapper() })
      expect(result.current.setStatus.isPending).toBe(false)
      expect(result.current.dismiss.isPending).toBe(false)
      expect(result.current.bulkDismiss.isPending).toBe(false)
      expect(result.current.bulkSave.isPending).toBe(false)
      expect(result.current.bulkApplied.isPending).toBe(false)
    })
  })
})
