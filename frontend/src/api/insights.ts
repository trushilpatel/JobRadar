const BASE = '/api'

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
  const res = await fetch(`${BASE}/insights/tech-stack?limit=${limit}`)
  if (!res.ok) throw new Error('Failed to fetch tech stack')
  return res.json().then((r: { data: TechCount[] }) => r.data)
}

export async function fetchCompanyActivity(): Promise<CompanyWeekCount[]> {
  const res = await fetch(`${BASE}/insights/company-activity`)
  if (!res.ok) throw new Error('Failed to fetch company activity')
  return res.json().then((r: { data: CompanyWeekCount[] }) => r.data)
}

export async function fetchFunnel(): Promise<FunnelStep[]> {
  const res = await fetch(`${BASE}/insights/funnel`)
  if (!res.ok) throw new Error('Failed to fetch funnel')
  return res.json().then((r: { data: FunnelStep[] }) => r.data)
}

export async function fetchLocation(): Promise<LocationData> {
  const res = await fetch(`${BASE}/insights/location`)
  if (!res.ok) throw new Error('Failed to fetch location')
  return res.json().then((r: { data: LocationData }) => r.data)
}

export async function fetchSponsorshipTrend(days = 30): Promise<SponsorshipDay[]> {
  const res = await fetch(`${BASE}/insights/sponsorship?days=${days}`)
  if (!res.ok) throw new Error('Failed to fetch sponsorship trend')
  return res.json().then((r: { data: SponsorshipDay[] }) => r.data)
}

export async function fetchVelocity(limit = 20): Promise<CompanyVelocity[]> {
  const res = await fetch(`${BASE}/insights/velocity?limit=${limit}`)
  if (!res.ok) throw new Error('Failed to fetch velocity')
  return res.json().then((r: { data: CompanyVelocity[] }) => r.data)
}

export async function fetchRoleCategories(): Promise<RoleCategoryCount[]> {
  const res = await fetch(`${BASE}/insights/role-categories`)
  if (!res.ok) throw new Error('Failed to fetch role categories')
  return res.json().then((r: { data: RoleCategoryCount[] }) => r.data)
}
