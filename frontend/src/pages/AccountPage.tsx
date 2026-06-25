import { useState, useEffect, type ComponentType } from 'react'
import { toast } from 'sonner'
import * as Dialog from '@radix-ui/react-dialog'
import { Sun, Moon, Monitor } from 'lucide-react'
import { useClerk } from '@clerk/react'
import { useQueryClient } from '@tanstack/react-query'
import { useIdentity, updateMe, deleteMe } from '@/features/auth'
import { JOB_COMMANDS, type JobCommandId } from '@/features/jobs'
import { formatKeys } from '@/lib/commands/formatKeys'
import type { CommandCategory, KeySpec } from '@/lib/commands/types'
import { ErrorBoundary } from '@/components/shared/ErrorBoundary'
import { Kbd, KbdGroup } from '@/components/ui/kbd'
import { Switch } from '@/components/ui/switch'
import { useUserConfig, usePatchUserConfig } from '@/lib/hooks/useUserConfig'
import { cn } from '@/lib/utils/utils'
import { useTheme, resolveSystemTheme } from '@/app/ThemeContext'
import type { ThemePreference } from '@/types'

const SHORTCUT_CATEGORIES: { category: CommandCategory; title: string }[] = [
  { category: 'navigation', title: 'Navigation' },
  { category: 'tabs', title: 'Tabs' },
  { category: 'status', title: 'Status' },
  { category: 'search', title: 'Search' },
]

/** Registry-derived keyboard reference table (same source as the help sheet). */
function ShortcutReference() {
  const ids = Object.keys(JOB_COMMANDS) as JobCommandId[]
  return (
    <div className="mt-4 space-y-5">
      {SHORTCUT_CATEGORIES.map(({ category, title }) => {
        const rows = ids.filter((id) => JOB_COMMANDS[id].category === category)
        if (rows.length === 0) return null
        return (
          <div key={category}>
            <h3 className="mb-2 text-xs font-semibold uppercase tracking-wide text-muted">
              {title}
            </h3>
            <dl className="space-y-1.5">
              {rows.map((id) => {
                const def = JOB_COMMANDS[id]
                const keys: readonly KeySpec[] = def.keys
                return (
                  <div key={id} className="grid grid-cols-[1fr_auto] items-center gap-x-4 gap-y-1">
                    <dt className="text-sm text-secondary">{def.label}</dt>
                    <dd className="flex items-center gap-2 justify-self-end">
                      {keys.length === 0 ? (
                        <span className="text-xs text-placeholder">—</span>
                      ) : (
                        keys.map((spec, si) => (
                          <span key={si} className="flex items-center gap-1">
                            {si > 0 && <span className="text-xs text-placeholder">/</span>}
                            {formatKeys(spec).map((group, gi) => (
                              <KbdGroup key={gi}>
                                {group.map((glyph, ki) => (
                                  <Kbd key={`${gi}-${ki}`}>{glyph}</Kbd>
                                ))}
                              </KbdGroup>
                            ))}
                          </span>
                        ))
                      )}
                    </dd>
                  </div>
                )
              })}
            </dl>
          </div>
        )
      })}
    </div>
  )
}

function ThemeToggle() {
  const { theme, setTheme } = useTheme()
  const [systemResolved, setSystemResolved] = useState<'light' | 'dark'>(resolveSystemTheme)

  useEffect(() => {
    const mq = window.matchMedia('(prefers-color-scheme: dark)')
    const handler = () => setSystemResolved(mq.matches ? 'dark' : 'light')
    mq.addEventListener('change', handler)
    return () => mq.removeEventListener('change', handler)
  }, [])

  const options: Array<{
    value: ThemePreference
    label: string
    icon: ComponentType<{ size?: number }>
  }> = [
    { value: 'light', label: 'Light', icon: Sun },
    { value: 'dark', label: 'Dark', icon: Moon },
    { value: 'system', label: 'System', icon: Monitor },
  ]
  return (
    <div className="flex flex-col items-end gap-1">
      <div className="flex rounded-lg border border-[var(--border)] bg-panel p-1 gap-1">
        {options.map(({ value, label, icon: Icon }) => (
          <button
            key={value}
            type="button"
            onClick={() => setTheme(value)}
            className={cn(
              'flex items-center gap-1.5 px-3 py-1.5 rounded-md text-xs font-medium transition-colors',
              theme === value
                ? 'bg-violet-600 text-white'
                : 'text-muted hover:text-primary hover:bg-hover'
            )}
          >
            <Icon size={14} />
            {label}
          </button>
        ))}
      </div>
      {theme === 'system' && <p className="text-xs text-placeholder">Currently {systemResolved}</p>}
    </div>
  )
}

