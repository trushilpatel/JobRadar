import { useState, useCallback } from 'react'
import * as Dialog from '@radix-ui/react-dialog'
import { X } from 'lucide-react'
import { cn } from '@/lib/utils/utils'
import { AdapterConfigCell } from './AdapterConfigCell'
import type { ATS } from '@/types'

const ADAPTER_COLORS: Record<string, string> = {
  greenhouse: 'bg-emerald-400',
  lever: 'bg-blue-400',
  ashby: 'bg-violet-400',
  google: 'bg-red-400',
  meta: 'bg-blue-500',
  netflix: 'bg-red-500',
  uber: 'bg-slate-300',
  apple: 'bg-slate-400',
  microsoft: 'bg-sky-400',
  rippling: 'bg-orange-400',
  gem: 'bg-indigo-400',
  deel: 'bg-amber-400',
}

type EditableField = 'scan_interval_min' | 'inter_company_delay_s'

interface PendingChange {
  atsName: string
  displayName: string
  field: EditableField
  newValue: number
  currentAts: ATS
  resolve: () => void
  reject: (err: Error) => void
}

export interface AtsTableProps {
  atsList: ATS[] | undefined
  isLoading: boolean
  isError: boolean
  refetch: () => void
  onUpdate: (
    name: string,
    data: Partial<
      Pick<ATS, 'scan_interval_min' | 'inter_company_delay_s' | 'is_active' | 'is_admin_creatable'>
    >
  ) => Promise<ATS>
}

