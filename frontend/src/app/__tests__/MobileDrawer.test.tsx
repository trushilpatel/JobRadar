import { useState } from 'react'
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { Menu } from 'lucide-react'
import { MemoryRouter } from 'react-router-dom'
import { TooltipProvider } from '@/components/ui/tooltip'
import { Sheet, SheetContent, SheetTitle } from '@/components/ui/sheet'
import { MobileSidebarNav } from '@/app/Sidebar'

// Clerk mock — admin so the nav renders its full set inside the drawer.
const signOut = vi.fn().mockResolvedValue(undefined)
vi.mock('@clerk/react', () => ({
  useAuth: () => ({ isSignedIn: true }),
  useUser: () => ({
    user: {
      primaryEmailAddress: { emailAddress: 'jane@example.com' },
      publicMetadata: { role: 'admin' },
    },
    isLoaded: true,
  }),
  useClerk: () => ({ signOut }),
}))

beforeEach(() => {
  signOut.mockClear()
})

/**
 * Mirrors the mobile drawer wiring inside router.tsx's ProtectedLayout: a
 * hamburger that opens a left Sheet whose body is the shared MobileSidebarNav,
 * which closes on navigation. Kept here so the Sheet/Radix integration is
 * exercised without mounting every page route.
 */
function MobileDrawerHarness() {
  const [open, setOpen] = useState(false)
  return (
    <div>
      <Sheet open={open} onOpenChange={setOpen}>
        <SheetContent side="left">
          <SheetTitle className="sr-only">Navigation</SheetTitle>
          <MobileSidebarNav onNavigate={() => setOpen(false)} />
        </SheetContent>
      </Sheet>
      <button type="button" onClick={() => setOpen(true)} aria-label="Open menu">
        <Menu size={18} />
      </button>
    </div>
  )
}

function renderHarness() {
  return render(
    <TooltipProvider>
      <MemoryRouter initialEntries={['/jobs']}>
        <MobileDrawerHarness />
      </MemoryRouter>
    </TooltipProvider>
  )
}

describe('Mobile drawer', () => {
  it('is closed initially — nav links are not in the document', () => {
    renderHarness()
    expect(screen.queryByRole('link', { name: /Jobs/i })).not.toBeInTheDocument()
  })

  it('opens the Sheet with the shared nav when the hamburger is clicked', async () => {
    renderHarness()
    await userEvent.click(screen.getByRole('button', { name: 'Open menu' }))
    // Drawer content (a Radix Dialog) is now mounted with the shared nav.
    expect(await screen.findByRole('dialog')).toBeInTheDocument()
    expect(screen.getByRole('link', { name: /Jobs/i })).toBeInTheDocument()
    expect(screen.getByText('Admin')).toBeInTheDocument()
  })

  it('closes the Sheet when a nav link is clicked', async () => {
    renderHarness()
    await userEvent.click(screen.getByRole('button', { name: 'Open menu' }))
    const jobsLink = await screen.findByRole('link', { name: /Jobs/i })
    await userEvent.click(jobsLink)
    // onNavigate -> setOpen(false) unmounts the drawer content.
    expect(screen.queryByRole('link', { name: /Jobs/i })).not.toBeInTheDocument()
  })
})
