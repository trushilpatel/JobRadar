import { describe, expect, it, beforeEach, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import { MemoryRouter, Route, Routes } from 'react-router-dom'
import { RequireAuth } from '../RequireAuth'
import { useAuthStore } from '../store'

vi.mock('@/lib/supabase', () => ({
  supabase: {
    auth: {
      getSession: vi.fn().mockResolvedValue({ data: { session: null } }),
      onAuthStateChange: vi.fn().mockReturnValue({ data: { subscription: { unsubscribe: vi.fn() } } }),
      signInWithOAuth: vi.fn(),
      signOut: vi.fn(),
    },
  },
}))

function Harness({ initialPath }: { initialPath: string }) {
  return (
    <MemoryRouter initialEntries={[initialPath]}>
      <Routes>
        <Route path="/login" element={<div>LOGIN PAGE</div>} />
        <Route element={<RequireAuth />}>
          <Route path="/" element={<div>PROTECTED CONTENT</div>} />
        </Route>
      </Routes>
    </MemoryRouter>
  )
}

beforeEach(() => {
  useAuthStore.setState({ session: null, loading: false, initialized: true })
})

describe('RequireAuth', () => {
  it('redirects to /login when there is no session', () => {
    useAuthStore.setState({ session: null, loading: false })
    render(<Harness initialPath="/" />)
    expect(screen.getByText('LOGIN PAGE')).toBeInTheDocument()
  })

  it('renders children when session is present', () => {
    useAuthStore.setState({
      // Minimal shape; RequireAuth only checks truthiness.
      session: { access_token: 'x', user: { id: 'u' } } as unknown as never,
      loading: false,
    })
    render(<Harness initialPath="/" />)
    expect(screen.getByText('PROTECTED CONTENT')).toBeInTheDocument()
  })

  it('renders nothing while loading', () => {
    useAuthStore.setState({ session: null, loading: true })
    const { container } = render(<Harness initialPath="/" />)
    expect(container.textContent).toBe('')
  })
})
