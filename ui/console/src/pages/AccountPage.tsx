import { useState } from 'react'
import { toast } from 'sonner'
import * as Dialog from '@radix-ui/react-dialog'
import { useAuthStore, updateMe, deleteMe } from '@/features/auth'
import { supabase } from '@/lib/core/supabase'
import { ErrorBoundary } from '@/components/shared/ErrorBoundary'

export function AccountPage() {
  const identity = useAuthStore((s) => s.identity)
  const refreshIdentity = useAuthStore((s) => s.refreshIdentity)
  const signOut = useAuthStore((s) => s.signOut)

  const [displayName, setDisplayName] = useState(identity?.display_name ?? '')
  const [saving, setSaving] = useState(false)
  const [deleteOpen, setDeleteOpen] = useState(false)
  const [deleting, setDeleting] = useState(false)

  async function handleSave(e: React.FormEvent) {
    e.preventDefault()
    setSaving(true)
    try {
      await updateMe(displayName)
      await refreshIdentity()
      toast.success('Profile updated.')
    } catch {
      toast.error('Failed to update profile.')
    } finally {
      setSaving(false)
    }
  }

  async function handleDelete() {
    setDeleting(true)
    try {
      await deleteMe()
      await supabase.auth.signOut({ scope: 'global' })
      signOut()
    } catch {
      toast.error('Failed to delete account.')
      setDeleting(false)
    }
  }

  return (
    <div className="flex flex-col h-full bg-[#0d0d14] overflow-y-auto">
      <div className="border-b border-white/[0.07] px-6 py-4 shrink-0">
        <h1 className="text-lg font-semibold text-white">Account</h1>
      </div>

      <div className="flex-1 px-6 py-6 max-w-lg space-y-8">
        {/* Profile */}
        <ErrorBoundary>
          <section>
            <h2 className="text-sm font-semibold text-white mb-4">Profile</h2>
            <div className="mb-3">
              <p className="text-xs text-slate-500 mb-1">Email</p>
              <p className="text-sm text-slate-300">{identity?.email ?? '—'}</p>
            </div>
            <form onSubmit={handleSave} className="space-y-3">
              <div>
                <label htmlFor="display-name" className="block text-xs text-slate-500 mb-1">
                  Display name
                </label>
                <input
                  id="display-name"
                  type="text"
                  value={displayName}
                  onChange={(e) => setDisplayName(e.target.value)}
                  maxLength={80}
                  className="w-full rounded-md bg-white/[0.04] border border-white/[0.1] px-3 py-2 text-sm text-white placeholder:text-slate-600 focus:outline-none focus:ring-1 focus:ring-violet-500"
                  placeholder="Your name"
                />
              </div>
              <button
                type="submit"
                disabled={saving}
                className="px-4 py-2 rounded-md text-sm font-medium bg-violet-600 hover:bg-violet-500 disabled:opacity-50 text-white transition-colors"
              >
                {saving ? 'Saving…' : 'Save'}
              </button>
            </form>
          </section>
        </ErrorBoundary>

        {/* Danger zone */}
        <ErrorBoundary>
          <section className="border border-red-900/40 rounded-lg p-4">
            <h2 className="text-sm font-semibold text-red-400 mb-2">Danger zone</h2>
            <p className="text-xs text-slate-400 mb-4">
              Permanently delete your account and all associated data. This cannot be undone.
            </p>
            <Dialog.Root open={deleteOpen} onOpenChange={setDeleteOpen}>
              <Dialog.Trigger asChild>
                <button className="px-4 py-2 rounded-md text-sm font-medium border border-red-700/60 text-red-400 hover:bg-red-900/20 transition-colors">
                  Delete account
                </button>
              </Dialog.Trigger>
              <Dialog.Portal>
                <Dialog.Overlay className="fixed inset-0 bg-black/60 z-40" />
                <Dialog.Content className="fixed left-1/2 top-1/2 -translate-x-1/2 -translate-y-1/2 z-50 w-full max-w-sm bg-[#141420] border border-white/[0.08] rounded-xl p-6 shadow-xl">
                  <Dialog.Title className="text-base font-semibold text-white mb-2">
                    Delete account?
                  </Dialog.Title>
                  <Dialog.Description className="text-sm text-slate-400 mb-6">
                    Your account, job state, and applications will be permanently deleted. This
                    action cannot be undone.
                  </Dialog.Description>
                  <div className="flex justify-end gap-3">
                    <Dialog.Close asChild>
                      <button className="px-4 py-2 rounded-md text-sm text-slate-400 hover:text-white hover:bg-white/[0.05] transition-colors">
                        Cancel
                      </button>
                    </Dialog.Close>
                    <button
                      onClick={handleDelete}
                      disabled={deleting}
                      className="px-4 py-2 rounded-md text-sm font-medium bg-red-700 hover:bg-red-600 disabled:opacity-50 text-white transition-colors"
                    >
                      {deleting ? 'Deleting…' : 'Delete forever'}
                    </button>
                  </div>
                </Dialog.Content>
              </Dialog.Portal>
            </Dialog.Root>
          </section>
        </ErrorBoundary>
      </div>
    </div>
  )
}
