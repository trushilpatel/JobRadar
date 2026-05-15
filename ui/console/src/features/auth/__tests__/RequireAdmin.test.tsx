import { describe, expect, it, beforeEach, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import { MemoryRouter, Route, Routes } from 'react-router-dom'
import { RequireAdmin } from '../RequireAdmin'
import { useAuthStore } from '../store'

vi.mock('@/lib/supabase', () => ({
  supabase: {
    auth: {
      getSession: vi.fn().mockResolvedValue({ data: { session: null } }),
      onAuthStateChange: vi
        .fn()
        .mockReturnValue({ data: { subscription: { unsubscribe: vi.fn() } } }),
      signOut: vi.fn(),
    },
  },
}))

// ── Harness: wraps RequireAdmin in a minimal router ───────────────────────────
function Harness({ initialPath }: { initialPath: string }) {
  return (
    <MemoryRouter initialEntries={[initialPath]}>
      <Routes>
        <Route path="/dashboard" element={<div>DASHBOARD</div>} />
        <Route element={<RequireAdmin />}>
          <Route path="/admin" element={<div>ADMIN CONTENT</div>} />
        </Route>
      </Routes>
    </MemoryRouter>
  )
}

beforeEach(() => {
  useAuthStore.setState({ isAdmin: false, loading: false, initialized: true })
})

describe('RequireAdmin', () => {
  it('renders children when user is admin', () => {
    useAuthStore.setState({ isAdmin: true, loading: false })
    render(<Harness initialPath="/admin" />)
    expect(screen.getByText('ADMIN CONTENT')).toBeInTheDocument()
  })

  it('redirects to /dashboard when user is not admin', () => {
    useAuthStore.setState({ isAdmin: false, loading: false })
    render(<Harness initialPath="/admin" />)
    expect(screen.getByText('DASHBOARD')).toBeInTheDocument()
    expect(screen.queryByText('ADMIN CONTENT')).not.toBeInTheDocument()
  })

  it('renders nothing while loading', () => {
    useAuthStore.setState({ isAdmin: false, loading: true })
    const { container } = render(<Harness initialPath="/admin" />)
    expect(container.textContent).toBe('')
  })

  it('does not render admin content for non-admin user even if session exists', () => {
    useAuthStore.setState({
      isAdmin: false,
      loading: false,
      session: { access_token: 'tok', user: { id: 'u1' } } as unknown as never,
    })
    render(<Harness initialPath="/admin" />)
    expect(screen.queryByText('ADMIN CONTENT')).not.toBeInTheDocument()
    expect(screen.getByText('DASHBOARD')).toBeInTheDocument()
  })
})
