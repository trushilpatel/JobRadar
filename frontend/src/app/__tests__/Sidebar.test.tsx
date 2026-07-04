import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter } from 'react-router-dom'
import { TooltipProvider } from '@/components/ui/tooltip'
import { Sidebar, MobileSidebarNav } from '@/app/Sidebar'

// ── Configurable Clerk mock ───────────────────────────────────────────────────
const signOut = vi.fn().mockResolvedValue(undefined)

interface MockUserState {
  user: {
    primaryEmailAddress: { emailAddress: string } | null
    publicMetadata: { role?: string }
  } | null
  isLoaded: boolean
}

let userState: MockUserState = {
  user: {
    primaryEmailAddress: { emailAddress: 'jane@example.com' },
    publicMetadata: {},
  },
  isLoaded: true,
}

let isSignedIn = true

vi.mock('@clerk/react', () => ({
  useAuth: () => ({ isSignedIn }),
  useUser: () => userState,
  useClerk: () => ({ signOut }),
}))

function renderSidebar(initialEntries = ['/jobs']) {
  return render(
    <TooltipProvider>
      <MemoryRouter initialEntries={initialEntries}>
        <Sidebar />
      </MemoryRouter>
    </TooltipProvider>
  )
}

function asAdmin() {
  userState = {
    ...userState,
    user: {
      primaryEmailAddress: { emailAddress: 'jane@example.com' },
      publicMetadata: { role: 'admin' },
    },
  }
}

beforeEach(() => {
  signOut.mockClear()
  isSignedIn = true
  userState = {
    user: {
      primaryEmailAddress: { emailAddress: 'jane@example.com' },
      publicMetadata: {},
    },
    isLoaded: true,
  }
})

describe('Sidebar', () => {
  it('renders the user nav (Jobs, Résumés) but no admin links for a non-admin', () => {
    renderSidebar()
    expect(screen.getByRole('link', { name: /Jobs/i })).toBeInTheDocument()
    // Résumés are user-facing — visible to every authenticated user.
    expect(screen.getByRole('link', { name: /Resumes/i })).toBeInTheDocument()
    // Dashboard/Insights/Audit are removed from the nav entirely.
    expect(screen.queryByRole('link', { name: /Dashboard/i })).not.toBeInTheDocument()
    expect(screen.queryByRole('link', { name: /Insights/i })).not.toBeInTheDocument()
    expect(screen.queryByRole('link', { name: /Audit/i })).not.toBeInTheDocument()
    // Admin-only links are hidden for non-admins.
    expect(screen.queryByText('Admin')).not.toBeInTheDocument()
    expect(screen.queryByRole('link', { name: /Companies/i })).not.toBeInTheDocument()
    expect(screen.queryByRole('link', { name: /Users/i })).not.toBeInTheDocument()
    expect(screen.queryByRole('link', { name: /Pipeline/i })).not.toBeInTheDocument()
  })

  it('renders the Admin section (Companies, Users, Pipeline) for an admin', () => {
    asAdmin()
    renderSidebar()
    expect(screen.getByText('Admin')).toBeInTheDocument()
    expect(screen.getByRole('link', { name: /Companies/i })).toBeInTheDocument()
    expect(screen.getByRole('link', { name: /Users/i })).toBeInTheDocument()
    expect(screen.getByRole('link', { name: /Pipeline/i })).toBeInTheDocument()
    // User-facing links remain visible for admins too.
    expect(screen.getByRole('link', { name: /Resumes/i })).toBeInTheDocument()
    // Removed-from-nav routes stay out even for admins.
    expect(screen.queryByRole('link', { name: /Insights/i })).not.toBeInTheDocument()
    expect(screen.queryByRole('link', { name: /Audit/i })).not.toBeInTheDocument()
  })

  it('highlights the active route', () => {
    renderSidebar(['/jobs'])
    const jobsLink = screen.getByRole('link', { name: /Jobs/i })
    expect(jobsLink.className).toContain('text-[var(--accent-text)]')
  })

  it('shows the user email and a sign-out button when expanded', async () => {
    renderSidebar()
    expect(screen.getByText('jane@example.com')).toBeInTheDocument()
    await userEvent.click(screen.getByRole('button', { name: 'Sign out' }))
    expect(signOut).toHaveBeenCalledOnce()
  })

  it('renders the version line', () => {
    renderSidebar()
    expect(screen.getByText('JobRadar · v1.0.0')).toBeInTheDocument()
  })

  it('collapses and expands via the toggle button', async () => {
    renderSidebar()
    // Expanded: email is visible
    expect(screen.getByText('jane@example.com')).toBeInTheDocument()
    await userEvent.click(screen.getByRole('button', { name: 'Collapse sidebar' }))
    // Collapsed: email label is hidden, only the initials avatar link remains
    expect(screen.queryByText('jane@example.com')).not.toBeInTheDocument()
    // Version line collapses to just the version
    expect(screen.getByText('v1.0.0')).toBeInTheDocument()
    // Toggle is now the expand control
    await userEvent.click(screen.getByRole('button', { name: 'Expand sidebar' }))
    expect(screen.getByText('jane@example.com')).toBeInTheDocument()
  })

  it('renders the avatar initial from the email and an account link when collapsed', async () => {
    asAdmin()
    renderSidebar()
    await userEvent.click(screen.getByRole('button', { name: 'Collapse sidebar' }))
    // Initial of "jane@example.com" → "J"
    expect(screen.getByText('J')).toBeInTheDocument()
    // Collapsed footer is an account link
    const accountLink = screen.getByRole('link', { name: 'J' })
    expect(accountLink).toHaveAttribute('href', '/account')
  })

  it('falls back to "?" initial when there is no email', () => {
    isSignedIn = false
    userState = {
      user: null,
      isLoaded: true,
    }
    renderSidebar()
    expect(screen.getByText('?')).toBeInTheDocument()
  })
})

describe('MobileSidebarNav', () => {
  function renderMobile(onNavigate = vi.fn(), initialEntries = ['/jobs']) {
    render(
      <TooltipProvider>
        <MemoryRouter initialEntries={initialEntries}>
          <MobileSidebarNav onNavigate={onNavigate} />
        </MemoryRouter>
      </TooltipProvider>
    )
    return onNavigate
  }

  it('renders the same nav links as the desktop rail', () => {
    asAdmin()
    renderMobile()
    expect(screen.getByRole('link', { name: /Jobs/i })).toBeInTheDocument()
    expect(screen.getByText('Admin')).toBeInTheDocument()
    expect(screen.getByRole('link', { name: /Companies/i })).toBeInTheDocument()
    expect(screen.getByText('JobRadar · v1.0.0')).toBeInTheDocument()
  })

  it('shows the user-facing links for a non-admin (no admin group)', () => {
    renderMobile()
    expect(screen.getByRole('link', { name: /Jobs/i })).toBeInTheDocument()
    expect(screen.getByRole('link', { name: /Resumes/i })).toBeInTheDocument()
    expect(screen.queryByText('Admin')).not.toBeInTheDocument()
  })

  it('calls onNavigate when a link is clicked (closes the drawer)', async () => {
    const onNavigate = renderMobile()
    await userEvent.click(screen.getByRole('link', { name: /Jobs/i }))
    expect(onNavigate).toHaveBeenCalledOnce()
  })
})
