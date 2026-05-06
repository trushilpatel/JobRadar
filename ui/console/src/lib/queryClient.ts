import { QueryClient } from '@tanstack/react-query'

export const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      staleTime: 1000 * 30,      // 30s — jobs data is fresh for 30 seconds
      gcTime: 1000 * 60 * 5,    // 5min — keep unused data in cache for 5 minutes
      retry: 1,
      refetchOnWindowFocus: true,
    },
  },
})
