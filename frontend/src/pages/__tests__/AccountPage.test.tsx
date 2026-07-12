import React from 'react'
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { MemoryRouter } from 'react-router-dom'

vi.mock('@/lib/hooks/useUserConfig')

const refreshIdentityMock = vi.fn().mockResolvedValue(undefined)
const signOutMock = vi.fn().mockResolvedValue(undefined)

vi.mock('@clerk/react', () => ({
  useClerk: () => ({ signOut: signOutMock }),
}))

vi.mock('@/features/auth', () => ({
  useIdentity: vi.fn(() => ({
    identity: {
      user_id: 'u1',
      email: 'test@test.com',
      display_name: 'Test User',
      status: 'active',
      roles: [],
    },
    isAdmin: false,
    isSignedIn: true,
    isLoaded: true,
  })),
  updateMe: vi.fn(),
  deleteMe: vi.fn(),
}))

vi.mock('sonner', () => ({ toast: { success: vi.fn(), error: vi.fn() } }))

import * as useUserConfigModule from '@/lib/hooks/useUserConfig'
import * as authModule from '@/features/auth'
import { toast } from 'sonner'
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
    refreshIdentityMock.mockResolvedValue(undefined)
    signOutMock.mockResolvedValue(undefined)
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
    expect(screen.getByText(JOB_COMMANDS['jobs.undo'].label)).toBeInTheDocument()
  })

  it('renders the current email', () => {
    mockUserConfig(false)
    render(<AccountPage />, { wrapper: makeWrapper() })
    const instances = screen.getAllByText('test@test.com')
    expect(instances.length).toBeGreaterThan(0)
    instances.forEach((el) => expect(el).toBeInTheDocument())
  })

  // ── Profile save ───────────────────────────────────────────────────────────
  it('saves the profile and refreshes identity on success', async () => {
    mockUserConfig(false)
    vi.mocked(authModule.updateMe).mockResolvedValue(undefined)
    const user = userEvent.setup()
    render(<AccountPage />, { wrapper: makeWrapper() })

    const input = screen.getByLabelText('Display name')
    await user.clear(input)
    await user.type(input, 'New Name')
    await user.click(screen.getByRole('button', { name: 'Save' }))

    expect(vi.mocked(authModule.updateMe)).toHaveBeenCalledWith('New Name')
    expect(vi.mocked(toast.success)).toHaveBeenCalledWith('Profile updated.')
  })

  it('shows an error toast when profile save fails', async () => {
    mockUserConfig(false)
    vi.mocked(authModule.updateMe).mockRejectedValue(new Error('boom'))
    const user = userEvent.setup()
    render(<AccountPage />, { wrapper: makeWrapper() })

    await user.click(screen.getByRole('button', { name: 'Save' }))
    await waitFor(() =>
      expect(vi.mocked(toast.error)).toHaveBeenCalledWith('Failed to update profile.')
    )
  })

  // ── Account deletion ─────────────────────────────────────────────────────────
  it('deletes the account and signs out on success', async () => {
    mockUserConfig(false)
    vi.mocked(authModule.deleteMe).mockResolvedValue(undefined)
    const user = userEvent.setup()
    render(<AccountPage />, { wrapper: makeWrapper() })

    await user.click(screen.getByRole('button', { name: 'Delete account' }))
    await user.click(screen.getByRole('button', { name: 'Delete forever' }))

    await waitFor(() => expect(vi.mocked(authModule.deleteMe)).toHaveBeenCalled())
    expect(signOutMock).toHaveBeenCalled()
  })

  it('shows an error toast when account deletion fails', async () => {
    mockUserConfig(false)
    vi.mocked(authModule.deleteMe).mockRejectedValue(new Error('nope'))
    const user = userEvent.setup()
    render(<AccountPage />, { wrapper: makeWrapper() })

    await user.click(screen.getByRole('button', { name: 'Delete account' }))
    await user.click(screen.getByRole('button', { name: 'Delete forever' }))

    await waitFor(() =>
      expect(vi.mocked(toast.error)).toHaveBeenCalledWith('Failed to delete account.')
    )
    expect(signOutMock).not.toHaveBeenCalled()
  })
})
