import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter } from 'react-router-dom'
import { TooltipProvider } from '@/components/ui/tooltip'
import { Sidebar } from '@/app/Sidebar'

// ── Configurable auth-store mock (Sidebar imports from the feature barrel) ────
interface MockAuthState {
  session: { user?: { email?: string } } | null
  isAdmin: boolean
  signOut: () => void
}

const signOut = vi.fn()
let authState: MockAuthState = {
  session: { user: { email: 'jane@example.com' } },
  isAdmin: false,
  signOut,
}

vi.mock('@/features/auth', () => ({
  useAuthStore: (selector: (s: MockAuthState) => unknown) => selector(authState),
}))

function renderSidebar(initialEntries = ['/dashboard']) {
  return render(
    <TooltipProvider>
      <MemoryRouter initialEntries={initialEntries}>
        <Sidebar />
      </MemoryRouter>
    </TooltipProvider>
  )
}

beforeEach(() => {
  signOut.mockClear()
  authState = {
    session: { user: { email: 'jane@example.com' } },
    isAdmin: false,
    signOut,
  }
})

describe('Sidebar', () => {
  it('renders user nav links but not admin links for a non-admin', () => {
    renderSidebar()
    expect(screen.getByRole('link', { name: /Dashboard/i })).toBeInTheDocument()
    expect(screen.getByRole('link', { name: /Jobs/i })).toBeInTheDocument()
    expect(screen.queryByRole('link', { name: /Companies/i })).not.toBeInTheDocument()
    expect(screen.queryByRole('link', { name: /Users/i })).not.toBeInTheDocument()
  })

  it('renders the Admin section and admin links for an admin', () => {
    authState.isAdmin = true
    renderSidebar()
    expect(screen.getByText('Admin')).toBeInTheDocument()
    expect(screen.getByRole('link', { name: /Companies/i })).toBeInTheDocument()
    expect(screen.getByRole('link', { name: /Users/i })).toBeInTheDocument()
    expect(screen.getByRole('link', { name: /Resumes/i })).toBeInTheDocument()
    expect(screen.getByRole('link', { name: /Apply Queue/i })).toBeInTheDocument()
    expect(screen.getByRole('link', { name: /Audit Log/i })).toBeInTheDocument()
    expect(screen.getByRole('link', { name: /Insights/i })).toBeInTheDocument()
    expect(screen.getByRole('link', { name: /Pipeline/i })).toBeInTheDocument()
  })

  it('highlights the active route', () => {
    renderSidebar(['/jobs'])
    const jobsLink = screen.getByRole('link', { name: /Jobs/i })
    expect(jobsLink.className).toContain('text-violet-300')
  })

  it('shows the user email and a sign-out button when expanded', async () => {
    renderSidebar()
    expect(screen.getByText('jane@example.com')).toBeInTheDocument()
    await userEvent.click(screen.getByRole('button', { name: 'Sign out' }))
    expect(signOut).toHaveBeenCalledOnce()
  })

  it('collapses and expands via the toggle button', async () => {
    renderSidebar()
    // Expanded: email is visible
    expect(screen.getByText('jane@example.com')).toBeInTheDocument()
    await userEvent.click(screen.getByRole('button', { name: 'Collapse sidebar' }))
    // Collapsed: email label is hidden, only the initials avatar link remains
    expect(screen.queryByText('jane@example.com')).not.toBeInTheDocument()
    // Toggle is now the expand control
    await userEvent.click(screen.getByRole('button', { name: 'Expand sidebar' }))
    expect(screen.getByText('jane@example.com')).toBeInTheDocument()
  })

  it('renders the avatar initial from the email and an account link when collapsed', async () => {
    authState.isAdmin = true
    renderSidebar()
    await userEvent.click(screen.getByRole('button', { name: 'Collapse sidebar' }))
    // Initial of "jane@example.com" → "J"
    expect(screen.getByText('J')).toBeInTheDocument()
    // Collapsed footer is an account link
    const accountLink = screen.getByRole('link', { name: 'J' })
    expect(accountLink).toHaveAttribute('href', '/account')
  })

  it('falls back to "?" initial when there is no email', () => {
    authState.session = null
    renderSidebar()
    expect(screen.getByText('?')).toBeInTheDocument()
  })
})
