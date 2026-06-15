import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { useAuth } from '@clerk/react'
import { toast } from 'sonner'
import { getUserConfig, patchUserConfig } from '@/lib/api/userConfig'

export const USER_CONFIG_QUERY_KEY = ['user-config'] as const

export function useUserConfig() {
  const { isLoaded, isSignedIn } = useAuth()
  return useQuery({
    queryKey: USER_CONFIG_QUERY_KEY,
    queryFn: getUserConfig,
    staleTime: 30 * 60 * 1000,
    enabled: isLoaded && !!isSignedIn,
  })
}

export function usePatchUserConfig() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: patchUserConfig,
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: USER_CONFIG_QUERY_KEY })
    },
    onError: () => {
      toast.error('Failed to update preferences.')
    },
  })
}
