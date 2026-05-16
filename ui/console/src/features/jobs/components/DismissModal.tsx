import { useEffect, useRef, useState } from 'react'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { cn } from '@/lib/utils'

/**
 * Visible reason labels for the dismiss flow. Backend currently constrains
 * `reason` to two enum values (`user_dismissed` / `user_not_relevant`), so
 * the visible label is also passed through as the `note` prefix (unless the
 * user has typed a custom note) — this preserves the nuance without a
 * backend migration. See docs/superpowers/specs/2026-05-16-jobs-page-fixes.md.
 */
const DISMISS_REASON_OPTIONS: { value: string; label: string }[] = [
  { value: 'user_dismissed', label: 'Not relevant' },
  { value: 'user_not_relevant', label: 'Too junior / senior' },
  { value: 'user_not_relevant', label: 'Bad location' },
  { value: 'user_not_relevant', label: 'Compensation too low' },
  { value: 'user_dismissed', label: 'Company not a fit' },
  { value: 'user_dismissed', label: 'Other' },
]

const NOTE_MAX = 500

export interface DismissPayload {
  reason: string
  note?: string
}

export interface DismissModalProps {
  open: boolean
  /** Shown in the subtitle when `count` is 1. */
  jobTitle: string
  /** Defaults to 1. When > 1, modal switches to bulk mode. */
  count?: number
  onConfirm: (payload: DismissPayload) => void
  onCancel: () => void
}

export function DismissModal({
  open,
  jobTitle,
  count = 1,
  onConfirm,
  onCancel,
}: DismissModalProps) {
  const [selectedIdx, setSelectedIdx] = useState(0)
  const [note, setNote] = useState('')
  const radioRefs = useRef<(HTMLButtonElement | null)[]>([])

  // Reset transient state when the modal closes so the next open starts clean.
  useEffect(() => {
    if (!open) {
      setSelectedIdx(0)
      setNote('')
    }
  }, [open])

  // Focus the selected radio on open for keyboard users.
  useEffect(() => {
    if (open) {
      // Defer to next tick — the dialog content needs to mount first.
      requestAnimationFrame(() => {
        radioRefs.current[selectedIdx]?.focus()
      })
    }
    // We only want to focus on open, not on every selectedIdx change.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open])

  const isBulk = count > 1
  const title = isBulk ? `Dismiss ${count} jobs?` : 'Dismiss job'
  const subtitle = isBulk ? `Apply this reason to all ${count} selected.` : `"${jobTitle}"`
  const confirmLabel = isBulk ? `Dismiss ${count}` : 'Dismiss'

  function handleConfirm() {
    const opt = DISMISS_REASON_OPTIONS[selectedIdx]
    const trimmed = note.trim()
    const finalNote = trimmed.length > 0 ? trimmed : opt.label
    onConfirm({ reason: opt.value, note: finalNote })
  }

  // ARIA radio-group keyboard pattern: arrow keys move and activate, Home /
  // End jump to ends, Space / Enter confirm the visible selection.
  function handleRadioKeyDown(e: React.KeyboardEvent<HTMLDivElement>) {
    const last = DISMISS_REASON_OPTIONS.length - 1
    let next: number | null = null
    if (e.key === 'ArrowDown' || e.key === 'ArrowRight') next = Math.min(selectedIdx + 1, last)
    else if (e.key === 'ArrowUp' || e.key === 'ArrowLeft') next = Math.max(selectedIdx - 1, 0)
    else if (e.key === 'Home') next = 0
    else if (e.key === 'End') next = last
    if (next !== null) {
      e.preventDefault()
      setSelectedIdx(next)
      radioRefs.current[next]?.focus()
    }
  }

  const noteId = 'dismiss-note'
  const groupLabelId = 'dismiss-reason-group'

  return (
    <Dialog open={open} onOpenChange={(v) => !v && onCancel()}>
      <DialogContent className="w-full max-w-sm border border-white/[0.07] bg-[#13131e] p-5 text-slate-200 shadow-2xl sm:rounded-xl [&>button]:hidden">
        <DialogHeader className="space-y-1">
          <DialogTitle className="text-sm font-medium text-white">{title}</DialogTitle>
          <DialogDescription className="truncate text-xs text-slate-500">
            {subtitle}
          </DialogDescription>
        </DialogHeader>

        <div
          role="radiogroup"
          aria-labelledby={groupLabelId}
          onKeyDown={handleRadioKeyDown}
          className="mb-1 mt-3 space-y-2"
        >
          <span id={groupLabelId} className="sr-only">
            Reason for dismissing
          </span>
          {DISMISS_REASON_OPTIONS.map((opt, i) => {
            const checked = selectedIdx === i
            return (
              <button
                key={`${opt.label}-${i}`}
                ref={(el) => {
                  radioRefs.current[i] = el
                }}
                type="button"
                role="radio"
                aria-checked={checked}
                tabIndex={checked ? 0 : -1}
                onClick={() => setSelectedIdx(i)}
                className={cn(
                  'w-full rounded-lg border px-3 py-2.5 text-left text-sm transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-violet-400',
                  checked
                    ? 'border-violet-500/40 bg-violet-600/20 text-violet-300'
                    : 'border-white/[0.07] bg-white/[0.03] text-slate-300 hover:bg-white/[0.06]'
                )}
              >
                {opt.label}
              </button>
            )
          })}
        </div>

        <div className="mt-3 space-y-1">
          <label htmlFor={noteId} className="block text-xs text-slate-400">
            Note (optional)
          </label>
          <textarea
            id={noteId}
            value={note}
            onChange={(e) => setNote(e.target.value.slice(0, NOTE_MAX))}
            maxLength={NOTE_MAX}
            rows={3}
            placeholder="Anything else? (visible only to you)"
            aria-label="Note (optional)"
            className="w-full resize-y rounded-lg border border-white/[0.07] bg-white/[0.03] px-3 py-2 text-sm text-slate-200 placeholder:text-slate-600 focus-visible:border-violet-500/40 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-violet-400"
          />
          <p className="text-[10px] text-slate-600">
            {note.length}/{NOTE_MAX}
          </p>
        </div>

        <DialogFooter className="mt-2 flex-row gap-2 sm:space-x-0">
          <button
            type="button"
            onClick={onCancel}
            className="flex-1 rounded-lg border border-white/10 bg-white/5 px-4 py-2 text-sm text-slate-400 transition-colors hover:bg-white/[0.08] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-violet-400"
          >
            Cancel
          </button>
          <button
            type="button"
            onClick={handleConfirm}
            className="flex-1 rounded-lg bg-red-600/80 px-4 py-2 text-sm font-medium text-white transition-colors hover:bg-red-600 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-red-300"
          >
            {confirmLabel}
          </button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
