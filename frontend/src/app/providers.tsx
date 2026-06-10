import type { ReactNode } from 'react'
import { QueryClientProvider } from '@tanstack/react-query'
import { NuqsAdapter } from 'nuqs/adapters/react-router/v7'
import { Toaster } from 'sonner'
import { queryClient } from '@/lib/core/queryClient'
import { ThemeProvider } from '@/app/ThemeProvider'
import { useTheme, resolveSystemTheme } from '@/app/ThemeContext'
import { useUserConfig, usePatchUserConfig } from '@/lib/hooks/useUserConfig'
import { TooltipProvider } from '@/components/ui/tooltip'
import type { ThemePreference } from '@/types'

function ThemeAwareToaster() {
  const { theme } = useTheme()
  const resolved: 'dark' | 'light' = theme === 'system' ? resolveSystemTheme() : theme
  return <Toaster position="bottom-center" theme={resolved} richColors closeButton />
}

function ThemeSync({ children }: { children: ReactNode }) {
  const { data: userConfig } = useUserConfig()
  const patchConfig = usePatchUserConfig()
  const pref: ThemePreference = userConfig?.theme ?? 'dark'

  return (
    <ThemeProvider preference={pref} onThemeChange={(theme) => patchConfig.mutate({ theme })}>
      <ThemeAwareToaster />
      {children}
    </ThemeProvider>
  )
}

export function Providers({ children }: { children: React.ReactNode }) {
  return (
    <QueryClientProvider client={queryClient}>
      <NuqsAdapter>
        <ThemeSync>
          <TooltipProvider>{children}</TooltipProvider>
        </ThemeSync>
      </NuqsAdapter>
    </QueryClientProvider>
  )
}
