import { apiClient } from '@/lib/api-client'

export interface TechCount {
  tech: string
  count: number
}

export interface CompanyWeekCount {
  company_name: string
  week: string
  count: number
}

export interface FunnelStep {
  stage: string
  count: number
}

export interface LocationCount {
  country: string
  count: number
}

export interface StateCount {
  state: string
  count: number
}

export interface LocationData {
  countries: LocationCount[]
  states: StateCount[]
}

export interface SponsorshipDay {
  date: string
  total: number
  sponsored: number
}

export interface CompanyVelocity {
  company_id: string
  company_name: string
  this_week: number
  last_week: number
  delta: number
}

export interface RoleCategoryCount {
  category: string
  count: number
}

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

export async function fetchVelocity(limit = 20): Promise<CompanyVelocity[]> {
  return apiClient.get<CompanyVelocity[]>(`/insights/velocity?limit=${limit}`)
}

export async function fetchRoleCategories(): Promise<RoleCategoryCount[]> {
  return apiClient.get<RoleCategoryCount[]>('/insights/role-categories')
}
