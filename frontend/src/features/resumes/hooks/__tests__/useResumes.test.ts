import { describe, it, expect, vi, beforeEach } from 'vitest'
import { renderHook, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { createElement } from 'react'
import * as resumesApi from '@/features/resumes/api/resumes.api'
import { useResumes } from '../useResumes'

vi.mock('@/features/resumes/api/resumes.api')

function makeWrapper() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: 0, gcTime: 0 } } })
  return ({ children }: { children: React.ReactNode }) =>
    createElement(QueryClientProvider, { client: qc }, children)
}

describe('useResumes', () => {
  beforeEach(() => vi.clearAllMocks())

  it('returns resumes when the query resolves', async () => {
    vi.mocked(resumesApi.fetchResumes).mockResolvedValue([
      {
        id: 'r1',
        user_id: 'u1',
        label: 'L',
        original_filename: 'f.pdf',
        content_type: 'application/pdf',
        size_bytes: 10,
        created_at: '2026-07-03T00:00:00Z',
        keywords: [],
      },
    ])
    const { result } = renderHook(() => useResumes(), { wrapper: makeWrapper() })
    await waitFor(() => expect(result.current.isLoading).toBe(false))
    expect(result.current.data).toHaveLength(1)
  })

  it('surfaces an error state when the fetch rejects', async () => {
    vi.mocked(resumesApi.fetchResumes).mockRejectedValue(new Error('network error'))
    const { result } = renderHook(() => useResumes(), { wrapper: makeWrapper() })
    await waitFor(() => expect(result.current.isError).toBe(true))
  })
})
