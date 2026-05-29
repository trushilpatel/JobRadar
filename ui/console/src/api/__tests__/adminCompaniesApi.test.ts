import { describe, it, expect, vi, beforeEach } from 'vitest'
import { apiClient } from '@/lib/api-client'
import {
  fetchAdminCompanies,
  createAdminCompany,
  updateAdminCompany,
  deleteAdminCompany,
  updateAdminCompanyScanSettings,
} from '@/api/adminCompanies'
import type { AdminCompany, AdminCompanyListResult } from '@/api/adminCompanies'

vi.mock('@/lib/api-client')

const mockAdminCompany: AdminCompany = {
  id: 'stripe',
  name: 'Stripe',
  website: 'https://stripe.com/jobs',
  created_at: '2024-01-01T00:00:00Z',
  scan_settings: {
    company_id: 'stripe',
    adapter_type: 'greenhouse',
    adapter_config: null,
    scan_interval_override: null,
    active: true,
    last_scan_at: null,
    last_success_at: null,
    last_error_at: null,
    last_error_message: null,
    consecutive_errors: 0,
    created_at: '2024-01-01T00:00:00Z',
    updated_at: '2024-01-01T00:00:00Z',
  },
}

const mockListResult: AdminCompanyListResult = {
  data: [mockAdminCompany],
  total: 1,
  total_active: 1,
  limit: 50,
  next_cursor: null,
}

