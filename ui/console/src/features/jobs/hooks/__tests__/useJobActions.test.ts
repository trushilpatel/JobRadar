import { describe, it, expect, vi, beforeEach } from 'vitest'
import { renderHook, waitFor, act } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { createElement } from 'react'
import * as jobsApi from '@/api/jobs'
import { useJobActions } from '../useJobActions'
import { JOB_STATUS } from '@/constants/jobStatus'

// Mock API module and toast
vi.mock('@/api/jobs')
vi.mock('sonner', () => ({
  toast: {
    success: vi.fn(),
    error: vi.fn(),
    warning: vi.fn(),
  },
}))

import { toast } from 'sonner'

// ── Wrapper ──────────────────────────────────────────────────────────────────

function makeWrapper() {
  const qc = new QueryClient({
    defaultOptions: { queries: { retry: 0, gcTime: 0 }, mutations: { retry: 0 } },
  })
  return ({ children }: { children: React.ReactNode }) =>
    createElement(QueryClientProvider, { client: qc }, children)
}

// ── Tests ─────────────────────────────────────────────────────────────────────

describe('useJobActions', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  // ── setStatus ─────────────────────────────────────────────────────────────

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

    it('calls setJobSaved(false) when status is "new" (move back to new)', async () => {
      vi.mocked(jobsApi.setJobSaved).mockResolvedValue(undefined)
      const { result } = renderHook(() => useJobActions(), { wrapper: makeWrapper() })
      await act(async () => {
        await result.current.setStatus.mutateAsync({ id: 'j1', status: JOB_STATUS.NEW })
      })
      expect(jobsApi.setJobSaved).toHaveBeenCalledWith('j1', false)
      expect(jobsApi.setJobStatus).not.toHaveBeenCalled()
    })

    it('shows "Moved back to New" toast when status is new', async () => {
      vi.mocked(jobsApi.setJobSaved).mockResolvedValue(undefined)
      const { result } = renderHook(() => useJobActions(), { wrapper: makeWrapper() })
      await act(async () => {
        await result.current.setStatus.mutateAsync({ id: 'j1', status: JOB_STATUS.NEW })
      })
      expect(toast.success).toHaveBeenCalledWith('Moved back to New')
    })

    it('shows "Moved to saved" toast when status is saved', async () => {
      vi.mocked(jobsApi.setJobStatus).mockResolvedValue(undefined)
      const { result } = renderHook(() => useJobActions(), { wrapper: makeWrapper() })
      await act(async () => {
        await result.current.setStatus.mutateAsync({ id: 'j1', status: JOB_STATUS.SAVED })
      })
      expect(toast.success).toHaveBeenCalledWith('Moved to saved')
    })

    it('shows "Moved to applied" toast when status is applied', async () => {
      vi.mocked(jobsApi.setJobStatus).mockResolvedValue(undefined)
      const { result } = renderHook(() => useJobActions(), { wrapper: makeWrapper() })
      await act(async () => {
        await result.current.setStatus.mutateAsync({ id: 'j1', status: JOB_STATUS.APPLIED })
      })
      expect(toast.success).toHaveBeenCalledWith('Moved to applied')
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

  // ── dismiss ───────────────────────────────────────────────────────────────

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

    it('shows "Job dismissed" success toast on success', async () => {
      vi.mocked(jobsApi.dismissJob).mockResolvedValue(undefined)
      const { result } = renderHook(() => useJobActions(), { wrapper: makeWrapper() })
      await act(async () => {
        await result.current.dismiss.mutateAsync({ id: 'j1', reason: 'user_dismissed' })
      })
      expect(toast.success).toHaveBeenCalledWith('Job dismissed')
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

  // ── bulkDismiss ───────────────────────────────────────────────────────────

  describe('bulkDismiss', () => {
    it('calls bulkDismissJobs with ids array', async () => {
      vi.mocked(jobsApi.bulkDismissJobs).mockResolvedValue({ dismissed: 3 })
      const { result } = renderHook(() => useJobActions(), { wrapper: makeWrapper() })
      await act(async () => {
        await result.current.bulkDismiss.mutateAsync(['j1', 'j2', 'j3'])
      })
      expect(jobsApi.bulkDismissJobs).toHaveBeenCalledWith(['j1', 'j2', 'j3'])
    })

    it('shows plural "jobs dismissed" toast when count > 1', async () => {
      vi.mocked(jobsApi.bulkDismissJobs).mockResolvedValue({ dismissed: 3 })
      const { result } = renderHook(() => useJobActions(), { wrapper: makeWrapper() })
      await act(async () => {
        await result.current.bulkDismiss.mutateAsync(['j1', 'j2', 'j3'])
      })
      expect(toast.success).toHaveBeenCalledWith('3 jobs dismissed')
    })

    it('shows singular "job dismissed" toast when count is 1', async () => {
      vi.mocked(jobsApi.bulkDismissJobs).mockResolvedValue({ dismissed: 1 })
      const { result } = renderHook(() => useJobActions(), { wrapper: makeWrapper() })
      await act(async () => {
        await result.current.bulkDismiss.mutateAsync(['j1'])
      })
      expect(toast.success).toHaveBeenCalledWith('1 job dismissed')
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

  // ── Return shape ──────────────────────────────────────────────────────────

  describe('return shape', () => {
    it('returns setStatus, dismiss, bulkDismiss mutations', () => {
      const { result } = renderHook(() => useJobActions(), { wrapper: makeWrapper() })
      expect(result.current.setStatus).toBeDefined()
      expect(result.current.dismiss).toBeDefined()
      expect(result.current.bulkDismiss).toBeDefined()
    })

    it('mutations start in idle state', () => {
      const { result } = renderHook(() => useJobActions(), { wrapper: makeWrapper() })
      expect(result.current.setStatus.isPending).toBe(false)
      expect(result.current.dismiss.isPending).toBe(false)
      expect(result.current.bulkDismiss.isPending).toBe(false)
    })
  })
})
