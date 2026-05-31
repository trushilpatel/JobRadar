import React from 'react'
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { MemoryRouter } from 'react-router-dom'

vi.mock('@/lib/hooks/useUserConfig')
vi.mock('@/lib/core/supabase', () => ({
  supabase: {
    auth: {
      getSession: vi.fn(),
      signOut: vi.fn(),
      onAuthStateChange: vi.fn(() => ({
        data: { subscription: { unsubscribe: vi.fn() } },
      })),
    },
  },
}))
vi.mock('@/features/auth', () => ({
  useAuthStore: vi.fn((selector: (s: unknown) => unknown) =>
    selector({
      identity: {
        user_id: 'u1',
        email: 'test@test.com',
        display_name: 'Test User',
        status: 'active',
        roles: [],
      },
      refreshIdentity: vi.fn(),
      signOut: vi.fn(),
    })
  ),
  updateMe: vi.fn(),
  deleteMe: vi.fn(),
}))
vi.mock('sonner', () => ({ toast: { success: vi.fn(), error: vi.fn() } }))

import * as useUserConfigModule from '@/lib/hooks/useUserConfig'
import { AccountPage } from '@/pages/AccountPage'
import { JOB_COMMANDS } from '@/features/jobs'

function makeWrapper() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: 0, gcTime: 0 } } })
  return function Wrapper({ children }: { children: React.ReactNode }) {
    return (
      <MemoryRouter>
        <QueryClientProvider client={qc}>{children}</QueryClientProvider>
      </MemoryRouter>
    )
  }
}

const mockPatchMutate = vi.fn()

function mockUserConfig(show = false, isPending = false, configLoading = false) {
  vi.mocked(useUserConfigModule.useUserConfig).mockReturnValue({
    data: configLoading ? undefined : { show_keyboard_shortcuts: show },
    isLoading: configLoading,
    isError: false,
    isSuccess: !configLoading,
  } as ReturnType<typeof useUserConfigModule.useUserConfig>)
  vi.mocked(useUserConfigModule.usePatchUserConfig).mockReturnValue({
    mutate: mockPatchMutate,
    isPending,
  } as unknown as ReturnType<typeof useUserConfigModule.usePatchUserConfig>)
}

describe('AccountPage', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('renders Show keyboard shortcut hints toggle', () => {
    mockUserConfig(false)
    render(<AccountPage />, { wrapper: makeWrapper() })
    expect(screen.getByText('Show keyboard shortcut hints')).toBeInTheDocument()
  })

  it('renders keyboard shortcut hints subtitle', () => {
    mockUserConfig(false)
    render(<AccountPage />, { wrapper: makeWrapper() })
    expect(screen.getByText('Display shortcut keys inside action buttons')).toBeInTheDocument()
  })

  it('toggle reflects useUserConfig data (unchecked when false)', () => {
    mockUserConfig(false)
    render(<AccountPage />, { wrapper: makeWrapper() })
    const switchEl = screen.getByRole('switch')
    expect(switchEl).toHaveAttribute('data-state', 'unchecked')
  })

  it('toggle reflects useUserConfig data (checked when true)', () => {
    mockUserConfig(true)
    render(<AccountPage />, { wrapper: makeWrapper() })
    const switchEl = screen.getByRole('switch')
    expect(switchEl).toHaveAttribute('data-state', 'checked')
  })

  it('toggle is disabled while mutation is in-flight', () => {
    mockUserConfig(false, true)
    render(<AccountPage />, { wrapper: makeWrapper() })
    const switchEl = screen.getByRole('switch')
    expect(switchEl).toBeDisabled()
  })

  it('calls patchUserConfig when toggled', async () => {
    mockUserConfig(false)
    const user = userEvent.setup()
    render(<AccountPage />, { wrapper: makeWrapper() })
    const switchEl = screen.getByRole('switch')
    await user.click(switchEl)
    expect(mockPatchMutate).toHaveBeenCalledWith({ show_keyboard_shortcuts: true })
  })

  it('toggle is disabled while useUserConfig is loading', () => {
    mockUserConfig(false, false, true)
    render(<AccountPage />, { wrapper: makeWrapper() })
    const switchEl = screen.getByRole('switch')
    expect(switchEl).toBeDisabled()
  })

  it('renders the registry-derived shortcut reference table', () => {
    mockUserConfig(false)
    render(<AccountPage />, { wrapper: makeWrapper() })
    expect(screen.getByText(JOB_COMMANDS['jobs.save'].label)).toBeInTheDocument()
    expect(screen.getByText(JOB_COMMANDS['jobs.nav.next'].label)).toBeInTheDocument()
    expect(screen.getByText(JOB_COMMANDS['jobs.tab.go-new'].label)).toBeInTheDocument()
  })

  it('renders the reference table even when config is loading (static registry)', () => {
    mockUserConfig(false, false, true)
    render(<AccountPage />, { wrapper: makeWrapper() })
    expect(screen.getByText(JOB_COMMANDS['jobs.dismiss'].label)).toBeInTheDocument()
  })
})
