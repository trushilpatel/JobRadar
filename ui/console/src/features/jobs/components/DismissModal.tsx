import { useState } from 'react'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { cn } from '@/lib/utils'

const DISMISS_REASON_OPTIONS = [
  { value: 'user_dismissed', label: 'Not relevant' },
  { value: 'user_not_relevant', label: 'Too junior / senior' },
  { value: 'user_not_relevant', label: 'Bad location' },
  { value: 'user_not_relevant', label: 'Compensation too low' },
  { value: 'user_dismissed', label: 'Company not a fit' },
  { value: 'user_dismissed', label: 'Other' },
]

interface DismissModalProps {
  open: boolean
  jobTitle: string
  onConfirm: (reason: string) => void
  onCancel: () => void
}

export function DismissModal({ open, jobTitle, onConfirm, onCancel }: DismissModalProps) {
  const [selectedIdx, setSelectedIdx] = useState(0)

  return (
    <Dialog open={open} onOpenChange={(v) => !v && onCancel()}>
      <DialogContent className="w-full max-w-sm border border-white/[0.07] bg-[#13131e] p-5 text-slate-200 shadow-2xl sm:rounded-xl [&>button]:hidden">
        <DialogHeader className="space-y-1">
          <DialogTitle className="text-sm font-medium text-white">Dismiss job</DialogTitle>
          <DialogDescription className="truncate text-xs text-slate-500">
            "{jobTitle}"
          </DialogDescription>
        </DialogHeader>
        <div className="mb-1 mt-3 space-y-2">
          {DISMISS_REASON_OPTIONS.map((opt, i) => (
            <button
              key={i}
              type="button"
              onClick={() => setSelectedIdx(i)}
              className={cn(
                'w-full rounded-lg border px-3 py-2.5 text-left text-sm transition-colors',
                selectedIdx === i
                  ? 'border-violet-500/40 bg-violet-600/20 text-violet-300'
                  : 'border-white/[0.07] bg-white/[0.03] text-slate-300 hover:bg-white/[0.06]'
              )}
            >
              {opt.label}
            </button>
          ))}
        </div>
        <DialogFooter className="mt-2 flex-row gap-2 sm:space-x-0">
          <button
            type="button"
            onClick={onCancel}
            className="flex-1 rounded-lg border border-white/10 bg-white/5 px-4 py-2 text-sm text-slate-400 transition-colors hover:bg-white/[0.08]"
          >
            Cancel
          </button>
          <button
            type="button"
            onClick={() => onConfirm(DISMISS_REASON_OPTIONS[selectedIdx].value)}
            className="flex-1 rounded-lg bg-red-600/80 px-4 py-2 text-sm font-medium text-white transition-colors hover:bg-red-600"
          >
            Dismiss
          </button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