describe('adminCompanies API', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  // ── fetchAdminCompanies ─────────────────────────────────────────────────────

  describe('fetchAdminCompanies', () => {
    it('calls the correct endpoint with default cursor and limit', async () => {
      vi.mocked(apiClient.get).mockResolvedValue(mockListResult)
      await fetchAdminCompanies()
      const [url] = vi.mocked(apiClient.get).mock.calls[0]!
      expect(url).toContain('/admin/companies')
      expect(url).not.toContain('cursor=')
      expect(url).toContain('limit=50')
    })

    it('appends cursor and limit params when cursor is non-empty', async () => {
      vi.mocked(apiClient.get).mockResolvedValue(mockListResult)
      await fetchAdminCompanies({}, 'cursor-abc', 25)
      const [url] = vi.mocked(apiClient.get).mock.calls[0]!
      expect(url).toContain('cursor=cursor-abc')
      expect(url).toContain('limit=25')
    })

    it('appends search param when provided', async () => {
      vi.mocked(apiClient.get).mockResolvedValue(mockListResult)
      await fetchAdminCompanies({ search: 'stripe' })
      const [url] = vi.mocked(apiClient.get).mock.calls[0]!
      expect(url).toContain('search=stripe')
    })

    it('appends active param when set to true', async () => {
      vi.mocked(apiClient.get).mockResolvedValue(mockListResult)
      await fetchAdminCompanies({ active: true })
      const [url] = vi.mocked(apiClient.get).mock.calls[0]!
      expect(url).toContain('active=true')
    })

    it('appends active param when set to false', async () => {
      vi.mocked(apiClient.get).mockResolvedValue(mockListResult)
      await fetchAdminCompanies({ active: false })
      const [url] = vi.mocked(apiClient.get).mock.calls[0]!
      expect(url).toContain('active=false')
    })

    it('omits active param when not set', async () => {
      vi.mocked(apiClient.get).mockResolvedValue(mockListResult)
      await fetchAdminCompanies({})
      const [url] = vi.mocked(apiClient.get).mock.calls[0]!
      expect(url).not.toContain('active=')
    })

    it('appends multiple adapter_types params', async () => {
      vi.mocked(apiClient.get).mockResolvedValue(mockListResult)
      await fetchAdminCompanies({ adapterTypes: ['greenhouse', 'lever'] })
      const [url] = vi.mocked(apiClient.get).mock.calls[0]!
      expect(url).toContain('adapter_types=greenhouse')
      expect(url).toContain('adapter_types=lever')
    })

    it('appends sort_by and sort_dir when provided', async () => {
      vi.mocked(apiClient.get).mockResolvedValue(mockListResult)
      await fetchAdminCompanies({ sortBy: 'name', sortDir: 'asc' })
      const [url] = vi.mocked(apiClient.get).mock.calls[0]!
      expect(url).toContain('sort_by=name')
      expect(url).toContain('sort_dir=asc')
    })

    it('appends scan_status when provided', async () => {
      vi.mocked(apiClient.get).mockResolvedValue(mockListResult)
      await fetchAdminCompanies({ scanStatus: 'error' })
      const [url] = vi.mocked(apiClient.get).mock.calls[0]!
      expect(url).toContain('scan_status=error')
    })

    it('returns the full list result', async () => {
      vi.mocked(apiClient.get).mockResolvedValue(mockListResult)
      const result = await fetchAdminCompanies()
      expect(result.total).toBe(1)
      expect(result.data).toHaveLength(1)
      expect(result.data[0]?.id).toBe('stripe')
    })

    it('propagates rejection when apiClient.get rejects', async () => {
      vi.mocked(apiClient.get).mockRejectedValue(new Error('Network error'))
      await expect(fetchAdminCompanies()).rejects.toThrow('Network error')
    })
  })

  // ── createAdminCompany ──────────────────────────────────────────────────────

  describe('createAdminCompany', () => {
    it('calls POST /admin/companies with the payload', async () => {
      vi.mocked(apiClient.post).mockResolvedValue(mockAdminCompany)
      const input = {
        id: 'stripe',
        name: 'Stripe',
        website: 'https://stripe.com/jobs',
        scan_settings: { adapter_type: 'greenhouse', active: true },
      }
      await createAdminCompany(input)
      expect(apiClient.post).toHaveBeenCalledWith('/admin/companies', {
        ...input,
        slug: 'stripe',
      })
    })

    it('returns the created company', async () => {
      vi.mocked(apiClient.post).mockResolvedValue(mockAdminCompany)
      const result = await createAdminCompany({
        id: 'stripe',
        name: 'Stripe',
        website: 'https://stripe.com/jobs',
      })
      expect(result.id).toBe('stripe')
      expect(result.name).toBe('Stripe')
    })

    it('calls POST without scan_settings when omitted', async () => {
      vi.mocked(apiClient.post).mockResolvedValue(mockAdminCompany)
      const input = { id: 'acme', name: 'Acme', website: 'https://acme.com' }
      await createAdminCompany(input)
      expect(apiClient.post).toHaveBeenCalledWith('/admin/companies', {
        ...input,
        slug: 'acme',
      })
    })

    it('derives slug from id so backend CompanyPublicInput validation passes', async () => {
      vi.mocked(apiClient.post).mockResolvedValue(mockAdminCompany)
      await createAdminCompany({
        id: 'inception',
        name: 'Inception',
        website: 'https://jobs.gem.com/inception',
        scan_settings: { adapter_type: 'gem', active: true },
      })
      const [, body] = vi.mocked(apiClient.post).mock.calls[0]!
      expect(body).toMatchObject({ id: 'inception', slug: 'inception' })
    })

    it('handles slugs that include hyphens (gem vanity boards like lumalabs-ai)', async () => {
      vi.mocked(apiClient.post).mockResolvedValue(mockAdminCompany)
      await createAdminCompany({
        id: 'lumalabs-ai',
        name: 'Luma AI',
        website: 'https://jobs.gem.com/lumalabs-ai',
        scan_settings: { adapter_type: 'gem', active: true },
      })
      const [, body] = vi.mocked(apiClient.post).mock.calls[0]!
      expect(body).toMatchObject({ id: 'lumalabs-ai', slug: 'lumalabs-ai' })
    })
  })

  // ── updateAdminCompany ──────────────────────────────────────────────────────

  describe('updateAdminCompany', () => {
    it('calls PATCH /admin/companies/:id with fields', async () => {
      vi.mocked(apiClient.patch).mockResolvedValue(undefined)
      await updateAdminCompany('stripe', { name: 'Stripe Inc' })
      expect(apiClient.patch).toHaveBeenCalledWith('/admin/companies/stripe', {
        name: 'Stripe Inc',
      })
    })

    it('URL-encodes the company id', async () => {
      vi.mocked(apiClient.patch).mockResolvedValue(undefined)
      await updateAdminCompany('my company', { name: 'My Company' })
      const [url] = vi.mocked(apiClient.patch).mock.calls[0]!
      expect(url).toContain('my%20company')
    })

    it('can patch only the website field', async () => {
      vi.mocked(apiClient.patch).mockResolvedValue(undefined)
      await updateAdminCompany('stripe', { website: 'https://stripe.com/careers' })
      expect(apiClient.patch).toHaveBeenCalledWith('/admin/companies/stripe', {
        website: 'https://stripe.com/careers',
      })
    })
  })

  // ── deleteAdminCompany ──────────────────────────────────────────────────────

  describe('deleteAdminCompany', () => {
    it('calls DELETE /admin/companies/:id', async () => {
      vi.mocked(apiClient.delete).mockResolvedValue(undefined)
      await deleteAdminCompany('stripe')
      expect(apiClient.delete).toHaveBeenCalledWith('/admin/companies/stripe')
    })

    it('URL-encodes special characters in id', async () => {
      vi.mocked(apiClient.delete).mockResolvedValue(undefined)
      await deleteAdminCompany('my company')
      const [url] = vi.mocked(apiClient.delete).mock.calls[0]!
      expect(url).toContain('my%20company')
    })
  })

  // ── updateAdminCompanyScanSettings ─────────────────────────────────────────

  describe('updateAdminCompanyScanSettings', () => {
    it('calls PATCH /admin/companies/:id/scan-settings with input', async () => {
      vi.mocked(apiClient.patch).mockResolvedValue(undefined)
      await updateAdminCompanyScanSettings('stripe', { active: false })
      expect(apiClient.patch).toHaveBeenCalledWith('/admin/companies/stripe/scan-settings', {
        active: false,
      })
    })

    it('sends adapter_type in patch body', async () => {
      vi.mocked(apiClient.patch).mockResolvedValue(undefined)
      await updateAdminCompanyScanSettings('acme', { adapter_type: 'lever', active: true })
      expect(apiClient.patch).toHaveBeenCalledWith('/admin/companies/acme/scan-settings', {
        adapter_type: 'lever',
        active: true,
      })
    })

    it('sends scan_interval_override as null to clear it', async () => {
      vi.mocked(apiClient.patch).mockResolvedValue(undefined)
      await updateAdminCompanyScanSettings('stripe', { scan_interval_override: null })
      expect(apiClient.patch).toHaveBeenCalledWith('/admin/companies/stripe/scan-settings', {
        scan_interval_override: null,
      })
    })

    it('URL-encodes id with special characters', async () => {
      vi.mocked(apiClient.patch).mockResolvedValue(undefined)
      await updateAdminCompanyScanSettings('my company', { active: true })
      const [url] = vi.mocked(apiClient.patch).mock.calls[0]!
      expect(url).toContain('my%20company')
    })
  })
})
