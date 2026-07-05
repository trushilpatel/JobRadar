import { describe, it, expect, vi, beforeEach } from 'vitest'
import { renderHook, waitFor, act } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { createElement } from 'react'
import * as applyQueueApi from '@/features/apply-queue/api/apply-queue.api'
import { useSetApplyQueueResume } from '../useSetApplyQueueResume'

vi.mock('@/features/apply-queue/api/apply-queue.api')
vi.mock('sonner', () => ({ toast: { success: vi.fn(), error: vi.fn() } }))

function makeWrapper() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: 0, gcTime: 0 } } })
  return {
    wrapper: ({ children }: { children: React.ReactNode }) =>
      createElement(QueryClientProvider, { client: qc }, children),
    qc,
  }
}

describe('useSetApplyQueueResume', () => {
  beforeEach(() => vi.clearAllMocks())

  it('calls setApplyQueueResume with the given jobId and resumeId', async () => {
    vi.mocked(applyQueueApi.setApplyQueueResume).mockResolvedValue(undefined)
    const { wrapper } = makeWrapper()
    const { result } = renderHook(() => useSetApplyQueueResume(), { wrapper })

    await act(async () => {
      await result.current.mutateAsync({ jobId: 'job-1', resumeId: 'resume-1' })
    })

    expect(applyQueueApi.setApplyQueueResume).toHaveBeenCalledWith('job-1', 'resume-1')
  })

  it('invalidates the apply-queue list cache on success', async () => {
    vi.mocked(applyQueueApi.setApplyQueueResume).mockResolvedValue(undefined)
    const { wrapper, qc } = makeWrapper()
    const invalidateSpy = vi.spyOn(qc, 'invalidateQueries')
    const { result } = renderHook(() => useSetApplyQueueResume(), { wrapper })

    await act(async () => {
      await result.current.mutateAsync({ jobId: 'job-1', resumeId: null })
    })

    await waitFor(() => expect(invalidateSpy).toHaveBeenCalledWith({ queryKey: ['apply-queue'] }))
  })

  it('surfaces an error state and does not invalidate on failure', async () => {
    vi.mocked(applyQueueApi.setApplyQueueResume).mockRejectedValue(new Error('conflict'))
    const { wrapper, qc } = makeWrapper()
    const invalidateSpy = vi.spyOn(qc, 'invalidateQueries')
    const { result } = renderHook(() => useSetApplyQueueResume(), { wrapper })

    await act(async () => {
      await result.current.mutateAsync({ jobId: 'job-1', resumeId: 'resume-1' }).catch(() => {})
    })

    await waitFor(() => expect(result.current.isError).toBe(true))
    expect(invalidateSpy).not.toHaveBeenCalled()
  })
})
