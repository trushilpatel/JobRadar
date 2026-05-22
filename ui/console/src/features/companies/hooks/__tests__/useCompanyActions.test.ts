import { describe, it, expect, vi, beforeEach } from 'vitest'
import { renderHook, act } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { createElement } from 'react'
import { toast } from 'sonner'
import * as adminApi from '@/api/adminCompanies'
import * as companiesApi from '@/api/companies'
import { useCompanyActions } from '../useCompanyActions'
import type { Company } from '@/types'

vi.mock('@/api/adminCompanies')
vi.mock('@/api/companies')
vi.mock('sonner', () => ({
  toast: {
    success: vi.fn(),
    error: vi.fn(),
  },
}))

const mockCompany: Company = {
  id: 'stripe',
  name: 'Stripe',
  url: 'https://stripe.com/jobs',
  adapter_type: 'greenhouse',
  active: true,
  created_at: '2024-01-01T00:00:00Z',
  last_scan_at: null,
  last_scan_status: null,
}

function makeWrapper() {
  const qc = new QueryClient({
    defaultOptions: { queries: { retry: 0, gcTime: 0 } },
  })
  return ({ children }: { children: React.ReactNode }) =>
    createElement(QueryClientProvider, { client: qc }, children)
}

describe('useCompanyActions', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  // ── toggleActive ────────────────────────────────────────────────────────────

  describe('toggleActive', () => {
    it('calls updateAdminCompanyScanSettings with active toggled off', async () => {
      vi.mocked(adminApi.updateAdminCompanyScanSettings).mockResolvedValue(undefined)
      const { result } = renderHook(() => useCompanyActions(), { wrapper: makeWrapper() })

      await act(async () => {
        await result.current.toggleActive(mockCompany) // active: true → false
      })

      expect(adminApi.updateAdminCompanyScanSettings).toHaveBeenCalledWith('stripe', {
        active: false,
      })
    })

    it('calls updateAdminCompanyScanSettings with active toggled on', async () => {
      vi.mocked(adminApi.updateAdminCompanyScanSettings).mockResolvedValue(undefined)
      const { result } = renderHook(() => useCompanyActions(), { wrapper: makeWrapper() })
      const inactiveCompany: Company = { ...mockCompany, active: false }

      await act(async () => {
        await result.current.toggleActive(inactiveCompany)
      })

      expect(adminApi.updateAdminCompanyScanSettings).toHaveBeenCalledWith('stripe', {
        active: true,
      })
    })

    it('shows success toast with deactivated message when toggling active company off', async () => {
      vi.mocked(adminApi.updateAdminCompanyScanSettings).mockResolvedValue(undefined)
      const { result } = renderHook(() => useCompanyActions(), { wrapper: makeWrapper() })

      await act(async () => {
        await result.current.toggleActive(mockCompany) // active: true → false
      })

      expect(toast.success).toHaveBeenCalledWith('Stripe deactivated')
    })

    it('shows success toast with activated message when toggling inactive company on', async () => {
      vi.mocked(adminApi.updateAdminCompanyScanSettings).mockResolvedValue(undefined)
      const { result } = renderHook(() => useCompanyActions(), { wrapper: makeWrapper() })
      const inactiveCompany: Company = { ...mockCompany, active: false }

      await act(async () => {
        await result.current.toggleActive(inactiveCompany)
      })

      expect(toast.success).toHaveBeenCalledWith('Stripe activated')
    })

    it('shows error toast when toggleActive fails', async () => {
      vi.mocked(adminApi.updateAdminCompanyScanSettings).mockRejectedValue(
        new Error('Server error')
      )
      const { result } = renderHook(() => useCompanyActions(), { wrapper: makeWrapper() })

      await act(async () => {
        await result.current.toggleActive(mockCompany)
      })

      expect(toast.error).toHaveBeenCalledWith('Failed to update company')
    })
  })

  // ── remove ──────────────────────────────────────────────────────────────────

  describe('remove', () => {
    it('calls deleteAdminCompany with the company id', async () => {
      vi.mocked(adminApi.deleteAdminCompany).mockResolvedValue(undefined)
      const { result } = renderHook(() => useCompanyActions(), { wrapper: makeWrapper() })

      await act(async () => {
        await result.current.remove('stripe')
      })

      expect(adminApi.deleteAdminCompany).toHaveBeenCalledWith('stripe')
    })

    it('shows success toast when delete succeeds', async () => {
      vi.mocked(adminApi.deleteAdminCompany).mockResolvedValue(undefined)
      const { result } = renderHook(() => useCompanyActions(), { wrapper: makeWrapper() })

      await act(async () => {
        await result.current.remove('stripe')
      })

      expect(toast.success).toHaveBeenCalledWith('Company deleted')
    })

    it('shows error toast when delete fails', async () => {
      vi.mocked(adminApi.deleteAdminCompany).mockRejectedValue(new Error('Network error'))
      const { result } = renderHook(() => useCompanyActions(), { wrapper: makeWrapper() })

      await act(async () => {
        await result.current.remove('stripe')
      })

      expect(toast.error).toHaveBeenCalledWith('Failed to delete company')
    })
  })

  // ── scan ────────────────────────────────────────────────────────────────────

  describe('scan', () => {
    it('calls triggerScan with company id', async () => {
      vi.mocked(companiesApi.triggerScan).mockResolvedValue(undefined)
      const { result } = renderHook(() => useCompanyActions(), { wrapper: makeWrapper() })

      await act(async () => {
        await result.current.scan(mockCompany)
      })

      expect(companiesApi.triggerScan).toHaveBeenCalledWith('stripe')
    })

    it('shows success toast with company name', async () => {
      vi.mocked(companiesApi.triggerScan).mockResolvedValue(undefined)
      const { result } = renderHook(() => useCompanyActions(), { wrapper: makeWrapper() })

      await act(async () => {
        await result.current.scan(mockCompany)
      })

      expect(toast.success).toHaveBeenCalledWith('Scan queued for Stripe')
    })

    it('adds company to scanning set during scan', async () => {
      let resolveRequest!: () => void
      vi.mocked(companiesApi.triggerScan).mockReturnValue(
        new Promise<void>((res) => {
          resolveRequest = res
        })
      )
      const { result } = renderHook(() => useCompanyActions(), { wrapper: makeWrapper() })

      act(() => {
        void result.current.scan(mockCompany)
      })

      expect(result.current.scanning.has('stripe')).toBe(true)

      await act(async () => {
        resolveRequest()
      })

      expect(result.current.scanning.has('stripe')).toBe(false)
    })

    it('shows error toast when scan fails', async () => {
      vi.mocked(companiesApi.triggerScan).mockRejectedValue(new Error('Queue full'))
      const { result } = renderHook(() => useCompanyActions(), { wrapper: makeWrapper() })

      await act(async () => {
        await result.current.scan(mockCompany)
      })

      expect(toast.error).toHaveBeenCalledWith('Queue full')
    })

    it('removes company from scanning set even on failure', async () => {
      vi.mocked(companiesApi.triggerScan).mockRejectedValue(new Error('Queue full'))
      const { result } = renderHook(() => useCompanyActions(), { wrapper: makeWrapper() })

      await act(async () => {
        await result.current.scan(mockCompany)
      })

      expect(result.current.scanning.has('stripe')).toBe(false)
    })
  })

  // ── scanAll ─────────────────────────────────────────────────────────────────

  describe('scanAll', () => {
    it('calls triggerScanAll', async () => {
      vi.mocked(companiesApi.triggerScanAll).mockResolvedValue({ queued: 5 })
      const { result } = renderHook(() => useCompanyActions(), { wrapper: makeWrapper() })

      await act(async () => {
        await result.current.scanAll()
      })

      expect(companiesApi.triggerScanAll).toHaveBeenCalled()
    })

    it('shows success toast with queued count (plural)', async () => {
      vi.mocked(companiesApi.triggerScanAll).mockResolvedValue({ queued: 5 })
      const { result } = renderHook(() => useCompanyActions(), { wrapper: makeWrapper() })

      await act(async () => {
        await result.current.scanAll()
      })

      expect(toast.success).toHaveBeenCalledWith('5 companies queued for scan')
    })

    it('shows success toast with singular "company" for queued=1', async () => {
      vi.mocked(companiesApi.triggerScanAll).mockResolvedValue({ queued: 1 })
      const { result } = renderHook(() => useCompanyActions(), { wrapper: makeWrapper() })

      await act(async () => {
        await result.current.scanAll()
      })

      expect(toast.success).toHaveBeenCalledWith('1 company queued for scan')
    })

    it('shows error toast on failure', async () => {
      vi.mocked(companiesApi.triggerScanAll).mockRejectedValue(new Error('Server unavailable'))
      const { result } = renderHook(() => useCompanyActions(), { wrapper: makeWrapper() })

      await act(async () => {
        await result.current.scanAll()
      })

      expect(toast.error).toHaveBeenCalledWith('Server unavailable')
    })
  })

  // ── handleCompanyAdded ──────────────────────────────────────────────────────

  describe('handleCompanyAdded', () => {
    it('triggers scan when company is active', async () => {
      vi.mocked(companiesApi.triggerScan).mockResolvedValue(undefined)
      const { result } = renderHook(() => useCompanyActions(), { wrapper: makeWrapper() })

      await act(async () => {
        await result.current.handleCompanyAdded(mockCompany)
      })

      expect(companiesApi.triggerScan).toHaveBeenCalledWith('stripe')
      expect(toast.success).toHaveBeenCalledWith('Stripe added and scan queued')
    })

    it('does not trigger scan when company is inactive', async () => {
      const inactiveCompany: Company = { ...mockCompany, active: false }
      const { result } = renderHook(() => useCompanyActions(), { wrapper: makeWrapper() })

      await act(async () => {
        await result.current.handleCompanyAdded(inactiveCompany)
      })

      expect(companiesApi.triggerScan).not.toHaveBeenCalled()
      expect(toast.success).toHaveBeenCalledWith('Stripe added')
    })
  })

  // ── handleCompanyUpdated ────────────────────────────────────────────────────

  describe('handleCompanyUpdated', () => {
    it('shows success toast with company name', () => {
      const { result } = renderHook(() => useCompanyActions(), { wrapper: makeWrapper() })

      act(() => {
        result.current.handleCompanyUpdated(mockCompany)
      })

      expect(toast.success).toHaveBeenCalledWith('Stripe updated')
    })
  })

  // ── bulkToggleActive ────────────────────────────────────────────────────────

  describe('bulkToggleActive', () => {
    it('calls setAllAdminCompaniesActive with next = true when currentAllActive is false', async () => {
      vi.mocked(adminApi.setAllAdminCompaniesActive).mockResolvedValue(undefined)
      const { result } = renderHook(() => useCompanyActions(), { wrapper: makeWrapper() })

      await act(async () => {
        await result.current.bulkToggleActive(false)
      })

      expect(adminApi.setAllAdminCompaniesActive).toHaveBeenCalledWith(true)
      expect(toast.success).toHaveBeenCalledWith('All companies activated')
    })

    it('calls setAllAdminCompaniesActive with next = false when currentAllActive is true', async () => {
      vi.mocked(adminApi.setAllAdminCompaniesActive).mockResolvedValue(undefined)
      const { result } = renderHook(() => useCompanyActions(), { wrapper: makeWrapper() })

      await act(async () => {
        await result.current.bulkToggleActive(true)
      })

      expect(adminApi.setAllAdminCompaniesActive).toHaveBeenCalledWith(false)
      expect(toast.success).toHaveBeenCalledWith('All companies deactivated')
    })

    it('shows error toast on bulk toggle failure', async () => {
      vi.mocked(adminApi.setAllAdminCompaniesActive).mockRejectedValue(new Error('DB error'))
      const { result } = renderHook(() => useCompanyActions(), { wrapper: makeWrapper() })

      await act(async () => {
        await result.current.bulkToggleActive(false)
      })

      expect(toast.error).toHaveBeenCalledWith('Failed to update companies')
    })
  })
})
