import { useCallback } from 'react'
import { toast } from 'sonner'

export interface UndoToastOptions {
  /** Toast duration in milliseconds. Default: 5000. */
  duration?: number
}

/**
 * Returns a function that displays a `sonner` toast with an "Undo" action.
 *
 * The Toaster itself is mounted globally in App.tsx; this hook simply wraps
 * the `toast()` call so destructive jobs-page actions don't have to repeat
 * the action shape at every call site.
 */
export function useUndoToast() {
  return useCallback((message: string, undo: () => void, opts: UndoToastOptions = {}) => {
    return toast(message, {
      action: { label: 'Undo', onClick: undo },
      duration: opts.duration ?? 5000,
    })
  }, [])
}
