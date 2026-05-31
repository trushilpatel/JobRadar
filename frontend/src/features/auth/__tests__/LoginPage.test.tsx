import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter, Route, Routes } from 'react-router-dom'

vi.mock('@/lib/core/supabase', () => ({
  supabase: {
    auth: {
      getSession: vi.fn().mockResolvedValue({ data: { session: null } }),
      onAuthStateChange: vi
        .fn()
        .mockReturnValue({ data: { subscription: { unsubscribe: vi.fn() } } }),
      signInWithOtp: vi.fn(),
      signOut: vi.fn(),
    },
  },
}))

import { LoginPage } from '../components/LoginPage'
import { useAuthStore } from '../store/auth.store'

const signInWithEmail = vi.fn()

function Harness({ initialPath = '/login' }: { initialPath?: string }) {
  return (
    <MemoryRouter initialEntries={[initialPath]}>
      <Routes>
        <Route path="/" element={<div>HOME</div>} />
        <Route path="/login" element={<LoginPage />} />
      </Routes>
    </MemoryRouter>
  )
}

beforeEach(() => {
  signInWithEmail.mockReset()
  useAuthStore.setState({
    session: null,
    identity: null,
    loading: false,
    identityLoading: false,
    initialized: true,
    isAdmin: false,
    signInWithEmail,
  })
})

afterEach(() => {
  vi.clearAllMocks()
})

describe('LoginPage', () => {
  it('renders the sign-in form with a disabled submit button when email is empty', () => {
    render(<Harness />)
    expect(screen.getByPlaceholderText('you@example.com')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /send sign-in link/i })).toBeDisabled()
  })

  it('redirects home when a session already exists', () => {
    useAuthStore.setState({ session: { access_token: 't' } as never, loading: false })
    render(<Harness />)
    expect(screen.getByText('HOME')).toBeInTheDocument()
    expect(screen.queryByPlaceholderText('you@example.com')).not.toBeInTheDocument()
  })

  it('stays on the login form while loading even if a session exists', () => {
    useAuthStore.setState({ session: { access_token: 't' } as never, loading: true })
    render(<Harness />)
    expect(screen.getByPlaceholderText('you@example.com')).toBeInTheDocument()
  })

  it('enables submit once an email is typed and shows the sent confirmation on success', async () => {
    const user = userEvent.setup()
    signInWithEmail.mockResolvedValue(undefined)
    render(<Harness />)

    await user.type(screen.getByPlaceholderText('you@example.com'), 'a@b.com')
    const button = screen.getByRole('button', { name: /send sign-in link/i })
    expect(button).toBeEnabled()

    await user.click(button)

    expect(signInWithEmail).toHaveBeenCalledWith('a@b.com')
    await waitFor(() => expect(screen.getByText(/sign-in link sent to/i)).toBeInTheDocument())
    expect(screen.getByText('a@b.com')).toBeInTheDocument()
  })

  it('returns to the form when Resend is clicked from the sent state', async () => {
    const user = userEvent.setup()
    signInWithEmail.mockResolvedValue(undefined)
    render(<Harness />)

    await user.type(screen.getByPlaceholderText('you@example.com'), 'a@b.com')
    await user.click(screen.getByRole('button', { name: /send sign-in link/i }))
    await waitFor(() => expect(screen.getByText(/sign-in link sent to/i)).toBeInTheDocument())

    await user.click(screen.getByRole('button', { name: /resend/i }))
    expect(screen.getByPlaceholderText('you@example.com')).toBeInTheDocument()
  })

  it('shows the error message when sign-in throws an Error', async () => {
    const user = userEvent.setup()
    signInWithEmail.mockRejectedValue(new Error('rate limited'))
    render(<Harness />)

    await user.type(screen.getByPlaceholderText('you@example.com'), 'a@b.com')
    await user.click(screen.getByRole('button', { name: /send sign-in link/i }))

    await waitFor(() => expect(screen.getByRole('alert')).toHaveTextContent('rate limited'))
    // form is still shown, not the sent confirmation
    expect(screen.getByPlaceholderText('you@example.com')).toBeInTheDocument()
  })

  it('shows a generic message when sign-in throws a non-Error value', async () => {
    const user = userEvent.setup()
    signInWithEmail.mockRejectedValue('boom')
    render(<Harness />)

    await user.type(screen.getByPlaceholderText('you@example.com'), 'a@b.com')
    await user.click(screen.getByRole('button', { name: /send sign-in link/i }))

    await waitFor(() => expect(screen.getByRole('alert')).toHaveTextContent('Sign-in failed'))
  })
})
