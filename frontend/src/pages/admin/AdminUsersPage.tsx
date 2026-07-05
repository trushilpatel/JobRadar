import { useMemo, useState } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { toast } from 'sonner'
import * as Dialog from '@radix-ui/react-dialog'
import * as Tooltip from '@radix-ui/react-tooltip'
import { Shield, ShieldOff, Trash2, ExternalLink } from 'lucide-react'
import { ErrorBoundary } from '@/components/shared/ErrorBoundary'
import { cn } from '@/lib/utils/utils'
import { useDebouncedValue } from '@/lib/hooks/use-debounced-value'
import { Pagination } from '@/components/ui/pagination'
import { SegmentedSwitch } from '@/components/ui/segmented-switch'
import { useIdentity } from '@/features/auth'
import {
  fetchAdminUsers,
  setUserStatus,
  deleteAdminUser,
  authQueryKeys,
  type AdminUser,
  type AdminUserFilter,
} from '@/features/auth'

function UserAvatar({ name, email }: { name: string | null; email: string }) {
  const src = name ?? email
  const initials = (() => {
    if (!src) return '?'
    const words = src.trim().split(/\s+/).filter(Boolean)
    if (words.length >= 2) {
      return words
        .slice(0, 2)
        .map((w) => w[0]?.toUpperCase() ?? '')
        .join('')
    }
    return (words[0] ?? '').slice(0, 2).toUpperCase()
  })()

  return (
    <div
      className="w-8 h-8 rounded-lg flex items-center justify-center text-xs font-bold text-white shrink-0"
      aria-hidden="true"
      style={{ background: 'linear-gradient(135deg, var(--violet), var(--cyan))' }}
    >
      {initials}
    </div>
  )
}

function StatusBadge({ status }: { status: 'active' | 'suspended' }) {
  const isActive = status === 'active'
  return (
    <span
      className="inline-flex items-center gap-1.5 px-2.5 py-0.5 rounded-full text-[11px] font-semibold whitespace-nowrap"
      style={
        isActive
          ? {
              color: 'var(--ok)',
              background: 'color-mix(in srgb, var(--ok) 12%, transparent)',
              border: '1px solid color-mix(in srgb, var(--ok) 28%, transparent)',
            }
          : {
              color: 'var(--danger)',
              background: 'color-mix(in srgb, var(--danger) 10%, transparent)',
              border: '1px solid color-mix(in srgb, var(--danger) 26%, transparent)',
            }
      }
    >
      <span
        className="w-1.5 h-1.5 rounded-full shrink-0"
        style={{ background: isActive ? 'var(--ok)' : 'var(--danger)' }}
        aria-hidden="true"
      />
      {status}
    </span>
  )
}

function ClerkRoleNote() {
  return (
    <Tooltip.Provider delayDuration={200}>
      <Tooltip.Root>
        <Tooltip.Trigger asChild>
          <a
            href="https://dashboard.clerk.com"
            target="_blank"
            rel="noopener noreferrer"
            className="inline-flex items-center gap-1 text-xs text-muted hover:text-primary transition-colors"
          >
            via Clerk
            <ExternalLink size={10} />
          </a>
        </Tooltip.Trigger>
        <Tooltip.Portal>
          <Tooltip.Content
            className="z-50 max-w-xs rounded-md bg-panel border border-[var(--border)] px-3 py-2 text-xs text-secondary shadow-lg"
            sideOffset={4}
          >
            Admin roles are managed in Clerk Dashboard → Users → publicMetadata{' '}
            <code className="text-violet-400">{'{"role":"admin"}'}</code>. Changes take effect on
            the user&apos;s next sign-in.
            <Tooltip.Arrow className="fill-[var(--border)]" />
          </Tooltip.Content>
        </Tooltip.Portal>
      </Tooltip.Root>
    </Tooltip.Provider>
  )
}

function DeleteDialog({ user, onClose }: { user: AdminUser | null; onClose: () => void }) {
  const qc = useQueryClient()
  const { mutate, isPending } = useMutation({
    mutationFn: () => deleteAdminUser(user!.id),
    onSuccess: () => {
      toast.success(`Deleted ${user!.email}`)
      qc.invalidateQueries({ queryKey: authQueryKeys.adminUsersBase() })
      onClose()
    },
    onError: () => toast.error('Failed to delete user'),
  })

  return (
    <Dialog.Root
      open={!!user}
      onOpenChange={(open) => {
        if (!open) onClose()
      }}
    >
      <Dialog.Portal>
        <Dialog.Overlay className="fixed inset-0 bg-black/60 z-40" />
        <Dialog.Content className="fixed left-1/2 top-1/2 -translate-x-1/2 -translate-y-1/2 z-50 w-full max-w-sm bg-panel border border-[var(--border)] rounded-xl p-6 shadow-xl">
          <Dialog.Title className="text-base font-semibold text-primary mb-2">
            Delete {user?.email}?
          </Dialog.Title>
          <Dialog.Description className="text-sm text-muted mb-6">
            Permanently deletes the Clerk auth record and all DB data for this user. Cannot be
            undone.
          </Dialog.Description>
          <div className="flex justify-end gap-3">
            <Dialog.Close asChild>
              <button className="px-4 py-2 rounded-md text-sm text-muted hover:text-primary hover:bg-hover transition-colors">
                Cancel
              </button>
            </Dialog.Close>
            <button
              onClick={() => mutate()}
              disabled={isPending}
              className="px-4 py-2 rounded-md text-sm font-medium bg-red-700 hover:bg-red-600 disabled:opacity-50 text-white transition-colors"
            >
              {isPending ? 'Deleting…' : 'Delete forever'}
            </button>
          </div>
        </Dialog.Content>
      </Dialog.Portal>
    </Dialog.Root>
  )
}

