import { describe, it, expect, vi, afterEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'

vi.mock('@clerk/react', () => ({
  useAuth: vi.fn(() => ({ isSignedIn: false, isLoaded: true })),
  SignIn: () => <div data-testid="clerk-sign-in" />,
}))

afterEach(() => {
  vi.clearAllMocks()
})

describe('SignInPage', () => {
  it('renders the value panel headline', async () => {
    const { SignInPage } = await import('../SignInPage')
    render(
      <MemoryRouter>
        <SignInPage />
      </MemoryRouter>
    )
    expect(screen.getByText(/every job that matters/i)).toBeInTheDocument()
  })

  it('renders the Clerk sign-in widget', async () => {
    const { SignInPage } = await import('../SignInPage')
    render(
      <MemoryRouter>
        <SignInPage />
      </MemoryRouter>
    )
    expect(screen.getByTestId('clerk-sign-in')).toBeInTheDocument()
  })

  it('renders the logo header', async () => {
    const { SignInPage } = await import('../SignInPage')
    render(
      <MemoryRouter>
        <SignInPage />
      </MemoryRouter>
    )
    expect(screen.getAllByText('JobRadar').length).toBeGreaterThan(0)
  })

  it('redirects to / when already signed in', async () => {
    const { useAuth } = await import('@clerk/react')
    vi.mocked(useAuth).mockReturnValueOnce({ isSignedIn: true, isLoaded: true } as never)
    const { SignInPage } = await import('../SignInPage')
    const { container } = render(
      <MemoryRouter initialEntries={['/login']}>
        <SignInPage />
      </MemoryRouter>
    )
    expect(container.querySelector('[data-testid="sign-in-panel"]')).not.toBeInTheDocument()
  })

  it('does not redirect while Clerk is still loading', async () => {
    const { useAuth } = await import('@clerk/react')
    vi.mocked(useAuth).mockReturnValueOnce({ isSignedIn: true, isLoaded: false } as never)
    const { SignInPage } = await import('../SignInPage')
    render(
      <MemoryRouter initialEntries={['/login']}>
        <SignInPage />
      </MemoryRouter>
    )
    expect(screen.getByTestId('sign-in-panel')).toBeInTheDocument()
  })
})
