import type {
  TechCount,
  CompanyWeekCount,
  FunnelStep,
  LocationData,
  SponsorshipDay,
  CompanyVelocityInsight,
  RoleCategoryCount,
} from '@/types'
import { apiClient } from '@/lib/api-client'

export async function fetchTechStack(limit = 15): Promise<TechCount[]> {
  const result = await apiClient.get<TechCount[] | null>(
    `/admin/insights/tech-stack?limit=${limit}`
  )
  return result ?? []
}

export async function fetchCompanyActivity(): Promise<CompanyWeekCount[]> {
  const result = await apiClient.get<CompanyWeekCount[] | null>('/admin/insights/company-activity')
  return result ?? []
}

export async function fetchFunnel(): Promise<FunnelStep[]> {
  const result = await apiClient.get<FunnelStep[] | null>('/admin/insights/funnel')
  return result ?? []
}

export async function fetchLocation(): Promise<LocationData> {
  const result = await apiClient.get<LocationData | null>('/admin/insights/location')
  return result ?? { countries: [], states: [] }
}

export async function fetchSponsorshipTrend(days = 30): Promise<SponsorshipDay[]> {
  const result = await apiClient.get<SponsorshipDay[] | null>(
    `/admin/insights/sponsorship?days=${days}`
  )
  return result ?? []
}

export async function fetchVelocity(limit = 20): Promise<CompanyVelocityInsight[]> {
  const result = await apiClient.get<CompanyVelocityInsight[] | null>(
    `/admin/insights/velocity?limit=${limit}`
  )
  return result ?? []
}

export async function fetchRoleCategories(): Promise<RoleCategoryCount[]> {
  const result = await apiClient.get<RoleCategoryCount[] | null>('/admin/insights/role-categories')
  return result ?? []
}
