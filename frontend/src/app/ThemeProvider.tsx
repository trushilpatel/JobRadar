import { useState, useEffect, useCallback, type ReactNode } from 'react'
import type { ThemePreference } from '@/types'
import { ThemeContext } from '@/app/ThemeContext'

function resolveTheme(pref: ThemePreference): 'dark' | 'light' {
  if (pref === 'system') {
    return window.matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light'
  }
  return pref
}

function applyTheme(resolved: 'dark' | 'light') {
  const root = document.documentElement
  if (resolved === 'dark') {
    root.classList.add('dark')
    root.style.colorScheme = 'dark'
  } else {
    root.classList.remove('dark')
    root.style.colorScheme = 'light'
  }
}

interface ThemeProviderProps {
  children: ReactNode
  preference: ThemePreference
  onThemeChange: (theme: ThemePreference) => void
}

export function ThemeProvider({ children, preference, onThemeChange }: ThemeProviderProps) {
  // `applied` tracks what's actually rendered — updated optimistically by
  // setTheme() so the toggle reflects the new choice immediately, without
  // waiting for the server round-trip.
  const [applied, setApplied] = useState<ThemePreference>(preference)

  // Sync applied when server preference changes (e.g. after a fresh load or
  // when the backend starts storing the theme field).
  useEffect(() => {
    setApplied(preference)
    applyTheme(resolveTheme(preference))
    localStorage.setItem('jobradar-theme', preference)
  }, [preference])

  // React to OS changes when preference is 'system'
  useEffect(() => {
    if (preference !== 'system') return
    const mq = window.matchMedia('(prefers-color-scheme: dark)')
    const handler = () => applyTheme(resolveTheme('system'))
    mq.addEventListener('change', handler)
    return () => mq.removeEventListener('change', handler)
  }, [preference])

  const setTheme = useCallback(
    (next: ThemePreference) => {
      applyTheme(resolveTheme(next))
      localStorage.setItem('jobradar-theme', next)
      setApplied(next) // optimistic — toggle updates instantly
      onThemeChange(next)
    },
    [onThemeChange]
  )

  return (
    <ThemeContext.Provider value={{ theme: applied, setTheme }}>{children}</ThemeContext.Provider>
  )
}
