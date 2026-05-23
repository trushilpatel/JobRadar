import { describe, it, expect, vi, beforeEach } from 'vitest'
import { renderHook, act } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { createElement } from 'react'

vi.mock('@/api/adapterConfigs', () => ({
  updateScanIntervalOverride: vi.fn(),
}))

vi.mock('sonner', () => ({
  toast: { success: vi.fn(), error: vi.fn() },
}))

import { useScanIntervalOverride } from '../useScanIntervalOverride'
import { updateScanIntervalOverride } from '@/api/adapterConfigs'
import { toast } from 'sonner'

function makeWrapper() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return ({ children }: { children: React.ReactNode }) =>
    createElement(QueryClientProvider, { client: qc }, children)
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
})
