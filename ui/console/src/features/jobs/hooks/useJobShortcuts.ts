import { useEffect } from 'react'
import type { Job, JobStatus } from '@/types'
import { JOB_STATUS } from '@/constants/jobStatus'

type SettableStatus = Exclude<JobStatus, 'dismissed'>

interface UseJobShortcutsArgs {
  activeJob: Job | null
  onSetStatus: (job: Job, status: SettableStatus) => void
  onDismiss: (job: Job) => void
  enabled?: boolean
}

export const JOB_SHORTCUTS: ReadonlyArray<{ key: string; label: string }> = [
  { key: 's', label: 'Save' },
  { key: 'a', label: 'Applied' },
  { key: 'i', label: 'Interviewing' },
  { key: 'o', label: 'Offer' },
  { key: 'r', label: 'Rejected' },
  { key: 'd', label: 'Dismiss' },
  { key: '↑ / ↓', label: 'Move selection' },
]

const KEY_TO_STATUS: Record<string, SettableStatus> = {
  s: JOB_STATUS.SAVED,
  a: JOB_STATUS.APPLIED,
  i: JOB_STATUS.INTERVIEWING,
  o: JOB_STATUS.OFFER,
  r: JOB_STATUS.REJECTED,
}

function isTypingTarget(target: EventTarget | null): boolean {
  if (!(target instanceof HTMLElement)) return false
  const tag = target.tagName
  if (tag === 'INPUT' || tag === 'TEXTAREA' || tag === 'SELECT') return true
  if (target.isContentEditable) return true
  return false
}

export function useJobShortcuts({
  activeJob,
  onSetStatus,
  onDismiss,
  enabled = true,
}: UseJobShortcutsArgs) {
  useEffect(() => {
    if (!enabled) return

    function handler(e: KeyboardEvent) {
      if (e.metaKey || e.ctrlKey || e.altKey) return
      if (isTypingTarget(e.target)) return
      if (!activeJob) return

      const key = e.key.toLowerCase()
      if (key === 'd') {
        e.preventDefault()
        onDismiss(activeJob)
        return
      }
      const status = KEY_TO_STATUS[key]
      if (status) {
        e.preventDefault()
        onSetStatus(activeJob, status)
      }
    }

    window.addEventListener('keydown', handler)
    return () => window.removeEventListener('keydown', handler)
  }, [activeJob, onSetStatus, onDismiss, enabled])
}
