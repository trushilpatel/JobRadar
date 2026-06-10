import { useState } from 'react'
import { toast } from 'sonner'
import { Check, X } from 'lucide-react'
import { cn } from '@/lib/utils/utils'
import { hiddenItems } from '@/lib/hooks/hidden-items'

/**
 * "DismissStripe" — Sonner-based toast template for destructive dismiss flows.
 *
 * Differs from {@link useUndoToast} by adding an optional "Don't show this
 * [item] again" checkbox that persists into the `hidden-items` registry, so
 * future renders of the dismissed item can be filtered out.
 *
 * Generic via the `hideTarget.category` field — works for companies today,
 * tomorrow for roles, locations, tags, anything you want a one-click forever-
 * block on.
 *
 * Usage:
 *   showDismissStripe({
 *     message: 'Senior Software Engineer',
 *     submessage: 'at Stripe',
 *     onUndo: () => actions.undoLast(),
 *     hideTarget: { category: 'companies', value: 'Stripe', label: 'Stripe' },
 *   })
 */

export interface HideTarget {
  /** Category passed through to `hiddenItems.add(category, value)`. */
  category: string
  /** The actual key persisted into the hidden list (e.g. company name). */
  value: string
  /**
   * Human-readable label used in the checkbox copy:
   *   `Don't show ${label} again`.
   * Defaults to `value`.
   */
  label?: string
}

export interface DismissStripeOptions {
  /** Headline of the toast. Required. */
  message: string
  /** Optional second line (e.g. company name, sub-context). */
  submessage?: string
  /** Click handler for the "Undo" action. Omit to hide the button. */
  onUndo?: () => void
  /** When provided, the checkbox is shown and wires into hidden-items. */
  hideTarget?: HideTarget
  /** Sonner duration override (ms). Default 8000. */
  duration?: number
}

const DEFAULT_DURATION = 8000

interface DismissStripeContentProps extends DismissStripeOptions {
  toastId: string | number
}

// eslint-disable-next-line react-refresh/only-export-components
function DismissStripeContent({
  message,
  submessage,
  onUndo,
  hideTarget,
  toastId,
}: DismissStripeContentProps) {
  // Tracks the checkbox's visible state. Persists to localStorage on change.
  const [hideChecked, setHideChecked] = useState(false)

  const handleHideToggle = (next: boolean) => {
    if (!hideTarget) return
    setHideChecked(next)
    if (next) hiddenItems.add(hideTarget.category, hideTarget.value)
    else hiddenItems.remove(hideTarget.category, hideTarget.value)
  }

  const handleUndo = () => {
    // If user checked the box then hits Undo, revert both effects so the user
    // is left with no surprise side-effects from a single mistake.
    if (hideChecked && hideTarget) {
      hiddenItems.remove(hideTarget.category, hideTarget.value)
    }
    onUndo?.()
    toast.dismiss(toastId)
  }

  const handleClose = () => {
    toast.dismiss(toastId)
  }

  const hideLabel = hideTarget?.label ?? hideTarget?.value ?? ''

  return (
    <div
      data-testid="dismiss-stripe"
      className="max-w-[360px] w-full rounded-xl border border-[var(--border-strong)] bg-panel shadow-2xl overflow-hidden"
    >
      <div className="flex items-start gap-3 px-4 py-3">
        <div className="flex items-center justify-center w-7 h-7 rounded-full shrink-0 bg-emerald-500/15">
          <Check size={14} strokeWidth={2.5} className="text-emerald-400" />
        </div>
        <div className="flex-1 min-w-0">
          <p className="text-sm font-medium text-primary">{message}</p>
          {submessage && (
            <p className="text-xs text-muted truncate" title={submessage}>
              {submessage}
            </p>
          )}
        </div>
        <div className="flex items-center gap-1 shrink-0">
          {onUndo && (
            <button
              type="button"
              onClick={handleUndo}
              className="text-xs font-medium text-violet-300 hover:text-violet-200 px-2 py-1 rounded-md hover:bg-hover transition-colors"
            >
              Undo
            </button>
          )}
          <button
            type="button"
            onClick={handleClose}
            aria-label="Dismiss toast"
            className="p-1 text-muted hover:text-primary rounded-md hover:bg-hover transition-colors"
          >
            <X size={13} />
          </button>
        </div>
      </div>

      {hideTarget && (
        <div className="px-4 pb-3">
          <label
            className={cn(
              'flex items-center gap-2 text-xs cursor-pointer transition-colors',
              hideChecked ? 'text-violet-300' : 'text-muted hover:text-primary'
            )}
          >
            <input
              type="checkbox"
              checked={hideChecked}
              onChange={(e) => handleHideToggle(e.target.checked)}
              className="w-3.5 h-3.5 rounded cursor-pointer accent-violet-500"
            />
            <span>
              Don&apos;t show <span className="font-medium">{hideLabel}</span> again
            </span>
          </label>
        </div>
      )}
    </div>
  )
}

export function showDismissStripe(options: DismissStripeOptions): string | number {
  return toast.custom((id) => <DismissStripeContent {...options} toastId={id} />, {
    duration: options.duration ?? DEFAULT_DURATION,
  })
}
