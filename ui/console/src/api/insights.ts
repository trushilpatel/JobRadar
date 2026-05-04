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
  return apiClient.get<TechCount[]>(`/insights/tech-stack?limit=${limit}`)
}

export async function fetchCompanyActivity(): Promise<CompanyWeekCount[]> {
  return apiClient.get<CompanyWeekCount[]>('/insights/company-activity')
}

export async function fetchFunnel(): Promise<FunnelStep[]> {
  return apiClient.get<FunnelStep[]>('/insights/funnel')
}

export async function fetchLocation(): Promise<LocationData> {
  return apiClient.get<LocationData>('/insights/location')
}

export async function fetchSponsorshipTrend(days = 30): Promise<SponsorshipDay[]> {
  return apiClient.get<SponsorshipDay[]>(`/insights/sponsorship?days=${days}`)
}

export async function fetchVelocity(limit = 20): Promise<CompanyVelocityInsight[]> {
  return apiClient.get<CompanyVelocityInsight[]>(`/insights/velocity?limit=${limit}`)
}

export async function fetchRoleCategories(): Promise<RoleCategoryCount[]> {
  return apiClient.get<RoleCategoryCount[]>('/insights/role-categories')
}
