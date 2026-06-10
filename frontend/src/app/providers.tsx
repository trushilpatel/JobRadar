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

function readStoredTheme(): ThemePreference {
  try {
    const v = localStorage.getItem('jobradar-theme')
    if (v === 'light' || v === 'dark' || v === 'system') return v
  } catch {
    return 'dark'
  }
  return 'dark'
}

function ThemeSync({ children }: { children: ReactNode }) {
  const { data: userConfig } = useUserConfig()
  const patchConfig = usePatchUserConfig()
  // Server value wins if set; fall back to localStorage so the user's
  // in-session choice survives React Router navigations before the backend
  // theme field is available.
  const pref: ThemePreference = userConfig?.theme ?? readStoredTheme()

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