function UserRow({
  user,
  selfId,
  onDeleteRequest,
}: {
  user: AdminUser
  selfId: string | undefined
  onDeleteRequest: (user: AdminUser) => void
}) {
  const qc = useQueryClient()
  const isSelf = selfId !== undefined && user.id === selfId

  const { mutate: toggleStatus, isPending: togglingStatus } = useMutation({
    mutationFn: () => setUserStatus(user.id, user.status === 'active' ? 'suspended' : 'active'),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: authQueryKeys.adminUsersBase() })
      toast.success(
        user.status === 'active' ? `${user.email} suspended` : `${user.email} reinstated`
      )
    },
    onError: () => toast.error('Failed to update status'),
  })

  const selfTitle = 'Cannot modify your own account'

  return (
    <>
      <tr className="border-b border-[var(--border)] hover:bg-hover transition-colors">
        <td className="px-4 py-3">
          <div className="flex items-center gap-2.5">
            <UserAvatar name={user.display_name} email={user.email} />
            <div>
              <div className="flex items-center gap-1.5">
                <div className="text-sm text-primary">{user.email}</div>
                {isSelf && (
                  <span className="text-xs text-muted bg-subtle px-1.5 py-0.5 rounded">you</span>
                )}
              </div>
              {user.display_name && (
                <div className="text-xs text-muted mt-0.5">{user.display_name}</div>
              )}
            </div>
          </div>
        </td>
        <td className="px-4 py-3">
          <StatusBadge status={user.status} />
        </td>
        <td className="px-4 py-3">
          <ClerkRoleNote />
        </td>
        <td className="px-4 py-3 text-xs text-muted">
          {new Date(user.created_at).toLocaleDateString()}
        </td>
        <td className="px-4 py-3">
          <div className="flex items-center gap-1">
            <Tooltip.Provider delayDuration={200}>
              <Tooltip.Root>
                <Tooltip.Trigger asChild>
                  <a
                    href="https://dashboard.clerk.com"
                    target="_blank"
                    rel="noopener noreferrer"
                    className="p-1.5 rounded-md transition-colors text-muted hover:text-violet-600 dark:hover:text-violet-400 hover:bg-hover inline-flex"
                    aria-label="Manage admin role in Clerk"
                  >
                    <Shield size={14} />
                  </a>
                </Tooltip.Trigger>
                <Tooltip.Portal>
                  <Tooltip.Content
                    className="z-50 rounded-md bg-panel border border-[var(--border)] px-3 py-2 text-xs text-secondary shadow-lg"
                    sideOffset={4}
                  >
                    Admin roles are managed in Clerk Dashboard
                    <Tooltip.Arrow className="fill-[var(--border)]" />
                  </Tooltip.Content>
                </Tooltip.Portal>
              </Tooltip.Root>
            </Tooltip.Provider>
            <button
              onClick={() => toggleStatus()}
              disabled={togglingStatus || isSelf}
              title={isSelf ? selfTitle : user.status === 'active' ? 'Suspend' : 'Reinstate'}
              className={cn(
                'p-1.5 rounded-md transition-colors',
                isSelf
                  ? 'text-placeholder cursor-not-allowed'
                  : user.status === 'suspended'
                    ? 'text-amber-500 dark:text-amber-400 hover:bg-amber-500/10'
                    : 'text-muted hover:text-amber-600 dark:hover:text-amber-400 hover:bg-hover'
              )}
            >
              <ShieldOff size={14} />
            </button>
            <button
              onClick={() => !isSelf && onDeleteRequest(user)}
              disabled={isSelf}
              title={isSelf ? selfTitle : 'Delete user'}
              className={cn(
                'p-1.5 rounded-md transition-colors',
                isSelf
                  ? 'text-placeholder cursor-not-allowed'
                  : 'text-muted hover:text-red-500 dark:hover:text-red-400 hover:bg-red-500/10'
              )}
            >
              <Trash2 size={14} />
            </button>
          </div>
        </td>
      </tr>
    </>
  )
}

