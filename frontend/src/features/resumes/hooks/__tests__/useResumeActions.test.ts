import { describe, it, expect, vi, beforeEach } from 'vitest'
import { renderHook, waitFor, act } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { createElement } from 'react'
import * as resumesApi from '@/features/resumes/api/resumes.api'
import { useResumeActions } from '../useResumeActions'

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
  extracted_text: '',
}

describe('useResumeActions', () => {
  beforeEach(() => vi.clearAllMocks())

  it('upload invalidates the resume list on success', async () => {
    vi.mocked(resumesApi.uploadResume).mockResolvedValue(sampleResume)
    const { wrapper, qc } = makeWrapper()
    const invalidateSpy = vi.spyOn(qc, 'invalidateQueries')
    const { result } = renderHook(() => useResumeActions(), { wrapper })

    await act(async () => {
      await result.current.upload.mutateAsync({ file: new File(['d'], 'f.pdf'), label: 'L' })
    })
    await waitFor(() => expect(invalidateSpy).toHaveBeenCalled())
  })

  it('upload surfaces an error without invalidating on failure', async () => {
    vi.mocked(resumesApi.uploadResume).mockRejectedValue(new Error('file too large'))
    const { wrapper } = makeWrapper()
    const { result } = renderHook(() => useResumeActions(), { wrapper })

    await act(async () => {
      await result.current.upload
        .mutateAsync({ file: new File(['d'], 'f.pdf'), label: 'L' })
        .catch(() => {})
    })
    await waitFor(() => expect(result.current.upload.isError).toBe(true))
  })

  it('remove invalidates the resume list on success', async () => {
    vi.mocked(resumesApi.deleteResume).mockResolvedValue(undefined)
    const { wrapper, qc } = makeWrapper()
    const invalidateSpy = vi.spyOn(qc, 'invalidateQueries')
    const { result } = renderHook(() => useResumeActions(), { wrapper })

    await act(async () => {
      await result.current.remove.mutateAsync('r1')
    })
    await waitFor(() => expect(invalidateSpy).toHaveBeenCalled())
  })

  it('rename invalidates the resume list on success', async () => {
    vi.mocked(resumesApi.renameResume).mockResolvedValue(sampleResume)
    const { wrapper, qc } = makeWrapper()
    const invalidateSpy = vi.spyOn(qc, 'invalidateQueries')
    const { result } = renderHook(() => useResumeActions(), { wrapper })

    await act(async () => {
      await result.current.rename.mutateAsync({ id: 'r1', label: 'New label' })
    })
    await waitFor(() => expect(invalidateSpy).toHaveBeenCalled())
  })

  it('download resolves the file blob without invalidating the list', async () => {
    const blob = new Blob(['%PDF-1.4'], { type: 'application/pdf' })
    vi.mocked(resumesApi.downloadResume).mockResolvedValue(blob)
    const { wrapper, qc } = makeWrapper()
    const invalidateSpy = vi.spyOn(qc, 'invalidateQueries')
    const { result } = renderHook(() => useResumeActions(), { wrapper })

    let resolved: Blob | undefined
    await act(async () => {
      resolved = await result.current.download.mutateAsync('r1')
    })
    expect(resolved).toBe(blob)
    expect(invalidateSpy).not.toHaveBeenCalled()
  })

  it('download surfaces an error on failure', async () => {
    vi.mocked(resumesApi.downloadResume).mockRejectedValue(new Error('storage unavailable'))
    const { wrapper } = makeWrapper()
    const { result } = renderHook(() => useResumeActions(), { wrapper })

    await act(async () => {
      await result.current.download.mutateAsync('r1').catch(() => {})
    })
    await waitFor(() => expect(result.current.download.isError).toBe(true))
  })
})
