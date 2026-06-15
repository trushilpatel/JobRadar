import { useAuth, useUser } from '@clerk/react'
import { useQuery } from '@tanstack/react-query'
import { fetchMe } from '../api/me.api'
import type { Identity } from '../api/me.api'

export interface IdentityResult {
  isLoaded: boolean
  isSignedIn: boolean
  isAdmin: boolean
  identity: Identity | undefined
}

export function useIdentity(): IdentityResult {
  const { isLoaded: authLoaded, isSignedIn } = useAuth()
  const { user, isLoaded: userLoaded } = useUser()

  const { data: identity } = useQuery({
    queryKey: ['me'],
    queryFn: fetchMe,
    enabled: !!isSignedIn,
    staleTime: 30_000,
  })

  const isAdmin = user?.publicMetadata?.role === 'admin'
  const isLoaded = authLoaded && userLoaded

  return { isLoaded, isSignedIn: !!isSignedIn, isAdmin, identity }
}
