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
})
