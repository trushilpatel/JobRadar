import { describe, expect, it, vi, beforeEach, afterEach } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import { MemoryRouter, Route, Routes } from 'react-router-dom'

const exchangeCodeForSession = vi.fn()
const getSession = vi.fn()

vi.mock('@/lib/core/supabase', () => ({
  supabase: {
    auth: {
      exchangeCodeForSession: (...args: unknown[]) => exchangeCodeForSession(...args),
      getSession: (...args: unknown[]) => getSession(...args),
    },
  },
}))

import { AuthCallback } from '../components/AuthCallback'

const originalLocation = window.location

function mockLocation(search: string, hash = '') {
  Object.defineProperty(window, 'location', {
    configurable: true,
    value: {
      ...originalLocation,
      search,
      hash,
      href: `http://localhost/auth/callback${search}${hash}`,
      pathname: '/auth/callback',
      origin: 'http://localhost',
    },
  })
}

function Harness() {
  return (
    <MemoryRouter initialEntries={['/auth/callback?code=abc']}>
      <Routes>
        <Route path="/" element={<div>HOME</div>} />
        <Route path="/login" element={<div>LOGIN</div>} />
        <Route path="/auth/callback" element={<AuthCallback />} />
      </Routes>
    </MemoryRouter>
  )
}

beforeEach(() => {
  exchangeCodeForSession.mockReset()
  getSession.mockReset()
  vi.spyOn(window.history, 'replaceState')
})

afterEach(() => {
  Object.defineProperty(window, 'location', { configurable: true, value: originalLocation })
  vi.restoreAllMocks()
})

describe('AuthCallback', () => {
  it('strips fragment and navigates home on success', async () => {
    mockLocation('?code=abc')
    exchangeCodeForSession.mockResolvedValue({ data: { session: {} }, error: null })

    render(<Harness />)

    await waitFor(() => expect(exchangeCodeForSession).toHaveBeenCalled())
    await waitFor(() => {
      expect(window.history.replaceState).toHaveBeenCalledWith({}, '', '/')
    })
    await waitFor(() => expect(screen.getByText('HOME')).toBeInTheDocument())
  })

  it('shows error on failure with link back to /login', async () => {
    mockLocation('?code=abc')
    exchangeCodeForSession.mockResolvedValue({ data: null, error: { message: 'bad code' } })

    render(<Harness />)

    await waitFor(() => expect(screen.getByRole('alert')).toHaveTextContent('bad code'))
    const link = screen.getByRole('link', { name: /back to sign in/i })
    expect(link).toHaveAttribute('href', '/login')
  })

  it('falls back to a generic message when the code exchange error has no message', async () => {
    mockLocation('?code=abc')
    exchangeCodeForSession.mockResolvedValue({ data: null, error: { message: '' } })

    render(<Harness />)

    await waitFor(() =>
      expect(screen.getByRole('alert')).toHaveTextContent('Authentication failed')
    )
  })

  it('succeeds via the hash-token path when a session is present', async () => {
    mockLocation('', '#access_token=xyz')
    getSession.mockResolvedValue({ data: { session: {} }, error: null })

    render(<Harness />)

    await waitFor(() => expect(getSession).toHaveBeenCalled())
    await waitFor(() => expect(screen.getByText('HOME')).toBeInTheDocument())
  })

  it('errors on the hash-token path when getSession returns an error', async () => {
    mockLocation('', '#error=denied')
    getSession.mockResolvedValue({ data: { session: null }, error: { message: 'denied' } })

    render(<Harness />)

    await waitFor(() => expect(screen.getByRole('alert')).toHaveTextContent('denied'))
  })

  it('errors on the hash-token path when no session is returned', async () => {
    mockLocation('', '#access_token=xyz')
    getSession.mockResolvedValue({ data: { session: null }, error: null })

    render(<Harness />)

    await waitFor(() => expect(screen.getByRole('alert')).toHaveTextContent('No session returned'))
  })

  it('falls back to checking an existing session when neither code nor hash present', async () => {
    mockLocation('', '')
    getSession.mockResolvedValue({ data: { session: {} } })

    render(<Harness />)

    await waitFor(() => expect(screen.getByText('HOME')).toBeInTheDocument())
  })

  it('errors with "Missing authorization code" when no code, no hash, and no session', async () => {
    mockLocation('', '')
    getSession.mockResolvedValue({ data: { session: null } })

    render(<Harness />)

    await waitFor(() =>
      expect(screen.getByRole('alert')).toHaveTextContent('Missing authorization code')
    )
  })
})
