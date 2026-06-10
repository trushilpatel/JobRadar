import { apiClient } from '@/lib/core/api-client'
import * as insightsApi from '@/features/insights/api/insights.api'

vi.mock('@/lib/core/api-client')

describe('insights API normalization', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  // ── fetchTechStack ─────────────────────────────────────────────────────────

  describe('fetchTechStack', () => {
    it('returns [] when server responds with null', async () => {
      vi.mocked(apiClient.get).mockResolvedValue(null)
      await expect(insightsApi.fetchTechStack(15)).resolves.toEqual([])
    })

    it('returns data when server responds with array', async () => {
      const data = [{ tech: 'Go', count: 10 }]
      vi.mocked(apiClient.get).mockResolvedValue(data)
      await expect(insightsApi.fetchTechStack(15)).resolves.toEqual(data)
    })

    it('calls correct endpoint with limit param', async () => {
      vi.mocked(apiClient.get).mockResolvedValue([])
      await insightsApi.fetchTechStack(5)
      expect(apiClient.get).toHaveBeenCalledWith('/admin/insights/tech-stack?limit=5')
    })

    it('uses default limit of 15', async () => {
      vi.mocked(apiClient.get).mockResolvedValue([])
      await insightsApi.fetchTechStack()
      expect(apiClient.get).toHaveBeenCalledWith('/admin/insights/tech-stack?limit=15')
    })
  })

  // ── fetchCompanyActivity ───────────────────────────────────────────────────

  describe('fetchCompanyActivity', () => {
    it('returns [] when server responds with null', async () => {
      vi.mocked(apiClient.get).mockResolvedValue(null)
      await expect(insightsApi.fetchCompanyActivity()).resolves.toEqual([])
    })

    it('returns data when server responds with array', async () => {
      const data = [{ company_name: 'Stripe', week: '2024-01-01', count: 8 }]
      vi.mocked(apiClient.get).mockResolvedValue(data)
      await expect(insightsApi.fetchCompanyActivity()).resolves.toEqual(data)
    })

    it('calls correct endpoint', async () => {
      vi.mocked(apiClient.get).mockResolvedValue([])
      await insightsApi.fetchCompanyActivity()
      expect(apiClient.get).toHaveBeenCalledWith('/admin/insights/company-activity')
    })
  })

  // ── fetchFunnel ────────────────────────────────────────────────────────────

  describe('fetchFunnel', () => {
    it('returns [] when server responds with null', async () => {
      vi.mocked(apiClient.get).mockResolvedValue(null)
      await expect(insightsApi.fetchFunnel()).resolves.toEqual([])
    })

    it('returns data when server responds with array', async () => {
      const data = [{ stage: 'enriched', count: 100 }]
      vi.mocked(apiClient.get).mockResolvedValue(data)
      await expect(insightsApi.fetchFunnel()).resolves.toEqual(data)
    })

    it('calls correct endpoint', async () => {
      vi.mocked(apiClient.get).mockResolvedValue([])
      await insightsApi.fetchFunnel()
      expect(apiClient.get).toHaveBeenCalledWith('/admin/insights/funnel')
    })
  })

  // ── fetchLocation ──────────────────────────────────────────────────────────

  describe('fetchLocation', () => {
    it('returns { countries: [], states: [] } when server responds with null', async () => {
      vi.mocked(apiClient.get).mockResolvedValue(null)
      await expect(insightsApi.fetchLocation()).resolves.toEqual({ countries: [], states: [] })
    })

    it('returns data when present', async () => {
      const data = { countries: [{ country: 'USA', count: 5 }], states: [] }
      vi.mocked(apiClient.get).mockResolvedValue(data)
      await expect(insightsApi.fetchLocation()).resolves.toEqual(data)
    })

    it('calls correct endpoint', async () => {
      vi.mocked(apiClient.get).mockResolvedValue({ countries: [], states: [] })
      await insightsApi.fetchLocation()
      expect(apiClient.get).toHaveBeenCalledWith('/admin/insights/location')
    })
  })

  // ── fetchSponsorshipTrend ──────────────────────────────────────────────────

  describe('fetchSponsorshipTrend', () => {
    it('returns [] when server responds with null', async () => {
      vi.mocked(apiClient.get).mockResolvedValue(null)
      await expect(insightsApi.fetchSponsorshipTrend(30)).resolves.toEqual([])
    })

    it('returns data when server responds with array', async () => {
      const data = [{ date: '2024-01-15', total: 100, sponsored: 40 }]
      vi.mocked(apiClient.get).mockResolvedValue(data)
      await expect(insightsApi.fetchSponsorshipTrend(30)).resolves.toEqual(data)
    })

    it('calls correct endpoint with days param', async () => {
      vi.mocked(apiClient.get).mockResolvedValue([])
      await insightsApi.fetchSponsorshipTrend(14)
      expect(apiClient.get).toHaveBeenCalledWith('/admin/insights/sponsorship?days=14')
    })

    it('uses default days of 30', async () => {
      vi.mocked(apiClient.get).mockResolvedValue([])
      await insightsApi.fetchSponsorshipTrend()
      expect(apiClient.get).toHaveBeenCalledWith('/admin/insights/sponsorship?days=30')
    })
  })

  // ── fetchVelocity ──────────────────────────────────────────────────────────

  describe('fetchVelocity', () => {
    it('returns [] when server responds with null', async () => {
      vi.mocked(apiClient.get).mockResolvedValue(null)
      await expect(insightsApi.fetchVelocity(20)).resolves.toEqual([])
    })

    it('returns data when server responds with array', async () => {
      const data = [
        {
          company_id: '1',
          company_name: 'Acme',
          week_current: 5,
          week_1: 3,
          week_2: 2,
          week_3: 1,
          multiplier: 1.7,
          velocity_tag: 'rising' as const,
        },
      ]
      vi.mocked(apiClient.get).mockResolvedValue(data)
      await expect(insightsApi.fetchVelocity(20)).resolves.toEqual(data)
    })

    it('calls correct endpoint with limit param', async () => {
      vi.mocked(apiClient.get).mockResolvedValue([])
      await insightsApi.fetchVelocity(10)
      expect(apiClient.get).toHaveBeenCalledWith('/admin/insights/velocity?limit=10')
    })

    it('uses default limit of 20', async () => {
      vi.mocked(apiClient.get).mockResolvedValue([])
      await insightsApi.fetchVelocity()
      expect(apiClient.get).toHaveBeenCalledWith('/admin/insights/velocity?limit=20')
    })
  })

  // ── fetchRoleCategories ────────────────────────────────────────────────────

  describe('fetchRoleCategories', () => {
    it('returns [] when server responds with null', async () => {
      vi.mocked(apiClient.get).mockResolvedValue(null)
      await expect(insightsApi.fetchRoleCategories()).resolves.toEqual([])
    })

    it('returns data when server responds with array', async () => {
      const data = [{ category: 'backend', count: 80 }]
      vi.mocked(apiClient.get).mockResolvedValue(data)
      await expect(insightsApi.fetchRoleCategories()).resolves.toEqual(data)
    })

    it('calls correct endpoint', async () => {
      vi.mocked(apiClient.get).mockResolvedValue([])
      await insightsApi.fetchRoleCategories()
      expect(apiClient.get).toHaveBeenCalledWith('/admin/insights/role-categories')
    })

    it('normalizes empty-string category to "unknown"', async () => {
      vi.mocked(apiClient.get).mockResolvedValue([
        { category: '', count: 658 },
        { category: 'backend', count: 400 },
      ])
      const result = await insightsApi.fetchRoleCategories()
      expect(result.find((r) => r.category === '')).toBeUndefined()
      expect(result.find((r) => r.category === 'unknown')).toEqual({
        category: 'unknown',
        count: 658,
      })
      expect(result.find((r) => r.category === 'backend')).toEqual({
        category: 'backend',
        count: 400,
      })
    })

    it('folds duplicate categories by summing counts', async () => {
      vi.mocked(apiClient.get).mockResolvedValue([
        { category: '', count: 300 },
        { category: '', count: 200 },
      ])
      const result = await insightsApi.fetchRoleCategories()
      expect(result).toHaveLength(1)
      expect(result[0]).toEqual({ category: 'unknown', count: 500 })
    })

    it('returns empty array when server returns null', async () => {
      vi.mocked(apiClient.get).mockResolvedValue(null)
      const result = await insightsApi.fetchRoleCategories()
      expect(result).toEqual([])
    })
  })
})
