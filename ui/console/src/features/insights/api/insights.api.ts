import { z } from 'zod'
import type {
  TechCount,
  CompanyWeekCount,
  FunnelStep,
  LocationData,
  SponsorshipDay,
  CompanyVelocityInsight,
  RoleCategoryCount,
} from '@/types'
import { apiClient } from '@/lib/core/api-client'

// ── Zod schemas ──────────────────────────────────────────────────────────────

const TechCountSchema = z.object({ tech: z.string(), count: z.number() })

const CompanyWeekCountSchema = z.object({
  company_name: z.string(),
  week: z.string(),
  count: z.number(),
})

const FunnelStepSchema = z.object({ stage: z.string(), count: z.number() })

const LocationCountSchema = z.object({ country: z.string(), count: z.number() })

const StateCountSchema = z.object({ state: z.string(), count: z.number() })

const LocationDataSchema = z
  .object({
    countries: z.array(LocationCountSchema),
    states: z.array(StateCountSchema),
  })
  .nullable()

const SponsorshipDaySchema = z.object({
  date: z.string(),
  total: z.number(),
  sponsored: z.number(),
})

const CompanyVelocityInsightSchema = z.object({
  company_id: z.string(),
  company_name: z.string(),
  week_current: z.number(),
  week_1: z.number(),
  week_2: z.number(),
  week_3: z.number(),
  multiplier: z.number(),
  velocity_tag: z.enum(['spike', 'rising', 'normal']),
})

const RoleCategoryCountSchema = z.object({ category: z.string(), count: z.number() })

// ── API functions ────────────────────────────────────────────────────────────

export async function fetchTechStack(limit = 15): Promise<TechCount[]> {
  const raw = await apiClient.get<unknown>(`/admin/insights/tech-stack?limit=${limit}`)
  const result = z.array(TechCountSchema).nullable().parse(raw)
  return result ?? []
}

export async function fetchCompanyActivity(): Promise<CompanyWeekCount[]> {
  const raw = await apiClient.get<unknown>('/admin/insights/company-activity')
  const result = z.array(CompanyWeekCountSchema).nullable().parse(raw)
  return result ?? []
}

export async function fetchFunnel(): Promise<FunnelStep[]> {
  const raw = await apiClient.get<unknown>('/admin/insights/funnel')
  const result = z.array(FunnelStepSchema).nullable().parse(raw)
  return result ?? []
}

export async function fetchLocation(): Promise<LocationData> {
  const raw = await apiClient.get<unknown>('/admin/insights/location')
  const result = LocationDataSchema.parse(raw)
  return result ?? { countries: [], states: [] }
}

export async function fetchSponsorshipTrend(days = 30): Promise<SponsorshipDay[]> {
  const raw = await apiClient.get<unknown>(`/admin/insights/sponsorship?days=${days}`)
  const result = z.array(SponsorshipDaySchema).nullable().parse(raw)
  return result ?? []
}

export async function fetchVelocity(limit = 20): Promise<CompanyVelocityInsight[]> {
  const raw = await apiClient.get<unknown>(`/admin/insights/velocity?limit=${limit}`)
  const result = z.array(CompanyVelocityInsightSchema).nullable().parse(raw)
  return result ?? []
}

export async function fetchRoleCategories(): Promise<RoleCategoryCount[]> {
  const raw = await apiClient.get<unknown>('/admin/insights/role-categories')
  const result = z.array(RoleCategoryCountSchema).nullable().parse(raw)
  return result ?? []
}
