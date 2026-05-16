import { useCallback, useRef } from 'react'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { toast } from 'sonner'
import { setJobStatus, setJobSaved, dismissJob, bulkDismissJobs, type JobsPage } from '@/api/jobs'
import type { Job, JobStatus } from '@/types'
import { JOB_STATUS } from '@/constants/jobStatus'
import { useUndoToast } from '@/lib/use-undo-toast'

type SettableStatus = Exclude<JobStatus, 'dismissed'>

interface SingleStatusEntry {
  kind: 'status'
  id: string
  priorStatus: JobStatus
}

interface SingleDismissEntry {
  kind: 'dismiss'
  id: string
  priorStatus: JobStatus
}

interface BulkStatusEntry {
  kind: 'bulkStatus'
  items: { id: string; priorStatus: JobStatus }[]
}

interface BulkDismissEntry {
  kind: 'bulkDismiss'
  items: { id: string; priorStatus: JobStatus }[]
}

type HistoryEntry = SingleStatusEntry | SingleDismissEntry | BulkStatusEntry | BulkDismissEntry

async function revertOne(id: string, priorStatus: JobStatus): Promise<void> {
  if (priorStatus === JOB_STATUS.NEW) {
    await setJobSaved(id, false)
    return
  }
  if (priorStatus === JOB_STATUS.DISMISSED) {
    return
  }
  await setJobStatus(id, priorStatus)
}

export function useJobActions() {
  const queryClient = useQueryClient()
  const historyRef = useRef<HistoryEntry[]>([])
  const showUndoToast = useUndoToast()

  const invalidateJobs = useCallback(
    () => queryClient.invalidateQueries({ queryKey: ['jobs'] }),
    [queryClient]
  )

  const lookupStatus = useCallback(
    (id: string): JobStatus => {
      const caches = queryClient.getQueriesData<{ pages: JobsPage[] }>({
        queryKey: ['jobs', 'list'],
      })
      for (const [, data] of caches) {
        if (!data?.pages) continue
        for (const page of data.pages) {
          const match = page.items.find((j: Job) => j.id === id)
          if (match) return match.status
        }
      }
      return JOB_STATUS.NEW
    },
    [queryClient]
  )

  const undoLast = useCallback(async () => {
    const entry = historyRef.current.pop()
    if (!entry) return
    try {
      if (entry.kind === 'status' || entry.kind === 'dismiss') {
        await revertOne(entry.id, entry.priorStatus)
      } else {
        await Promise.allSettled(entry.items.map((i) => revertOne(i.id, i.priorStatus)))
      }
      toast.success('Undone')
      invalidateJobs()
    } catch {
      toast.error('Undo failed')
    }
  }, [invalidateJobs])

  const setStatus = useMutation({
    mutationFn: async ({ id, status }: { id: string; status: SettableStatus }) => {
      const priorStatus = lookupStatus(id)
      if (status === JOB_STATUS.NEW) {
        await setJobSaved(id, false)
      } else {
        await setJobStatus(id, status)
      }
      return { id, priorStatus }
    },
    onSuccess: ({ id, priorStatus }, { status }) => {
      historyRef.current.push({ kind: 'status', id, priorStatus })
      showUndoToast(
        status === JOB_STATUS.NEW ? 'Moved back to New' : `Moved to ${status}`,
        undoLast
      )
      invalidateJobs()
    },
    onError: () => toast.error('Failed to update status'),
  })

  const dismiss = useMutation({
    mutationFn: async ({ id, reason, note }: { id: string; reason: string; note?: string }) => {
      const priorStatus = lookupStatus(id)
      await dismissJob(id, reason, note)
      return { id, priorStatus }
    },
    onSuccess: ({ id, priorStatus }) => {
      historyRef.current.push({ kind: 'dismiss', id, priorStatus })
      showUndoToast('Job dismissed', undoLast)
      invalidateJobs()
    },
    onError: () => toast.error('Failed to dismiss job'),
  })

  const bulkDismiss = useMutation({
    mutationFn: async (ids: string[]) => {
      const items = ids.map((id) => ({ id, priorStatus: lookupStatus(id) }))
      const result = await bulkDismissJobs(ids)
      return { result, items }
    },
    onSuccess: ({ result, items }) => {
      historyRef.current.push({ kind: 'bulkDismiss', items })
      showUndoToast(
        `${result.dismissed} job${result.dismissed !== 1 ? 's' : ''} dismissed`,
        undoLast
      )
      invalidateJobs()
    },
    onError: () => toast.error('Failed to dismiss jobs'),
  })

  const bulkSave = useMutation({
    mutationFn: async (ids: string[]) => {
      const items = ids.map((id) => ({ id, priorStatus: lookupStatus(id) }))
      const results = await Promise.allSettled(ids.map((id) => setJobStatus(id, JOB_STATUS.SAVED)))
      const succeeded = items.filter((_, i) => results[i].status === 'fulfilled')
      const failed = results.length - succeeded.length
      return { succeeded, failed, total: results.length }
    },
    onSuccess: ({ succeeded, failed, total }) => {
      historyRef.current.push({ kind: 'bulkStatus', items: succeeded })
      const message =
        failed > 0
          ? `${succeeded.length} of ${total} jobs saved (${failed} failed)`
          : `${succeeded.length} job${succeeded.length !== 1 ? 's' : ''} saved`
      showUndoToast(message, undoLast)
      invalidateJobs()
    },
    onError: () => toast.error('Failed to bulk save'),
  })

  const bulkApplied = useMutation({
    mutationFn: async (ids: string[]) => {
      const items = ids.map((id) => ({ id, priorStatus: lookupStatus(id) }))
      const results = await Promise.allSettled(
        ids.map((id) => setJobStatus(id, JOB_STATUS.APPLIED))
      )
      const succeeded = items.filter((_, i) => results[i].status === 'fulfilled')
      const failed = results.length - succeeded.length
      return { succeeded, failed, total: results.length }
    },
    onSuccess: ({ succeeded, failed, total }) => {
      historyRef.current.push({ kind: 'bulkStatus', items: succeeded })
      const message =
        failed > 0
          ? `${succeeded.length} of ${total} marked applied (${failed} failed)`
          : `${succeeded.length} job${succeeded.length !== 1 ? 's' : ''} marked applied`
      showUndoToast(message, undoLast)
      invalidateJobs()
    },
    onError: () => toast.error('Failed to bulk mark applied'),
  })

  return { setStatus, dismiss, bulkDismiss, bulkSave, bulkApplied, undoLast }
}
