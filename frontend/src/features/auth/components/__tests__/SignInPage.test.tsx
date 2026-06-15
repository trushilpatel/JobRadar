import { describe, it, expect, vi, afterEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'

vi.mock('@clerk/react', () => ({
  SignIn: ({ fallbackRedirectUrl }: { fallbackRedirectUrl: string }) => (
    <div data-testid="clerk-sign-in" data-fallback-redirect={fallbackRedirectUrl}>
      Clerk SignIn
    </div>
  ),
  useAuth: vi.fn(() => ({ isSignedIn: false, isLoaded: true })),
}))

vi.mock('@/components/brand/Logo', () => ({
  LogoLockup: ({ size }: { size: string }) => <div data-testid={`logo-${size}`} />,
}))

afterEach(() => {
  vi.clearAllMocks()
})

describe('SignInPage', () => {
  it('renders Clerk SignIn component', async () => {
    const { SignInPage } = await import('../SignInPage')
    render(
      <MemoryRouter>
        <SignInPage />
      </MemoryRouter>
    )
    expect(screen.getByTestId('clerk-sign-in')).toBeInTheDocument()
  })

  it('passes redirectUrl="/" to SignIn', async () => {
    const { SignInPage } = await import('../SignInPage')
    render(
      <MemoryRouter>
        <SignInPage />
      </MemoryRouter>
    )
    expect(screen.getByTestId('clerk-sign-in')).toHaveAttribute('data-fallback-redirect', '/')
  })

  it('renders logo', async () => {
    const { SignInPage } = await import('../SignInPage')
    render(
      <MemoryRouter>
        <SignInPage />
      </MemoryRouter>
    )
    expect(screen.getByTestId('logo-lg')).toBeInTheDocument()
  })

  it('redirects to / when already signed in', async () => {
    const { useAuth } = await import('@clerk/react')
    vi.mocked(useAuth).mockReturnValue({ isSignedIn: true, isLoaded: true } as never)
    const { SignInPage } = await import('../SignInPage')
    const { container } = render(
      <MemoryRouter initialEntries={['/login']}>
        <SignInPage />
      </MemoryRouter>
    )
    // Should show Navigate (no clerk-sign-in visible)
    expect(container.querySelector('[data-testid="clerk-sign-in"]')).not.toBeInTheDocument()
  })
})
