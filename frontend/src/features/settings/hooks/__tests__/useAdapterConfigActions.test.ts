import { describe, it, expect, vi, beforeEach } from 'vitest'
import { renderHook, act } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { createElement } from 'react'
import { toast } from 'sonner'
import * as adapterConfigsApi from '@/features/settings/api/adapterConfigs.api'
import { useAdapterConfigActions } from '../useAdapterConfigActions'
import type { AdapterConfig } from '@/features/settings/api/adapterConfigs.api'
import { settingsQueryKeys } from '@/features/settings/queryKeys'

vi.mock('@/features/settings/api/adapterConfigs.api')
vi.mock('sonner', () => ({
  toast: {
    success: vi.fn(),
    error: vi.fn(),
  },
}))

const mockConfig: AdapterConfig = {
  adapter_type: 'greenhouse',
  scan_interval_minutes: 1440,
  inter_company_delay_s: 3,
  last_scheduled_at: null,
  updated_at: '2026-01-01T00:00:00Z',
}

const updatedConfig: AdapterConfig = {
  ...mockConfig,
  scan_interval_minutes: 720,
  inter_company_delay_s: 5,
  updated_at: '2026-01-02T00:00:00Z',
}

const leverConfig: AdapterConfig = {
  adapter_type: 'lever',
  scan_interval_minutes: 720,
  inter_company_delay_s: 10,
  last_scheduled_at: null,
  updated_at: '2026-01-01T00:00:00Z',
}

/** Returns both the wrapper and the client so tests can inspect cache state. */
function makeClient(initialData?: AdapterConfig[]) {
  const qc = new QueryClient({
    // Keep gcTime > 0 so setQueryData survives long enough for assertions
    defaultOptions: { queries: { retry: 0, gcTime: 5_000, staleTime: Infinity } },
  })
  if (initialData) {
    qc.setQueryData<AdapterConfig[]>(settingsQueryKeys.adapterConfigs(), initialData)
  }
  const wrapper = ({ children }: { children: React.ReactNode }) =>
    createElement(QueryClientProvider, { client: qc }, children)
  return { qc, wrapper }
}

