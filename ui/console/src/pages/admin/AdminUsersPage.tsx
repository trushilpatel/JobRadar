import { useMemo, useState } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { toast } from 'sonner'
import * as Dialog from '@radix-ui/react-dialog'
import { Shield, ShieldOff, Trash2 } from 'lucide-react'
import { cn } from '@/lib/utils/utils'
import { useDebouncedValue } from '@/lib/hooks/use-debounced-value'
import { Pagination } from '@/components/ui/pagination'
import { useAuthStore } from '@/features/auth/store'
import {
  fetchAdminUsers,
  setUserRoles,
  setUserStatus,
  deleteAdminUser,
  type AdminUser,
  type AdminUserFilter,
} from '@/features/auth'

function StatusBadge({ status }: { status: 'active' | 'suspended' }) {
  return (
    <span
      className={cn(
        'inline-flex items-center px-2 py-0.5 rounded text-xs font-medium',
        status === 'active' ? 'bg-emerald-900/30 text-emerald-400' : 'bg-red-900/30 text-red-400'
      )}
    >
      {status}
    </span>
  )
}

function RoleBadge({ role }: { role: string }) {
  return (
    <span
      className={cn(
        'inline-flex items-center px-1.5 py-0.5 rounded text-xs font-medium',
        role === 'admin' ? 'bg-violet-900/30 text-violet-300' : 'bg-white/[0.05] text-slate-400'
      )}
    >
      {role}
    </span>
  )
}

function DeleteDialog({
  user,
  open,
  onOpenChange,
}: {
  user: AdminUser
  open: boolean
  onOpenChange: (v: boolean) => void
}) {
  const qc = useQueryClient()
  const { mutate, isPending } = useMutation({
    mutationFn: () => deleteAdminUser(user.id),
    onSuccess: () => {
      toast.success(`Deleted ${user.email}`)
      qc.invalidateQueries({ queryKey: ['admin-users'] })
      onOpenChange(false)
    },
    onError: () => toast.error('Failed to delete user'),
  })

  return (
    <Dialog.Root open={open} onOpenChange={onOpenChange}>
      <Dialog.Portal>
        <Dialog.Overlay className="fixed inset-0 bg-black/60 z-40" />
        <Dialog.Content className="fixed left-1/2 top-1/2 -translate-x-1/2 -translate-y-1/2 z-50 w-full max-w-sm bg-[#141420] border border-white/[0.08] rounded-xl p-6 shadow-xl">
          <Dialog.Title className="text-base font-semibold text-white mb-2">
            Delete {user.email}?
          </Dialog.Title>
          <Dialog.Description className="text-sm text-slate-400 mb-6">
            Permanently deletes the Supabase auth record and all DB data for this user. Cannot be
            undone.
          </Dialog.Description>
          <div className="flex justify-end gap-3">
            <Dialog.Close asChild>
              <button className="px-4 py-2 rounded-md text-sm text-slate-400 hover:text-white hover:bg-white/[0.05] transition-colors">
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

function UserRow({ user, selfId }: { user: AdminUser; selfId: string | undefined }) {
  const qc = useQueryClient()
  const [deleteOpen, setDeleteOpen] = useState(false)
  const isSelf = selfId !== undefined && user.id === selfId

  const { mutate: toggleAdmin, isPending: togglingAdmin } = useMutation({
    mutationFn: () => {
      const newRoles = user.roles.includes('admin')
        ? user.roles.filter((r) => r !== 'admin')
        : [...user.roles, 'admin']
      return setUserRoles(user.id, newRoles)
    },
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['admin-users'] })
      toast.success(
        user.roles.includes('admin')
          ? `Admin revoked for ${user.email}`
          : `Admin granted to ${user.email}`
      )
    },
    onError: () => toast.error('Failed to update roles'),
  })

  const { mutate: toggleStatus, isPending: togglingStatus } = useMutation({
    mutationFn: () => setUserStatus(user.id, user.status === 'active' ? 'suspended' : 'active'),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['admin-users'] })
      toast.success(
        user.status === 'active' ? `${user.email} suspended` : `${user.email} reinstated`
      )
    },
    onError: () => toast.error('Failed to update status'),
  })

  const isAdmin = user.roles.includes('admin')
  const selfTitle = 'Cannot modify your own account'

  return (
    <>
      <tr className="border-b border-white/[0.05] hover:bg-white/[0.02] transition-colors">
        <td className="px-4 py-3">
          <div className="flex items-center gap-2">
            <div className="text-sm text-white">{user.email}</div>
            {isSelf && (
              <span className="text-xs text-slate-500 bg-white/[0.05] px-1.5 py-0.5 rounded">
                you
              </span>
            )}
          </div>
          {user.display_name && (
            <div className="text-xs text-slate-500 mt-0.5">{user.display_name}</div>
          )}
        </td>
        <td className="px-4 py-3">
          <StatusBadge status={user.status} />
        </td>
        <td className="px-4 py-3">
          <div className="flex flex-wrap gap-1">
            {user.roles.map((r) => (
              <RoleBadge key={r} role={r} />
            ))}
          </div>
        </td>
        <td className="px-4 py-3 text-xs text-slate-500">
          {new Date(user.created_at).toLocaleDateString()}
        </td>
        <td className="px-4 py-3">
          <div className="flex items-center gap-1">
            <button
              onClick={() => toggleAdmin()}
              disabled={togglingAdmin || isSelf}
              title={isSelf ? selfTitle : isAdmin ? 'Revoke admin' : 'Grant admin'}
              className={cn(
                'p-1.5 rounded-md transition-colors',
                isSelf
                  ? 'text-slate-700 cursor-not-allowed'
                  : isAdmin
                    ? 'text-violet-400 hover:bg-violet-900/30'
                    : 'text-slate-500 hover:text-violet-400 hover:bg-white/[0.05]'
              )}
            >
              <Shield size={14} />
            </button>
            <button
              onClick={() => toggleStatus()}
              disabled={togglingStatus || isSelf}
              title={isSelf ? selfTitle : user.status === 'active' ? 'Suspend' : 'Reinstate'}
              className={cn(
                'p-1.5 rounded-md transition-colors',
                isSelf
                  ? 'text-slate-700 cursor-not-allowed'
                  : user.status === 'suspended'
                    ? 'text-amber-400 hover:bg-amber-900/30'
                    : 'text-slate-500 hover:text-amber-400 hover:bg-white/[0.05]'
              )}
            >
              <ShieldOff size={14} />
            </button>
            <button
              onClick={() => !isSelf && setDeleteOpen(true)}
              disabled={isSelf}
              title={isSelf ? selfTitle : 'Delete user'}
              className={cn(
                'p-1.5 rounded-md transition-colors',
                isSelf
                  ? 'text-slate-700 cursor-not-allowed'
                  : 'text-slate-500 hover:text-red-400 hover:bg-red-900/20'
              )}
            >
              <Trash2 size={14} />
            </button>
          </div>
        </td>
      </tr>
      {!isSelf && <DeleteDialog user={user} open={deleteOpen} onOpenChange={setDeleteOpen} />}
    </>
  )
}

