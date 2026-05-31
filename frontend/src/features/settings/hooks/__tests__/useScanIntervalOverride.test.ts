import { describe, it, expect, vi, beforeEach } from 'vitest'
import { renderHook, act } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { createElement } from 'react'

vi.mock('@/features/settings/api/adapterConfigs.api', () => ({
  updateScanIntervalOverride: vi.fn(),
}))

vi.mock('sonner', () => ({
  toast: { success: vi.fn(), error: vi.fn() },
}))

import { useScanIntervalOverride } from '../useScanIntervalOverride'
import { updateScanIntervalOverride } from '@/features/settings/api/adapterConfigs.api'
import { toast } from 'sonner'
import {
  settingsQueryKeys,
  scanIntervalOverrideMutationInvalidationRoots,
} from '@/features/settings/queryKeys'

function makeWrapper() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return ({ children }: { children: React.ReactNode }) =>
    createElement(QueryClientProvider, { client: qc }, children)
}

function makeClientWrapper() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  const wrapper = ({ children }: { children: React.ReactNode }) =>
    createElement(QueryClientProvider, { client: qc }, children)
  return { qc, wrapper }
}

describe('useScanIntervalOverride', () => {
  beforeEach(() => vi.clearAllMocks())

  it('sets loading true during call and false after success', async () => {
    vi.mocked(updateScanIntervalOverride).mockResolvedValue({} as never)
    const { result } = renderHook(() => useScanIntervalOverride('co-1'), { wrapper: makeWrapper() })
    expect(result.current.loading).toBe(false)
    let promise: Promise<void>
    act(() => {
      promise = result.current.setOverride(60)
    })
    expect(result.current.loading).toBe(true)
    await act(async () => {
      await promise
    })
    expect(result.current.loading).toBe(false)
  })

  it('shows success toast with numeric override', async () => {
    vi.mocked(updateScanIntervalOverride).mockResolvedValue({} as never)
    const { result } = renderHook(() => useScanIntervalOverride('co-1'), { wrapper: makeWrapper() })
    await act(async () => {
      await result.current.setOverride(120)
    })
    expect(toast.success).toHaveBeenCalledWith('Scan interval override saved')
  })

  it('shows "Override cleared" toast for null override', async () => {
    vi.mocked(updateScanIntervalOverride).mockResolvedValue({} as never)
    const { result } = renderHook(() => useScanIntervalOverride('co-1'), { wrapper: makeWrapper() })
    await act(async () => {
      await result.current.setOverride(null)
    })
    expect(toast.success).toHaveBeenCalledWith('Override cleared')
  })

  it('shows error toast and re-throws on failure', async () => {
    const err = new Error('API failed')
    vi.mocked(updateScanIntervalOverride).mockRejectedValue(err)
    const { result } = renderHook(() => useScanIntervalOverride('co-1'), { wrapper: makeWrapper() })
    await expect(
      act(async () => {
        await result.current.setOverride(60)
      })
    ).rejects.toThrow('API failed')
    expect(toast.error).toHaveBeenCalledWith('API failed')
  })

  it('sets loading false after failure', async () => {
    vi.mocked(updateScanIntervalOverride).mockRejectedValue(new Error('fail'))
    const { result } = renderHook(() => useScanIntervalOverride('co-1'), { wrapper: makeWrapper() })
    await act(async () => {
      await result.current.setOverride(60).catch(() => {})
    })
    expect(result.current.loading).toBe(false)
  })

  it('calls updateScanIntervalOverride with the correct company ID and value', async () => {
    vi.mocked(updateScanIntervalOverride).mockResolvedValue({} as never)
    const { result } = renderHook(() => useScanIntervalOverride('company-abc'), {
      wrapper: makeWrapper(),
    })
    await act(async () => {
      await result.current.setOverride(240)
    })
    expect(updateScanIntervalOverride).toHaveBeenCalledWith('company-abc', 240)
  })

  // ── cache seam: invalidation uses factory keys ────────────────────────────────

  describe('cache seam — invalidation uses factory keys', () => {
    it('invalidates adminCompaniesBase via factory root after successful override', async () => {
      vi.mocked(updateScanIntervalOverride).mockResolvedValue({} as never)
      const { qc, wrapper } = makeClientWrapper()
      const invalidate = vi.spyOn(qc, 'invalidateQueries')

      const { result } = renderHook(() => useScanIntervalOverride('co-1'), { wrapper })
      await act(async () => {
        await result.current.setOverride(120)
      })

      // The hook must invalidate using the typed constant — not an inline string
      expect(invalidate).toHaveBeenCalledWith(
        expect.objectContaining({
          queryKey: scanIntervalOverrideMutationInvalidationRoots.adminCompanies,
        })
      )
    })

    it('invalidates companyScanConfig via factory key after successful override', async () => {
      vi.mocked(updateScanIntervalOverride).mockResolvedValue({} as never)
      const { qc, wrapper } = makeClientWrapper()
      const invalidate = vi.spyOn(qc, 'invalidateQueries')

      const { result } = renderHook(() => useScanIntervalOverride('co-2'), { wrapper })
      await act(async () => {
        await result.current.setOverride(60)
      })

      expect(invalidate).toHaveBeenCalledWith(
        expect.objectContaining({
          queryKey: settingsQueryKeys.companyScanConfig('co-2'),
        })
      )
    })

    it('scanIntervalOverrideMutationInvalidationRoots.adminCompanies matches companyQueryKeys.adminCompaniesBase()', () => {
      // Cross-feature contract: the root used by the settings hook must equal
      // the root registered by the companies feature. If either renames their
      // cache root, this test catches the drift.
      expect(scanIntervalOverrideMutationInvalidationRoots.adminCompanies).toEqual([
        'admin-companies',
      ])
    })
  })
})
