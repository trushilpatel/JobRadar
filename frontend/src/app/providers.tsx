import { QueryClientProvider } from '@tanstack/react-query'
import { NuqsAdapter } from 'nuqs/adapters/react-router/v7'
import { Toaster } from 'sonner'
import { queryClient } from '@/lib/core/queryClient'

export function Providers({ children }: { children: React.ReactNode }) {
  return (
    <QueryClientProvider client={queryClient}>
      <NuqsAdapter>
        <Toaster position="bottom-center" theme="dark" richColors />
        {children}
      </NuqsAdapter>
    </QueryClientProvider>
  )
}