export function AdminUsersPage() {
  const selfId = useAuthStore((s) => s.identity?.user_id)
  const [emailInput, setEmailInput] = useState('')
  const debouncedEmail = useDebouncedValue(emailInput, 300)
  const [statusFilter, setStatusFilter] = useState<AdminUserFilter['status']>(undefined)
  const [page, setPage] = useState(1)
  const [limit, setLimit] = useState(20)

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

  const { data, isLoading } = useQuery({
    queryKey: ['admin-users', filter, page, limit],
    queryFn: () => fetchAdminUsers(filter, page, limit),
    staleTime: 30_000,
  })

  const users = data?.data ?? []
  const total = data?.total ?? 0

  return (
    <div className="flex flex-col h-full bg-[#0d0d14] overflow-y-auto">
      <div className="border-b border-white/[0.07] px-6 py-4 shrink-0">
        <h1 className="text-lg font-semibold text-white">Users</h1>
      </div>

      {/* Filters */}
      <div className="px-6 py-3 border-b border-white/[0.07] flex items-center gap-3 shrink-0">
        <input
          type="text"
          placeholder="Filter by email…"
          value={emailInput}
          onChange={(e) => setEmailInput(e.target.value)}
          className="w-56 rounded-md bg-white/[0.04] border border-white/[0.1] px-3 py-1.5 text-sm text-white placeholder:text-slate-600 focus:outline-none focus:ring-1 focus:ring-violet-500"
        />
        <select
          value={statusFilter ?? ''}
          onChange={(e) =>
            setStatusFilter((e.target.value as AdminUserFilter['status']) || undefined)
          }
          className="rounded-md bg-white/[0.04] border border-white/[0.1] px-3 py-1.5 text-sm text-white focus:outline-none focus:ring-1 focus:ring-violet-500"
        >
          <option value="">All statuses</option>
          <option value="active">Active</option>
          <option value="suspended">Suspended</option>
        </select>
        <span className="text-xs text-slate-500 ml-auto">
          {total} user{total !== 1 ? 's' : ''}
        </span>
      </div>

      {/* Table */}
      <div className="flex-1 overflow-auto">
        {isLoading ? (
          <div className="flex items-center justify-center h-32 text-slate-500 text-sm">
            Loading…
          </div>
        ) : users.length === 0 ? (
          <div className="flex items-center justify-center h-32 text-slate-500 text-sm">
            No users found.
          </div>
        ) : (
          <table className="w-full text-sm">
            <thead>
              <tr className="border-b border-white/[0.07] text-xs text-slate-500 text-left">
                <th className="px-4 py-2 font-medium">Email</th>
                <th className="px-4 py-2 font-medium">Status</th>
                <th className="px-4 py-2 font-medium">Roles</th>
                <th className="px-4 py-2 font-medium">Joined</th>
                <th className="px-4 py-2 font-medium">Actions</th>
              </tr>
            </thead>
            <tbody>
              {users.map((u) => (
                <UserRow key={u.id} user={u} selfId={selfId} />
              ))}
            </tbody>
          </table>
        )}
      </div>

      <div className="border-t border-white/[0.07] shrink-0">
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
    </div>
  )
}
