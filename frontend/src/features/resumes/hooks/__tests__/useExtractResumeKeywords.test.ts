import { describe, it, expect, vi, beforeEach } from 'vitest'
import { renderHook, waitFor, act } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { createElement } from 'react'
import { toast } from 'sonner'
import * as resumesApi from '@/features/resumes/api/resumes.api'
import { useExtractResumeKeywords } from '../useExtractResumeKeywords'

vi.mock('@/features/resumes/api/resumes.api')
vi.mock('sonner', () => ({ toast: { success: vi.fn(), error: vi.fn() } }))

function makeWrapper() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: 0, gcTime: 0 } } })
  return {
    wrapper: ({ children }: { children: React.ReactNode }) =>
      createElement(QueryClientProvider, { client: qc }, children),
    qc,
  }
}

const sampleResume = {
  id: 'r1',
  user_id: 'u1',
  label: 'L',
  original_filename: 'f.pdf',
  content_type: 'application/pdf' as const,
  size_bytes: 10,
  created_at: '2026-07-03T00:00:00Z',
  keywords: ['python', 'react'],
}

describe('useExtractResumeKeywords', () => {
  beforeEach(() => vi.clearAllMocks())

  it('invalidates the resume list and toasts success', async () => {
    vi.mocked(resumesApi.extractResumeKeywords).mockResolvedValue(sampleResume)
    const { wrapper, qc } = makeWrapper()
    const invalidateSpy = vi.spyOn(qc, 'invalidateQueries')
    const { result } = renderHook(() => useExtractResumeKeywords(), { wrapper })

    await act(async () => {
      await result.current.mutateAsync('r1')
    })

    expect(resumesApi.extractResumeKeywords).toHaveBeenCalledWith('r1')
    await waitFor(() => expect(invalidateSpy).toHaveBeenCalled())
    expect(toast.success).toHaveBeenCalledWith('Keyword extraction started')
  })

  it('surfaces an error toast without invalidating on failure', async () => {
    vi.mocked(resumesApi.extractResumeKeywords).mockRejectedValue(new Error('extraction failed'))
    const { wrapper, qc } = makeWrapper()
    const invalidateSpy = vi.spyOn(qc, 'invalidateQueries')
    const { result } = renderHook(() => useExtractResumeKeywords(), { wrapper })

    await act(async () => {
      await result.current.mutateAsync('r1').catch(() => {})
    })

    await waitFor(() => expect(result.current.isError).toBe(true))
    expect(invalidateSpy).not.toHaveBeenCalled()
    expect(toast.error).toHaveBeenCalledWith('extraction failed')
  })

  it('falls back to a generic error message when the error is not an Error instance', async () => {
    vi.mocked(resumesApi.extractResumeKeywords).mockRejectedValue('boom')
    const { wrapper } = makeWrapper()
    const { result } = renderHook(() => useExtractResumeKeywords(), { wrapper })

    await act(async () => {
      await result.current.mutateAsync('r1').catch(() => {})
    })

    await waitFor(() => expect(result.current.isError).toBe(true))
    expect(toast.error).toHaveBeenCalledWith('Failed to extract keywords')
  })
})