export function AtsTable({ atsList, isLoading, isError, refetch, onUpdate }: AtsTableProps) {
  const [pending, setPending] = useState<PendingChange | null>(null)

  const handleSave = useCallback(
    (
      atsName: string,
      displayName: string,
      field: EditableField,
      newValue: number,
      currentAts: ATS
    ): Promise<void> => {
      return new Promise<void>((resolve, reject) => {
        setPending({ atsName, displayName, field, newValue, currentAts, resolve, reject })
      })
    },
    []
  )

  async function confirmSave() {
    if (!pending) return
    const { atsName, field, newValue, currentAts, resolve, reject } = pending
    const data: Partial<Pick<ATS, 'scan_interval_min' | 'inter_company_delay_s'>> = {
      scan_interval_min: currentAts.scan_interval_min,
      inter_company_delay_s: currentAts.inter_company_delay_s,
      [field]: newValue,
    }
    setPending(null)
    try {
      await onUpdate(atsName, data)
      resolve()
    } catch (err) {
      reject(err instanceof Error ? err : new Error('Failed'))
    }
  }

  function cancelSave() {
    if (!pending) return
    pending.reject(new Error('Cancelled'))
    setPending(null)
  }

  if (isLoading) {
    return (
      <div className="space-y-2">
        {Array.from({ length: 5 }).map((_, i) => (
          <div key={i} className="h-10 bg-subtle rounded animate-pulse" />
        ))}
      </div>
    )
  }

  if (isError) {
    return (
      <div className="flex items-center gap-3 text-sm text-red-400 bg-red-500/10 border border-red-500/20 rounded-lg px-4 py-3">
        <span>Failed to load ATS configs.</span>
        <button
          type="button"
          onClick={() => void refetch()}
          className="underline hover:no-underline transition-all"
        >
          Retry
        </button>
      </div>
    )
  }

  const rows = atsList ?? []

  if (rows.length === 0) {
    return <p className="text-sm text-muted py-4">No ATS configurations found.</p>
  }

  return (
    <>
      <div className="rounded-lg border border-[var(--border)] overflow-hidden">
        <table className="w-full text-sm">
          <thead>
            <tr className="border-b border-[var(--border)] bg-subtle">
              <th className="text-left px-4 py-2.5 text-[11px] font-medium text-muted uppercase tracking-wider">
                Adapter
              </th>
              <th className="text-left px-4 py-2.5 text-[11px] font-medium text-muted uppercase tracking-wider">
                Scan Interval
              </th>
              <th className="text-left px-4 py-2.5 text-[11px] font-medium text-muted uppercase tracking-wider">
                Inter-Company Delay
              </th>
              <th className="text-left px-4 py-2.5 text-[11px] font-medium text-muted uppercase tracking-wider">
                Status
              </th>
            </tr>
          </thead>
          <tbody className="divide-y divide-[var(--border)]">
            {rows.map((ats) => (
              <tr key={ats.name} className="hover:bg-hover transition-colors">
                <td className="px-4 py-3">
                  <div className="flex items-center gap-2">
                    <span
                      className={cn(
                        'w-2 h-2 rounded-full shrink-0',
                        ADAPTER_COLORS[ats.name] ?? 'bg-slate-400'
                      )}
                    />
                    <div className="flex flex-col">
                      <span className="text-primary font-medium">{ats.display_name}</span>
                      <span className="text-muted text-xs">{ats.name}</span>
                    </div>
                  </div>
                </td>
                <td className="px-4 py-3">
                  <AdapterConfigCell
                    value={ats.scan_interval_min}
                    min={1}
                    max={10080}
                    unit="min"
                    onSave={(v) =>
                      handleSave(ats.name, ats.display_name, 'scan_interval_min', v, ats)
                    }
                  />
                </td>
                <td className="px-4 py-3">
                  <AdapterConfigCell
                    value={ats.inter_company_delay_s}
                    min={0}
                    max={3600}
                    unit="s"
                    onSave={(v) =>
                      handleSave(ats.name, ats.display_name, 'inter_company_delay_s', v, ats)
                    }
                  />
                </td>
                <td className="px-4 py-3">
                  <span
                    className={cn(
                      'inline-flex items-center px-2 py-0.5 rounded text-xs font-medium',
                      ats.is_active ? 'bg-emerald-500/10 text-emerald-400' : 'bg-subtle text-muted'
                    )}
                  >
                    {ats.is_active ? 'Active' : 'Inactive'}
                  </span>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>

      {/* Confirmation modal */}
      <Dialog.Root open={!!pending} onOpenChange={(v) => !v && cancelSave()}>
        <Dialog.Portal>
          <Dialog.Overlay className="fixed inset-0 bg-black/60 backdrop-blur-sm z-40" />
          <Dialog.Content className="fixed left-1/2 top-1/2 -translate-x-1/2 -translate-y-1/2 z-50 w-full max-w-sm bg-panel border border-[var(--border-strong)] rounded-xl shadow-2xl p-6">
            <div className="flex items-center justify-between mb-4">
              <Dialog.Title className="text-base font-semibold text-primary">
                Confirm Change
              </Dialog.Title>
              <button
                type="button"
                onClick={cancelSave}
                className="text-muted hover:text-primary transition-colors"
              >
                <X size={16} />
              </button>
            </div>
            <Dialog.Description className="text-sm text-secondary mb-5">
              {pending && (
                <>
                  Changing <span className="text-primary font-medium">{pending.displayName}</span>{' '}
                  {pending.field === 'scan_interval_min' ? 'scan interval' : 'inter-company delay'}{' '}
                  to{' '}
                  <span className="text-violet-300 font-medium">
                    {pending.newValue}
                    {pending.field === 'scan_interval_min' ? ' min' : ' s'}
                  </span>{' '}
                  will affect all companies using this adapter.
                </>
              )}
            </Dialog.Description>
            <div className="flex gap-3">
              <button
                type="button"
                onClick={cancelSave}
                className="flex-1 px-4 py-2 rounded-lg text-sm font-medium bg-subtle hover:bg-hover text-secondary transition-colors"
              >
                Cancel
              </button>
              <button
                type="button"
                onClick={() => void confirmSave()}
                className="flex-1 px-4 py-2 rounded-lg text-sm font-medium bg-violet-600 hover:bg-violet-500 text-white transition-colors"
              >
                Confirm
              </button>
            </div>
          </Dialog.Content>
        </Dialog.Portal>
      </Dialog.Root>
    </>
  )
}
