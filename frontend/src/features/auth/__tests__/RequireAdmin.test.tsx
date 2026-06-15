import { describe, it, expect, vi, afterEach } from 'vitest'
import { render } from '@testing-library/react'
import { MemoryRouter, Route, Routes } from 'react-router-dom'

vi.mock('@clerk/react', () => ({
  useUser: vi.fn(),
}))

async function renderAdminGuard(isLoaded: boolean, role: string | undefined) {
  const { useUser } = await import('@clerk/react')
  vi.mocked(useUser).mockReturnValue({
    isLoaded,
    user: isLoaded ? { publicMetadata: role !== undefined ? { role } : {} } : null,
  } as never)
  const { RequireAdmin } = await import('../components/RequireAdmin')
  return render(
    <MemoryRouter initialEntries={['/admin/users']}>
      <Routes>
        <Route element={<RequireAdmin />}>
          <Route path="/admin/users" element={<div data-testid="admin-content" />} />
        </Route>
        <Route path="/dashboard" element={<div data-testid="dashboard" />} />
      </Routes>
    </MemoryRouter>
  )
}

afterEach(() => {
  vi.clearAllMocks()
  vi.resetModules()
})

describe('RequireAdmin', () => {
  it('renders null while Clerk is loading', async () => {
    const { container } = await renderAdminGuard(false, undefined)
    expect(container.firstChild).toBeNull()
  })

  it('redirects non-admin to /dashboard', async () => {
    const { getByTestId } = await renderAdminGuard(true, 'user')
    expect(getByTestId('dashboard')).toBeInTheDocument()
  })

  it('redirects when publicMetadata has no role', async () => {
    const { getByTestId } = await renderAdminGuard(true, undefined)
    expect(getByTestId('dashboard')).toBeInTheDocument()
  })

  it('renders outlet for admin user', async () => {
    const { getByTestId } = await renderAdminGuard(true, 'admin')
    expect(getByTestId('admin-content')).toBeInTheDocument()
  })
})