describe('useAdapterConfigActions', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  // ── updateConfig success ─────────────────────────────────────────────────────

  describe('updateConfig — success path', () => {
    it('calls updateAdapterConfig with correct adapter type and data', async () => {
      vi.mocked(adapterConfigsApi.updateAdapterConfig).mockResolvedValue(updatedConfig)
      const { wrapper } = makeClient([mockConfig])
      const { result } = renderHook(() => useAdapterConfigActions(), { wrapper })

      await act(async () => {
        await result.current.updateConfig('greenhouse', {
          scan_interval_minutes: 720,
          inter_company_delay_s: 5,
        })
      })

      expect(adapterConfigsApi.updateAdapterConfig).toHaveBeenCalledWith('greenhouse', {
        scan_interval_minutes: 720,
        inter_company_delay_s: 5,
      })
    })

    it('shows success toast with adapter type', async () => {
      vi.mocked(adapterConfigsApi.updateAdapterConfig).mockResolvedValue(updatedConfig)
      const { wrapper } = makeClient([mockConfig])
      const { result } = renderHook(() => useAdapterConfigActions(), { wrapper })

      await act(async () => {
        await result.current.updateConfig('greenhouse', {
          scan_interval_minutes: 720,
          inter_company_delay_s: 5,
        })
      })

      expect(toast.success).toHaveBeenCalledWith('greenhouse config updated')
    })

    it('applies optimistic update to cache before API call resolves', async () => {
      let resolveRequest!: (value: AdapterConfig) => void
      vi.mocked(adapterConfigsApi.updateAdapterConfig).mockReturnValue(
        new Promise<AdapterConfig>((res) => {
          resolveRequest = res
        })
      )

      const { qc, wrapper } = makeClient([mockConfig])
      const { result } = renderHook(() => useAdapterConfigActions(), { wrapper })

      act(() => {
        void result.current.updateConfig('greenhouse', {
          scan_interval_minutes: 720,
          inter_company_delay_s: 5,
        })
      })

      // Optimistic update should have been applied immediately
      const cached = qc.getQueryData<AdapterConfig[]>(settingsQueryKeys.adapterConfigs())
      const cachedConfig = cached?.find((c) => c.adapter_type === 'greenhouse')
      expect(cachedConfig?.scan_interval_minutes).toBe(720)

      // Resolve the pending API call
      await act(async () => {
        resolveRequest(updatedConfig)
      })
    })

    it('replaces cache with server response after success', async () => {
      const serverResponse: AdapterConfig = {
        ...mockConfig,
        scan_interval_minutes: 480,
        inter_company_delay_s: 10,
      }
      vi.mocked(adapterConfigsApi.updateAdapterConfig).mockResolvedValue(serverResponse)

      const { qc, wrapper } = makeClient([mockConfig])
      const { result } = renderHook(() => useAdapterConfigActions(), { wrapper })

      await act(async () => {
        await result.current.updateConfig('greenhouse', {
          scan_interval_minutes: 480,
          inter_company_delay_s: 10,
        })
      })

      const cached = qc.getQueryData<AdapterConfig[]>(settingsQueryKeys.adapterConfigs())
      const cachedConfig = cached?.find((c) => c.adapter_type === 'greenhouse')
      expect(cachedConfig?.scan_interval_minutes).toBe(480)
      expect(cachedConfig?.inter_company_delay_s).toBe(10)
    })
  })

  // ── updateConfig error handling ─────────────────────────────────────────────

  describe('updateConfig — error path', () => {
    it('shows error toast with message on API failure', async () => {
      vi.mocked(adapterConfigsApi.updateAdapterConfig).mockRejectedValue(
        new Error('Server unavailable')
      )
      const { wrapper } = makeClient([mockConfig])
      const { result } = renderHook(() => useAdapterConfigActions(), { wrapper })

      await act(async () => {
        try {
          await result.current.updateConfig('greenhouse', {
            scan_interval_minutes: 720,
            inter_company_delay_s: 5,
          })
        } catch {
          // expected — hook re-throws
        }
      })

      expect(toast.error).toHaveBeenCalledWith('Server unavailable')
    })

    it('shows generic error toast when error is not an Error instance', async () => {
      vi.mocked(adapterConfigsApi.updateAdapterConfig).mockRejectedValue('string error')
      const { wrapper } = makeClient([mockConfig])
      const { result } = renderHook(() => useAdapterConfigActions(), { wrapper })

      await act(async () => {
        try {
          await result.current.updateConfig('greenhouse', {
            scan_interval_minutes: 720,
            inter_company_delay_s: 5,
          })
        } catch {
          // expected
        }
      })

      expect(toast.error).toHaveBeenCalledWith('Failed to update config')
    })

    it('rolls back the cache to previous value on failure', async () => {
      vi.mocked(adapterConfigsApi.updateAdapterConfig).mockRejectedValue(new Error('Server error'))

      const { qc, wrapper } = makeClient([mockConfig])
      const { result } = renderHook(() => useAdapterConfigActions(), { wrapper })

      await act(async () => {
        try {
          await result.current.updateConfig('greenhouse', {
            scan_interval_minutes: 720,
            inter_company_delay_s: 5,
          })
        } catch {
          // expected — hook re-throws
        }
      })

      // Cache should be rolled back to original values
      const cached = qc.getQueryData<AdapterConfig[]>(settingsQueryKeys.adapterConfigs())
      const cachedConfig = cached?.find((c) => c.adapter_type === 'greenhouse')
      expect(cachedConfig?.scan_interval_minutes).toBe(1440)
      expect(cachedConfig?.inter_company_delay_s).toBe(3)
    })

    it('re-throws the error after rollback', async () => {
      const error = new Error('Server error')
      vi.mocked(adapterConfigsApi.updateAdapterConfig).mockRejectedValue(error)

      const { wrapper } = makeClient([mockConfig])
      const { result } = renderHook(() => useAdapterConfigActions(), { wrapper })

      await expect(
        act(async () => {
          await result.current.updateConfig('greenhouse', {
            scan_interval_minutes: 720,
            inter_company_delay_s: 5,
          })
        })
      ).rejects.toThrow('Server error')
    })

    it('does not show success toast on failure', async () => {
      vi.mocked(adapterConfigsApi.updateAdapterConfig).mockRejectedValue(new Error('Fail'))
      const { wrapper } = makeClient([mockConfig])
      const { result } = renderHook(() => useAdapterConfigActions(), { wrapper })

      await act(async () => {
        try {
          await result.current.updateConfig('greenhouse', {
            scan_interval_minutes: 720,
            inter_company_delay_s: 5,
          })
        } catch {
          // expected
        }
      })

      expect(toast.success).not.toHaveBeenCalled()
    })
  })

  // ── cache seam: optimistic update and rollback use factory keys ─────────────

  describe('cache seam — factory key usage', () => {
    it('reads and writes cache via settingsQueryKeys.adapterConfigs() factory key', async () => {
      const serverResponse: AdapterConfig = { ...mockConfig, scan_interval_minutes: 360 }
      vi.mocked(adapterConfigsApi.updateAdapterConfig).mockResolvedValue(serverResponse)

      const qc = new QueryClient({
        defaultOptions: { queries: { retry: 0, gcTime: 5_000, staleTime: Infinity } },
      })
      // Seed cache using the factory key — if the hook uses a different key, the
      // optimistic update will see no existing data (undefined) and the rollback
      // will also be a no-op, leaving the cache empty after an error.
      qc.setQueryData<AdapterConfig[]>(settingsQueryKeys.adapterConfigs(), [mockConfig])

      const wrapper = ({ children }: { children: React.ReactNode }) =>
        createElement(QueryClientProvider, { client: qc }, children)

      const { result } = renderHook(() => useAdapterConfigActions(), { wrapper })

      await act(async () => {
        await result.current.updateConfig('greenhouse', {
          scan_interval_minutes: 360,
          inter_company_delay_s: 3,
        })
      })

      // The hook must read/write via the factory key so the update is visible
      const cached = qc.getQueryData<AdapterConfig[]>(settingsQueryKeys.adapterConfigs())
      const cachedConfig = cached?.find((c) => c.adapter_type === 'greenhouse')
      expect(cachedConfig?.scan_interval_minutes).toBe(360)
    })

    it('rollback writes back to settingsQueryKeys.adapterConfigs() factory key on error', async () => {
      vi.mocked(adapterConfigsApi.updateAdapterConfig).mockRejectedValue(new Error('Server error'))

      const qc = new QueryClient({
        defaultOptions: { queries: { retry: 0, gcTime: 5_000, staleTime: Infinity } },
      })
      qc.setQueryData<AdapterConfig[]>(settingsQueryKeys.adapterConfigs(), [mockConfig])

      const wrapper = ({ children }: { children: React.ReactNode }) =>
        createElement(QueryClientProvider, { client: qc }, children)

      const { result } = renderHook(() => useAdapterConfigActions(), { wrapper })

      await act(async () => {
        try {
          await result.current.updateConfig('greenhouse', {
            scan_interval_minutes: 720,
            inter_company_delay_s: 5,
          })
        } catch {
          // expected
        }
      })

      // After rollback the factory key should still hold the original data
      const cached = qc.getQueryData<AdapterConfig[]>(settingsQueryKeys.adapterConfigs())
      expect(cached?.find((c) => c.adapter_type === 'greenhouse')?.scan_interval_minutes).toBe(1440)
    })
  })

  // ── multiple adapters in cache ───────────────────────────────────────────────

  describe('multiple adapters in cache', () => {
    it('only updates the matching adapter in cache', async () => {
      const updatedGreenhouse: AdapterConfig = { ...mockConfig, scan_interval_minutes: 360 }
      vi.mocked(adapterConfigsApi.updateAdapterConfig).mockResolvedValue(updatedGreenhouse)

      const { qc, wrapper } = makeClient([mockConfig, leverConfig])
      const { result } = renderHook(() => useAdapterConfigActions(), { wrapper })

      await act(async () => {
        await result.current.updateConfig('greenhouse', {
          scan_interval_minutes: 360,
          inter_company_delay_s: 3,
        })
      })

      const cached = qc.getQueryData<AdapterConfig[]>(settingsQueryKeys.adapterConfigs())
      expect(cached?.find((c) => c.adapter_type === 'greenhouse')?.scan_interval_minutes).toBe(360)
      // Lever config should be unchanged
      expect(cached?.find((c) => c.adapter_type === 'lever')?.scan_interval_minutes).toBe(720)
    })

    it('only rolls back the matching adapter on failure', async () => {
      vi.mocked(adapterConfigsApi.updateAdapterConfig).mockRejectedValue(new Error('Server error'))

      const { qc, wrapper } = makeClient([mockConfig, leverConfig])
      const { result } = renderHook(() => useAdapterConfigActions(), { wrapper })

      await act(async () => {
        try {
          await result.current.updateConfig('greenhouse', {
            scan_interval_minutes: 720,
            inter_company_delay_s: 5,
          })
        } catch {
          // expected
        }
      })

      const cached = qc.getQueryData<AdapterConfig[]>(settingsQueryKeys.adapterConfigs())
      // Greenhouse rolled back
      expect(cached?.find((c) => c.adapter_type === 'greenhouse')?.scan_interval_minutes).toBe(1440)
      // Lever untouched
      expect(cached?.find((c) => c.adapter_type === 'lever')?.scan_interval_minutes).toBe(720)
    })
  })
})
