/**
 * Cache seam tests for AdminUsersPage mutations.
 *
 * Contract: every mutation (toggleAdmin, toggleStatus, deleteUser) must
 * invalidate using authQueryKeys.adminUsersBase(), not an inline string.
 * If the factory root key changes, both the mutation code and this test
 * must be updated together.
 */
import React from 'react'
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, fireEvent, waitFor, screen } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { MemoryRouter } from 'react-router-dom'
import { authQueryKeys } from '@/features/auth'

// ── API mocks ─────────────────────────────────────────────────────────────────
// Mock the deep module — the barrel re-exports without transformation, so the
// page (which imports via the barrel) gets the same mock instance.
// We import for vi.mocked() via the barrel (architecture rule: no deep imports).
vi.mock('@/features/auth/api/adminUsers.api')
vi.mock('sonner', () => ({
  toast: {
    success: vi.fn(),
    error: vi.fn(),
  },
}))

// ── Auth identity mock ────────────────────────────────────────────────────────
vi.mock('@/features/auth/hooks/useIdentity', () => ({
  useIdentity: vi.fn(() => ({
    identity: { user_id: 'admin-self', email: 'admin@test.com', roles: ['admin'] },
    isAdmin: true,
    isSignedIn: true,
    isLoaded: true,
  })),
}))

// Import via feature barrel to satisfy the no-restricted-imports rule.
// vi.mocked resolves to the same mock instance because the barrel re-exports
// the same module that was mocked above.
import * as adminAuth from '@/features/auth'
import { AdminUsersPage } from '@/pages/admin/AdminUsersPage'

// ── Test data ─────────────────────────────────────────────────────────────────
const baseUser = {
  id: 'user-1',
  email: 'alice@example.com',
  display_name: 'Alice',
  status: 'active' as const,
  roles: ['user'],
  created_at: '2024-01-01T00:00:00Z',
}

function makeUserListResult() {
  return { data: [baseUser], total: 1, page: 1, limit: 20 }
}

let lastQc: QueryClient

function makeWrapper() {
  lastQc = new QueryClient({
    defaultOptions: {
      queries: { retry: 0, gcTime: 5 * 60_000 },
      mutations: { retry: 0 },
    },
  })
  const qc = lastQc
  return ({ children }: { children: React.ReactNode }) => (
    <MemoryRouter>
      <QueryClientProvider client={qc}>{children}</QueryClientProvider>
    </MemoryRouter>
  )
}

// ─────────────────────────────────────────────────────────────────────────────
// Cache seam — invalidation contracts
// ─────────────────────────────────────────────────────────────────────────────

describe('AdminUsersPage cache seam — invalidation contracts', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  // NOTE: in-app admin granting was removed in the Clerk migration — roles are
  // managed in the Clerk dashboard (the page now shows a "Manage admin role in
  // Clerk" link, no Grant/Revoke button), so there is no toggleAdmin seam to test.

  it('toggleStatus invalidates authQueryKeys.adminUsersBase() on success', async () => {
    vi.mocked(adminAuth.fetchAdminUsers).mockResolvedValue(makeUserListResult())
    vi.mocked(adminAuth.setUserStatus).mockResolvedValue(undefined)

    render(<AdminUsersPage />, { wrapper: makeWrapper() })
    const invalidateSpy = vi.spyOn(lastQc, 'invalidateQueries')

    await screen.findByText('alice@example.com')

    const suspendBtn = screen.getByTitle('Suspend')
    fireEvent.click(suspendBtn)

    await waitFor(() => {
      expect(adminAuth.setUserStatus).toHaveBeenCalled()
    })

    const calledKeys = invalidateSpy.mock.calls.map((c) => c[0]?.queryKey)
    expect(calledKeys).toContainEqual(authQueryKeys.adminUsersBase())
  })

  it('deleteUser invalidates authQueryKeys.adminUsersBase() on success', async () => {
    vi.mocked(adminAuth.fetchAdminUsers).mockResolvedValue(makeUserListResult())
    vi.mocked(adminAuth.deleteAdminUser).mockResolvedValue(undefined)

    render(<AdminUsersPage />, { wrapper: makeWrapper() })
    const invalidateSpy = vi.spyOn(lastQc, 'invalidateQueries')

    await screen.findByText('alice@example.com')

    // Open delete dialog
    const deleteBtn = screen.getByTitle('Delete user')
    fireEvent.click(deleteBtn)

    // Confirm deletion
    const confirmBtn = screen.getByRole('button', { name: /delete forever/i })
    fireEvent.click(confirmBtn)

    await waitFor(() => {
      expect(adminAuth.deleteAdminUser).toHaveBeenCalled()
    })

    const calledKeys = invalidateSpy.mock.calls.map((c) => c[0]?.queryKey)
    expect(calledKeys).toContainEqual(authQueryKeys.adminUsersBase())
  })

  it('invalidation uses factory key — not a raw inline string', () => {
    // Guard: the factory root must equal the literal used by invalidateQueries.
    // If someone edits queryKeys.ts and changes the root, this catches the drift.
    expect(authQueryKeys.adminUsersBase()).toEqual(['admin-users'])
  })
})
