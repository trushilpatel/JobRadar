import { useQuery } from '@tanstack/react-query'
import { insightQueryKeys } from '../queryKeys'
import {
  fetchTechStack,
  fetchCompanyActivity,
  fetchFunnel,
  fetchLocation,
  fetchSponsorshipTrend,
  fetchVelocity,
  fetchRoleCategories,
} from '@/api/insights'

export function useTechStack(limit = 15) {
  return useQuery({
    queryKey: insightQueryKeys.techStack(limit),
    queryFn: () => fetchTechStack(limit),
    staleTime: 30_000,
    refetchInterval: 5 * 60_000,
  })
}

export function useCompanyActivity() {
  return useQuery({
    queryKey: insightQueryKeys.companyActivity(),
    queryFn: fetchCompanyActivity,
    staleTime: 30_000,
    refetchInterval: 5 * 60_000,
  })
}

export function useFunnel() {
  return useQuery({
    queryKey: insightQueryKeys.funnel(),
    queryFn: fetchFunnel,
    staleTime: 30_000,
    refetchInterval: 5 * 60_000,
  })
}

export function useLocation() {
  return useQuery({
    queryKey: insightQueryKeys.location(),
    queryFn: fetchLocation,
    staleTime: 30_000,
    refetchInterval: 5 * 60_000,
  })
}

export function useSponsorshipTrend(days = 30) {
  return useQuery({
    queryKey: insightQueryKeys.sponsorshipTrend(days),
    queryFn: () => fetchSponsorshipTrend(days),
    staleTime: 30_000,
    refetchInterval: 5 * 60_000,
  })
}

export function useVelocity(limit = 20) {
  return useQuery({
    queryKey: insightQueryKeys.velocity(limit),
    queryFn: () => fetchVelocity(limit),
    staleTime: 30_000,
    refetchInterval: 5 * 60_000,
  })
}

export function useRoleCategories() {
  return useQuery({
    queryKey: insightQueryKeys.roleCategories(),
    queryFn: fetchRoleCategories,
    staleTime: 30_000,
    refetchInterval: 5 * 60_000,
  })
}
