import { describe, it, expect, vi, beforeEach } from 'vitest'
import { renderHook, waitFor, act } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { createElement } from 'react'
import * as jobsApi from '@/api/jobs'
import { useJobActions } from '../useJobActions'
import { JOB_STATUS } from '@/constants/jobStatus'

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

function makeWrapper() {
  const qc = new QueryClient({
    defaultOptions: { queries: { retry: 0, gcTime: 0 }, mutations: { retry: 0 } },
  })
  return ({ children }: { children: React.ReactNode }) =>
    createElement(QueryClientProvider, { client: qc }, children)
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

    it('is a no-op when history is empty', async () => {
      const { result } = renderHook(() => useJobActions(), { wrapper: makeWrapper() })
      await act(async () => {
        await result.current.undoLast()
      })
      expect(jobsApi.setJobStatus).not.toHaveBeenCalled()
      expect(jobsApi.setJobSaved).not.toHaveBeenCalled()
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
