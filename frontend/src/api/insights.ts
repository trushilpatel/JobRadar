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

export async function fetchTechStack(): Promise<TechCount[]> {
  const res = await fetch(`${BASE}/insights/tech-stack`)
  if (!res.ok) throw new Error('Failed to fetch tech stack')
  return res.json()
}

export async function fetchCompanyActivity(): Promise<CompanyWeekCount[]> {
  const res = await fetch(`${BASE}/insights/company-activity`)
  if (!res.ok) throw new Error('Failed to fetch company activity')
  return res.json()
}
