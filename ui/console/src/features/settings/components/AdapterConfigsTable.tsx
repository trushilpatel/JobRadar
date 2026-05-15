import { useState, useCallback } from 'react'
import * as Dialog from '@radix-ui/react-dialog'
import { X } from 'lucide-react'
import { cn } from '@/lib/utils'
import { formatRelativeTime } from '@/lib/utils'
import { useAdapterConfigs } from '../hooks/useAdapterConfigs'
import { useAdapterConfigActions } from '../hooks/useAdapterConfigActions'
import { AdapterConfigCell } from './AdapterConfigCell'
import type { AdapterConfig, UpdateAdapterConfigInput } from '@/api/adapterConfigs'

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
}

interface PendingChange {
  adapterType: string
  field: keyof UpdateAdapterConfigInput
  newValue: number
  currentConfig: AdapterConfig
  resolve: () => void
  reject: (err: Error) => void
}

export function AdapterConfigsTable() {
  const { data: configs, isLoading, isError, refetch } = useAdapterConfigs()
  const { updateConfig } = useAdapterConfigActions()
  const [pending, setPending] = useState<PendingChange | null>(null)

  const handleSave = useCallback(
    (
      adapterType: string,
      field: keyof UpdateAdapterConfigInput,
      newValue: number,
      currentConfig: AdapterConfig
    ): Promise<void> => {
      return new Promise<void>((resolve, reject) => {
        setPending({ adapterType, field, newValue, currentConfig, resolve, reject })
      })
    },
    []
  )

  async function confirmSave() {
    if (!pending) return
    const { adapterType, field, newValue, currentConfig, resolve, reject } = pending
    const data: UpdateAdapterConfigInput = {
      scan_interval_minutes: currentConfig.scan_interval_minutes,
      inter_company_delay_s: currentConfig.inter_company_delay_s,
      [field]: newValue,
    }
    setPending(null)
    try {
      await updateConfig(adapterType, data)
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
          <div key={i} className="h-10 bg-white/5 rounded animate-pulse" />
        ))}
      </div>
    )
  }

  if (isError) {
    return (
      <div className="flex items-center gap-3 text-sm text-red-400 bg-red-500/10 border border-red-500/20 rounded-lg px-4 py-3">
        <span>Failed to load adapter configs.</span>
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

  const rows = configs ?? []

  return (
    <>
      <div className="rounded-lg border border-white/[0.07] overflow-hidden">
        <table className="w-full text-sm">
          <thead>
            <tr className="border-b border-white/[0.07] bg-white/[0.02]">
              <th className="text-left px-4 py-2.5 text-[11px] font-medium text-white/40 uppercase tracking-wider">
                Adapter
              </th>
              <th className="text-left px-4 py-2.5 text-[11px] font-medium text-white/40 uppercase tracking-wider">
                Scan Interval
              </th>
              <th className="text-left px-4 py-2.5 text-[11px] font-medium text-white/40 uppercase tracking-wider">
                Inter-Company Delay
              </th>
              <th className="text-left px-4 py-2.5 text-[11px] font-medium text-white/40 uppercase tracking-wider">
                Last Scheduled
              </th>
              <th className="text-left px-4 py-2.5 text-[11px] font-medium text-white/40 uppercase tracking-wider">
                Updated
              </th>
            </tr>
          </thead>
          <tbody className="divide-y divide-white/[0.04]">
            {rows.map((cfg) => (
              <tr key={cfg.adapter_type} className="hover:bg-white/[0.02] transition-colors">
                <td className="px-4 py-3">
                  <div className="flex items-center gap-2">
                    <span
                      className={cn(
                        'w-2 h-2 rounded-full shrink-0',
                        ADAPTER_COLORS[cfg.adapter_type] ?? 'bg-slate-400'
                      )}
                    />
                    <span className="text-white font-medium">{cfg.adapter_type}</span>
                  </div>
                </td>
                <td className="px-4 py-3">
                  <AdapterConfigCell
                    value={cfg.scan_interval_minutes}
                    min={1}
                    max={10080}
                    unit="min"
                    onSave={(v) => handleSave(cfg.adapter_type, 'scan_interval_minutes', v, cfg)}
                  />
                </td>
                <td className="px-4 py-3">
                  <AdapterConfigCell
                    value={cfg.inter_company_delay_s}
                    min={0}
                    max={3600}
                    unit="s"
                    onSave={(v) => handleSave(cfg.adapter_type, 'inter_company_delay_s', v, cfg)}
                  />
                </td>
                <td className="px-4 py-3 text-white/50">
                  {cfg.last_scheduled_at ? formatRelativeTime(cfg.last_scheduled_at) : 'Never'}
                </td>
                <td className="px-4 py-3 text-white/50">
                  {new Date(cfg.updated_at).toLocaleDateString()}
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
          <Dialog.Content className="fixed left-1/2 top-1/2 -translate-x-1/2 -translate-y-1/2 z-50 w-full max-w-sm bg-[#13131a] border border-white/10 rounded-xl shadow-2xl p-6">
            <div className="flex items-center justify-between mb-4">
              <Dialog.Title className="text-base font-semibold text-white">
                Confirm Change
              </Dialog.Title>
              <button
                type="button"
                onClick={cancelSave}
                className="text-slate-400 hover:text-white transition-colors"
              >
                <X size={16} />
              </button>
            </div>
            <Dialog.Description className="text-sm text-white/70 mb-5">
              {pending && (
                <>
                  Changing <span className="text-white font-medium">{pending.adapterType}</span>{' '}
                  {pending.field === 'scan_interval_minutes'
                    ? 'scan interval'
                    : 'inter-company delay'}{' '}
                  to{' '}
                  <span className="text-violet-300 font-medium">
                    {pending.newValue}
                    {pending.field === 'scan_interval_minutes' ? ' min' : ' s'}
                  </span>{' '}
                  will affect all companies using this adapter.
                </>
              )}
            </Dialog.Description>
            <div className="flex gap-3">
              <button
                type="button"
                onClick={cancelSave}
                className="flex-1 px-4 py-2 rounded-lg text-sm font-medium bg-white/5 hover:bg-white/10 text-slate-300 transition-colors"
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
