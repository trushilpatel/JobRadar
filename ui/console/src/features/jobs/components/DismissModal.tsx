import { useState } from 'react'
import * as Dialog from '@radix-ui/react-dialog'
import { cn } from '@/lib/utils'

const DISMISS_REASON_OPTIONS = [
  { value: 'user_dismissed',    label: 'Not relevant' },
  { value: 'user_not_relevant', label: 'Too junior / senior' },
  { value: 'user_not_relevant', label: 'Bad location' },
  { value: 'user_not_relevant', label: 'Compensation too low' },
  { value: 'user_dismissed',    label: 'Company not a fit' },
  { value: 'user_dismissed',    label: 'Other' },
]

interface DismissModalProps {
  open: boolean
  jobTitle: string
  onConfirm: (reason: string) => void
  onCancel: () => void
}

// State lives inside so each mount starts fresh at index 0.
// The caller should pass a key prop (e.g. key={dismissTarget?.id}) so
// this component remounts whenever the target job changes.
export function DismissModal({ open, jobTitle, onConfirm, onCancel }: DismissModalProps) {
  const [selectedIdx, setSelectedIdx] = useState(0)

  return (
    <Dialog.Root open={open} onOpenChange={v => !v && onCancel()}>
      <Dialog.Portal>
        <Dialog.Overlay className="fixed inset-0 bg-black/60 z-50" />
        <Dialog.Content className="fixed left-1/2 top-1/2 -translate-x-1/2 -translate-y-1/2 z-50 w-full max-w-sm bg-[#13131e] border border-white/[0.07] rounded-xl p-5 shadow-2xl">
          <Dialog.Title className="text-sm font-medium text-white mb-1">
            Dismiss job
          </Dialog.Title>
          <p className="text-xs text-slate-500 mb-4 truncate">"{jobTitle}"</p>
          <div className="space-y-2 mb-5">
            {DISMISS_REASON_OPTIONS.map((opt, i) => (
              <button
                key={i}
                onClick={() => setSelectedIdx(i)}
                className={cn(
                  'w-full text-left px-3 py-2.5 rounded-lg text-sm transition-colors border',
                  selectedIdx === i
                    ? 'bg-violet-600/20 border-violet-500/40 text-violet-300'
                    : 'bg-white/[0.03] border-white/[0.07] text-slate-300 hover:bg-white/[0.06]',
                )}
              >
                {opt.label}
              </button>
            ))}
          </div>
          <div className="flex gap-2">
            <button
              onClick={onCancel}
              className="flex-1 px-4 py-2 text-sm text-slate-400 bg-white/5 border border-white/10 rounded-lg hover:bg-white/[0.08] transition-colors"
            >
              Cancel
            </button>
            <button
              onClick={() => onConfirm(DISMISS_REASON_OPTIONS[selectedIdx].value)}
              className="flex-1 px-4 py-2 text-sm font-medium text-white bg-red-600/80 hover:bg-red-600 rounded-lg transition-colors"
            >
              Dismiss
            </button>
          </div>
        </Dialog.Content>
      </Dialog.Portal>
    </Dialog.Root>
  )
}
