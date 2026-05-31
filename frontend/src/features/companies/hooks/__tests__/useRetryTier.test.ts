import { describe, it, expect, vi, beforeEach } from 'vitest'
import { renderHook, act } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { createElement } from 'react'
import { toast } from 'sonner'
import * as pipelineApi from '@/lib/api/pipeline'
import { useRetryTier } from '../useRetryTier'
import { companyQueryKeys } from '../../queryKeys'

vi.mock('@/lib/api/pipeline')
vi.mock('sonner', () => ({
  toast: {
    success: vi.fn(),
    error: vi.fn(),
  },
}))

function makeWrapper() {
  const qc = new QueryClient({
    defaultOptions: { queries: { retry: 0, gcTime: 0 } },
  })
  return {
    qc,
    wrapper: ({ children }: { children: React.ReactNode }) =>
      createElement(QueryClientProvider, { client: qc }, children),
  }
}

describe('useRetryTier', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('calls triggerRetry with the correct worker and tier', async () => {
    vi.mocked(pipelineApi.triggerRetry).mockResolvedValue({ drained: 3 })
    const { wrapper } = makeWrapper()

    const { result } = renderHook(() => useRetryTier(), { wrapper })

    await act(async () => {
      await result.current.mutateAsync({ worker: 'filter', tier: '1m' })
    })

    expect(pipelineApi.triggerRetry).toHaveBeenCalledWith('filter', '1m')
  })

  it('shows success toast with drained count after successful retry', async () => {
    vi.mocked(pipelineApi.triggerRetry).mockResolvedValue({ drained: 3 })
    const { wrapper } = makeWrapper()

    const { result } = renderHook(() => useRetryTier(), { wrapper })

    await act(async () => {
      await result.current.mutateAsync({ worker: 'filter', tier: '1m' })
    })

    expect(toast.success).toHaveBeenCalledWith('Retrying 3 jobs')
  })

  it('shows singular "job" when drained is 1', async () => {
    vi.mocked(pipelineApi.triggerRetry).mockResolvedValue({ drained: 1 })
    const { wrapper } = makeWrapper()

    const { result } = renderHook(() => useRetryTier(), { wrapper })

    await act(async () => {
      await result.current.mutateAsync({ worker: 'enricher', tier: '10m' })
    })

    expect(toast.success).toHaveBeenCalledWith('Retrying 1 job')
  })

  it('invalidates pipelineStats query key after success', async () => {
    vi.mocked(pipelineApi.triggerRetry).mockResolvedValue({ drained: 2 })
    const { qc, wrapper } = makeWrapper()
    const invalidateSpy = vi.spyOn(qc, 'invalidateQueries')

    const { result } = renderHook(() => useRetryTier(), { wrapper })

    await act(async () => {
      await result.current.mutateAsync({ worker: 'filter', tier: 'dlq' })
    })

    expect(invalidateSpy).toHaveBeenCalledWith({
      queryKey: companyQueryKeys.pipelineStats(),
    })
  })

  it('shows error toast when triggerRetry rejects', async () => {
    vi.mocked(pipelineApi.triggerRetry).mockRejectedValue(new Error('Server unavailable'))
    const { wrapper } = makeWrapper()

    const { result } = renderHook(() => useRetryTier(), { wrapper })

    await act(async () => {
      try {
        await result.current.mutateAsync({ worker: 'enricher', tier: '1h' })
      } catch {
        // mutation error expected
      }
    })

    expect(toast.error).toHaveBeenCalledWith('Failed to trigger retry')
  })
})
