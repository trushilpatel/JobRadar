import { describe, it, expect, vi, afterEach } from 'vitest'
import { render } from '@testing-library/react'
import { MemoryRouter, Route, Routes } from 'react-router-dom'

vi.mock('@clerk/react', () => ({
  useAuth: vi.fn(),
}))

async function renderGuard(isLoaded: boolean, isSignedIn: boolean) {
  const { useAuth } = await import('@clerk/react')
  vi.mocked(useAuth).mockReturnValue({ isSignedIn, isLoaded } as never)
  const { RequireAuth } = await import('../components/RequireAuth')
  return render(
    <MemoryRouter initialEntries={['/protected']}>
      <Routes>
        <Route element={<RequireAuth />}>
          <Route path="/protected" element={<div data-testid="protected" />} />
        </Route>
        <Route path="/login" element={<div data-testid="login-page" />} />
      </Routes>
    </MemoryRouter>
  )
}

afterEach(() => {
  vi.clearAllMocks()
  vi.resetModules()
})

describe('RequireAuth', () => {
  it('renders null while Clerk is loading', async () => {
    const { container } = await renderGuard(false, false)
    expect(container.firstChild).toBeNull()
  })

  it('redirects to /login when not signed in', async () => {
    const { getByTestId } = await renderGuard(true, false)
    expect(getByTestId('login-page')).toBeInTheDocument()
  })

  it('renders outlet when signed in', async () => {
    const { getByTestId } = await renderGuard(true, true)
    expect(getByTestId('protected')).toBeInTheDocument()
  })
})