export function AccountPage() {
  const { identity } = useIdentity()
  const { signOut } = useClerk()
  const queryClient = useQueryClient()
  async function refreshIdentity() {
    await queryClient.invalidateQueries({ queryKey: ['me'] })
  }

  const { data: userConfig, isLoading: configLoading } = useUserConfig()
  const patchConfig = usePatchUserConfig()

  const [displayName, setDisplayName] = useState('')
  useEffect(() => {
    setDisplayName(identity?.display_name ?? '')
  }, [identity?.display_name])

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
      await signOut()
    } catch {
      toast.error('Failed to delete account.')
      setDeleting(false)
    }
  }

  return (
    <div className="flex flex-col h-full bg-panel overflow-y-auto">
      <div className="border-b border-[var(--border)] px-6 py-4 shrink-0">
        <h1 className="text-lg font-semibold text-primary">Account</h1>
      </div>

      <div className="flex-1 px-6 py-6 max-w-2xl space-y-8">
        {/* Profile */}
        <ErrorBoundary>
          <section>
            <h2 className="text-sm font-semibold text-primary mb-4">Profile</h2>
            <div className="mb-3">
              <p className="text-xs text-muted mb-1">Email</p>
              <p className="text-sm text-secondary">{identity?.email ?? '—'}</p>
              <p className="text-xs text-placeholder mt-0.5">
                Email is managed by your login provider and cannot be changed here.
              </p>
            </div>
            <form onSubmit={handleSave} className="space-y-3">
              <div>
                <label htmlFor="display-name" className="block text-xs text-muted mb-1">
                  Display name
                </label>
                <input
                  id="display-name"
                  type="text"
                  value={displayName}
                  onChange={(e) => setDisplayName(e.target.value)}
                  maxLength={80}
                  className="w-full rounded-md bg-subtle border border-[var(--border-strong)] px-3 py-2 text-sm text-primary placeholder:text-placeholder focus:outline-none focus:ring-1 focus:ring-violet-500"
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

        {/* Appearance */}
        <ErrorBoundary>
          <section className="space-y-4">
            <h2 className="text-sm font-semibold text-primary mb-4">Appearance</h2>
            <div className="flex items-center justify-between">
              <div>
                <p className="text-sm text-primary">Theme</p>
                <p className="text-xs text-muted mt-0.5">Choose how JobRadar looks to you</p>
              </div>
              <ThemeToggle />
            </div>
          </section>
        </ErrorBoundary>

        {/* Keyboard shortcuts */}
        <ErrorBoundary>
          <section>
            <h2 className="text-sm font-semibold text-primary mb-4">Keyboard shortcuts</h2>
            <div className="flex items-center justify-between">
              <div>
                <p className="text-sm text-secondary">Show keyboard shortcut hints</p>
                <p className="text-xs text-muted">Display shortcut keys inside action buttons</p>
              </div>
              <Switch
                id="show-keyboard-shortcuts"
                checked={userConfig?.show_keyboard_shortcuts ?? false}
                onCheckedChange={(checked) =>
                  patchConfig.mutate({ show_keyboard_shortcuts: checked })
                }
                disabled={patchConfig.isPending || configLoading}
              />
            </div>
            {/* Static registry-derived reference — always visible. The toggle
                above only controls in-button hint glyphs, not this table. */}
            <div className="mt-4 border-t border-[var(--border)] pt-4">
              <ShortcutReference />
            </div>
          </section>
        </ErrorBoundary>

        {/* Danger zone */}
        <ErrorBoundary>
          <section className="border border-red-400/30 rounded-lg p-4">
            <h2 className="text-sm font-semibold text-red-400 mb-2">Danger zone</h2>
            <p className="text-xs text-muted mb-4">
              Permanently delete your account and all associated data. This cannot be undone.
            </p>
            <Dialog.Root open={deleteOpen} onOpenChange={setDeleteOpen}>
              <Dialog.Trigger asChild>
                <button className="px-4 py-2 rounded-md text-sm font-medium border border-red-400/50 hover:bg-red-500/8 text-red-400 transition-colors">
                  Delete account
                </button>
              </Dialog.Trigger>
              <Dialog.Portal>
                <Dialog.Overlay className="fixed inset-0 bg-black/60 z-40" />
                <Dialog.Content className="fixed left-1/2 top-1/2 -translate-x-1/2 -translate-y-1/2 z-50 w-full max-w-sm bg-panel border border-[var(--border)] rounded-xl p-6 shadow-xl">
                  <Dialog.Title className="text-base font-semibold text-primary mb-2">
                    Delete account?
                  </Dialog.Title>
                  <Dialog.Description className="text-sm text-muted mb-6">
                    Your account, job state, and applications will be permanently deleted. This
                    action cannot be undone.
                  </Dialog.Description>
                  <div className="flex justify-end gap-3">
                    <Dialog.Close asChild>
                      <button className="px-4 py-2 rounded-md text-sm text-muted hover:text-primary hover:bg-hover transition-colors">
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
