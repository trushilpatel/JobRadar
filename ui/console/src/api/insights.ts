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
  return apiClient.get<TechCount[]>(`/admin/insights/tech-stack?limit=${limit}`)
}

export async function fetchCompanyActivity(): Promise<CompanyWeekCount[]> {
  return apiClient.get<CompanyWeekCount[]>('/admin/insights/company-activity')
}

export async function fetchFunnel(): Promise<FunnelStep[]> {
  return apiClient.get<FunnelStep[]>('/admin/insights/funnel')
}

export async function fetchLocation(): Promise<LocationData> {
  return apiClient.get<LocationData>('/admin/insights/location')
}

export async function fetchSponsorshipTrend(days = 30): Promise<SponsorshipDay[]> {
  return apiClient.get<SponsorshipDay[]>(`/admin/insights/sponsorship?days=${days}`)
}

export async function fetchVelocity(limit = 20): Promise<CompanyVelocityInsight[]> {
  return apiClient.get<CompanyVelocityInsight[]>(`/admin/insights/velocity?limit=${limit}`)
}

export async function fetchRoleCategories(): Promise<RoleCategoryCount[]> {
  return apiClient.get<RoleCategoryCount[]>('/admin/insights/role-categories')
}
