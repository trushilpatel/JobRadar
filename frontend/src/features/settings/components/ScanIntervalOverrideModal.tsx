import { useState, useEffect } from 'react'
import * as Dialog from '@radix-ui/react-dialog'
import { X } from 'lucide-react'
import { cn } from '@/lib/utils/utils'
import { useScanIntervalOverride } from '../hooks/useScanIntervalOverride'

interface ScanIntervalOverrideModalProps {
  open: boolean
  onClose: () => void
  companyId: string
  companyName: string
  adapterDefault: number
  currentOverride: number | null
}

export function ScanIntervalOverrideModal({
  open,
  onClose,
  companyId,
  companyName,
  adapterDefault,
  currentOverride,
}: ScanIntervalOverrideModalProps) {
  const [value, setValue] = useState(currentOverride !== null ? String(currentOverride) : '')
  const [error, setError] = useState('')
  const { setOverride, loading } = useScanIntervalOverride(companyId)

  useEffect(() => {
    if (open) {
      // eslint-disable-next-line react-hooks/set-state-in-effect
      setValue(currentOverride !== null ? String(currentOverride) : '')
      setError('')
    }
  }, [open, currentOverride])

  function validate(raw: string): number | null {
    const n = Number(raw)
    if (!raw || !Number.isInteger(n) || Number.isNaN(n)) return null
    if (n < 1 || n > 10080) return null
    return n
  }

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault()
    const parsed = validate(value)
    if (parsed === null) {
      setError('Enter a value between 1 and 10080 minutes')
      return
    }
    try {
      await setOverride(parsed)
      onClose()
    } catch {
      setError('Failed to save override. Please try again.')
    }
  }

  const inputClass =
    'w-full bg-subtle border border-[var(--border-strong)] rounded-lg px-3 py-2 text-sm text-primary placeholder:text-placeholder focus:outline-none focus:border-violet-500 transition-colors'

  return (
    <Dialog.Root open={open} onOpenChange={(v) => !v && onClose()}>
      <Dialog.Portal>
        <Dialog.Overlay className="fixed inset-0 bg-black/60 backdrop-blur-sm z-40" />
        <Dialog.Content className="fixed left-1/2 top-1/2 -translate-x-1/2 -translate-y-1/2 z-50 w-full max-w-sm bg-panel border border-[var(--border-strong)] rounded-xl shadow-2xl p-6">
          <div className="flex items-center justify-between mb-2">
            <Dialog.Title className="text-base font-semibold text-primary">
              Custom Scan Interval
            </Dialog.Title>
            <button
              type="button"
              onClick={onClose}
              disabled={loading}
              className="text-muted hover:text-primary transition-colors disabled:opacity-50"
            >
              <X size={16} />
            </button>
          </div>

          <Dialog.Description className="text-sm text-secondary mb-5">
            Override the adapter default ({adapterDefault} min) for{' '}
            <span className="text-primary font-medium">{companyName}</span> only.
          </Dialog.Description>

          <form onSubmit={(e) => void handleSubmit(e)} className="space-y-4">
            <div className="space-y-1">
              <label className="text-sm font-medium text-secondary">
                Scan interval
                <span className="ml-2 text-xs text-muted">minutes</span>
              </label>
              <div className="flex items-center gap-2">
                <input
                  type="number"
                  value={value}
                  onChange={(e) => {
                    setValue(e.target.value)
                    setError('')
                  }}
                  placeholder="e.g. 120"
                  min={1}
                  max={10080}
                  className={cn(inputClass, error && 'border-red-500')}
                  autoFocus
                />
              </div>
              <p className="text-[11px] text-muted">Adapter default: {adapterDefault} min</p>
              {error && <p className="text-xs text-red-400">{error}</p>}
            </div>

            <div className="flex gap-3 pt-1">
              <button
                type="button"
                onClick={onClose}
                disabled={loading}
                className="flex-1 px-4 py-2 rounded-lg text-sm font-medium bg-subtle hover:bg-hover text-secondary transition-colors disabled:opacity-50"
              >
                Cancel
              </button>
              <button
                type="submit"
                disabled={loading}
                className="flex-1 px-4 py-2 rounded-lg text-sm font-medium bg-violet-600 hover:bg-violet-500 text-white transition-colors disabled:opacity-50"
              >
                {loading ? 'Saving…' : 'Save'}
              </button>
            </div>
          </form>
        </Dialog.Content>
      </Dialog.Portal>
    </Dialog.Root>
  )
}