export function AdminUsersPage() {
  const { identity } = useIdentity()
  const selfId = identity?.user_id
  const [emailInput, setEmailInput] = useState('')
  const debouncedEmail = useDebouncedValue(emailInput, 300)
  const [statusFilter, setStatusFilter] = useState<AdminUserFilter['status']>(undefined)
  const [page, setPage] = useState(1)
  const [limit, setLimit] = useState(20)
  const [deleteTarget, setDeleteTarget] = useState<AdminUser | null>(null)

  const filter = useMemo<AdminUserFilter>(
    () => ({
      email: debouncedEmail.trim() || undefined,
      status: statusFilter,
    }),
    [debouncedEmail, statusFilter]
  )

  // Reset page on filter or limit change (in-render derived-state pattern).
  const [prevKey, setPrevKey] = useState({ filter, limit })
  if (prevKey.filter !== filter || prevKey.limit !== limit) {
    setPrevKey({ filter, limit })
    setPage(1)
  }

  const { data, isLoading, isError, refetch } = useQuery({
    queryKey: authQueryKeys.adminUsers(filter, page, limit),
    queryFn: () => fetchAdminUsers(filter, page, limit),
    staleTime: 30_000,
  })

  const users = data?.data ?? []
  const total = data?.total ?? 0

  return (
    <div className="flex flex-col h-full bg-panel overflow-y-auto">
      <div className="border-b border-[var(--border)] px-6 py-4 shrink-0">
        <h1
          className="text-2xl font-semibold tracking-tight"
          style={{
            background:
              'linear-gradient(100deg, var(--text-primary) 38%, var(--violet-bright) 86%, var(--cyan))',
            WebkitBackgroundClip: 'text',
            WebkitTextFillColor: 'transparent',
            backgroundClip: 'text',
          }}
        >
          Users
        </h1>
      </div>

      {/* Filters */}
      <div className="px-6 py-3 border-b border-[var(--border)] flex items-center gap-3 shrink-0">
        <input
          type="text"
          placeholder="Filter by email…"
          value={emailInput}
          onChange={(e) => setEmailInput(e.target.value)}
          className="w-56 rounded-full bg-[var(--bg-panel)] border border-[var(--border-strong)] px-4 py-1.5 text-sm text-primary placeholder:text-placeholder focus:outline-none focus:border-[var(--violet)] focus:ring-2 focus:ring-[color-mix(in_srgb,var(--violet)_14%,transparent)]"
        />
        <SegmentedSwitch
          aria-label="Status filter"
          value={statusFilter ?? 'all'}
          onChange={(v) =>
            setStatusFilter(v === 'all' ? undefined : (v as AdminUserFilter['status']))
          }
          options={[
            { value: 'all', label: 'All', accent: 'violet' },
            { value: 'active', label: 'Active', accent: 'emerald' },
            { value: 'suspended', label: 'Suspended', accent: 'violet' },
          ]}
        />
        <span className="text-xs text-muted ml-auto">
          {total} user{total !== 1 ? 's' : ''}
        </span>
      </div>

      {isError && (
        <div className="flex flex-col items-center justify-center py-16 gap-3 text-secondary">
          <p className="text-sm">Failed to load users.</p>
          <button
            type="button"
            onClick={() => refetch()}
            className="text-xs text-violet-400 hover:text-violet-300 underline"
          >
            Try again
          </button>
        </div>
      )}

      {/* Table */}
      <ErrorBoundary>
        <div className="flex-1 overflow-auto">
          {isLoading ? (
            <div className="flex items-center justify-center h-32 text-muted text-sm">Loading…</div>
          ) : users.length === 0 ? (
            <div className="flex items-center justify-center h-32 text-muted text-sm">
              No users found.
            </div>
          ) : (
            <table className="w-full text-sm">
              <thead>
                <tr className="border-b border-[var(--border)] text-xs text-muted text-left">
                  <th className="px-4 py-2 font-medium">Email</th>
                  <th className="px-4 py-2 font-medium">Status</th>
                  <th className="px-4 py-2 font-medium">Admin</th>
                  <th className="px-4 py-2 font-medium">Joined</th>
                  <th className="px-4 py-2 font-medium">Actions</th>
                </tr>
              </thead>
              <tbody>
                {users.map((u) => (
                  <UserRow key={u.id} user={u} selfId={selfId} onDeleteRequest={setDeleteTarget} />
                ))}
              </tbody>
            </table>
          )}
        </div>
      </ErrorBoundary>

      <div className="border-t border-[var(--border)] shrink-0">
        <Pagination
          mode="offset"
          page={page}
          total={total}
          limit={limit}
          onPageChange={setPage}
          onLimitChange={setLimit}
          loading={isLoading}
        />
      </div>

      <DeleteDialog user={deleteTarget} onClose={() => setDeleteTarget(null)} />
    </div>
  )
}
