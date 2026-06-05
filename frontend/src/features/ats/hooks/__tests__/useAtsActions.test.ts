import { describe, it, expect, vi, beforeEach } from 'vitest'
import { renderHook, act, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { createElement } from 'react'
import { useAtsActions } from '../useAtsActions'
import { atsQueryKeys } from '../../queryKeys'
import type { ATS } from '@/features/ats'

vi.mock('../../api/ats.api', () => ({
  updateAts: vi.fn(),
}))

vi.mock('sonner', () => ({ toast: { success: vi.fn(), error: vi.fn() } }))

import { updateAts } from '../../api/ats.api'
import { toast } from 'sonner'
const mockUpdateAts = vi.mocked(updateAts)

const mockUpdatedAts: ATS = {
  name: 'greenhouse',
  display_name: 'Greenhouse',
  base_url: 'https://boards-api.greenhouse.io/v1/boards/',
  is_active: true,
  is_admin_creatable: true,
  scan_interval_min: 720,
  inter_company_delay_s: 3,
}

function makeWrapper() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
  return {
    wrapper: ({ children }: { children: React.ReactNode }) =>
      createElement(QueryClientProvider, { client }, children),
    client,
  }
}

describe('useAtsActions', () => {
  beforeEach(() => {
    mockUpdateAts.mockReset()
    vi.mocked(toast.success).mockReset()
    vi.mocked(toast.error).mockReset()
  })

  it('calls updateAts with correct name and data', async () => {
    mockUpdateAts.mockResolvedValue(mockUpdatedAts)
    const { wrapper } = makeWrapper()

    const { result } = renderHook(() => useAtsActions(), { wrapper })

    await act(async () => {
      await result.current.mutateAsync({
        name: 'greenhouse',
        data: { scan_interval_min: 720 },
      })
    })

    expect(mockUpdateAts).toHaveBeenCalledWith('greenhouse', { scan_interval_min: 720 })
  })

  it('invalidates both list and detail query keys on success', async () => {
    mockUpdateAts.mockResolvedValue(mockUpdatedAts)
    const { wrapper, client } = makeWrapper()

    const invalidateSpy = vi.spyOn(client, 'invalidateQueries')

    const { result } = renderHook(() => useAtsActions(), { wrapper })

    await act(async () => {
      await result.current.mutateAsync({
        name: 'greenhouse',
        data: { scan_interval_min: 720 },
      })
    })

    await waitFor(() => {
      expect(invalidateSpy).toHaveBeenCalledWith({
        queryKey: atsQueryKeys.list(),
      })
      expect(invalidateSpy).toHaveBeenCalledWith({
        queryKey: atsQueryKeys.detail('greenhouse'),
      })
    })
  })

  it('shows success toast on success', async () => {
    mockUpdateAts.mockResolvedValue(mockUpdatedAts)
    const { wrapper } = makeWrapper()

    const { result } = renderHook(() => useAtsActions(), { wrapper })

    await act(async () => {
      await result.current.mutateAsync({
        name: 'greenhouse',
        data: { scan_interval_min: 720 },
      })
    })

    expect(toast.success).toHaveBeenCalled()
  })

  it('shows error toast on failure', async () => {
    mockUpdateAts.mockRejectedValue(new Error('Failed'))
    const { wrapper } = makeWrapper()

    const { result } = renderHook(() => useAtsActions(), { wrapper })

    await act(async () => {
      try {
        await result.current.mutateAsync({
          name: 'greenhouse',
          data: { scan_interval_min: 720 },
        })
      } catch {
        // expected
      }
    })

    await waitFor(() => {
      expect(toast.error).toHaveBeenCalled()
    })
  })
})
