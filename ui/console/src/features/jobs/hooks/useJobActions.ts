import { useCallback, useRef } from 'react'
import { useMutation, useQueryClient, type QueryClient } from '@tanstack/react-query'
import { toast } from 'sonner'
import {
  setJobStatus,
  setJobSaved,
  dismissJob,
  bulkDismissJobs,
  type JobsPage,
} from '../api/jobs.api'
import type { Job, JobStatus } from '@/types'
import { JOB_STATUS } from '@/config/constants/jobStatus'
import { useUndoToast } from '@/lib/hooks/use-undo-toast'
import { isJobListKey } from '../queryKeys'

/**
 * Statuses settable via the workflow `setStatus` mutation. Excludes:
 *  - 'saved'    — orthogonal flag; use the dedicated `setSaved` mutation
 *  - 'dismissed'— uses the dedicated `dismiss` mutation
 */
type SettableStatus = Exclude<JobStatus, 'saved' | 'dismissed'>

// ── Undo history entries ────────────────────────────────────────────────────
// `priorStatus` and `priorSaved` are captured BEFORE the mutation lands so
// undo can reverse the change. Saved is orthogonal to status — each has its
// own entry kind.

interface SingleStatusEntry {
  kind: 'status'
  id: string
  priorStatus: JobStatus
}

interface SingleSavedEntry {
  kind: 'saved'
  id: string
  priorSaved: boolean
}

interface SingleDismissEntry {
  kind: 'dismiss'
  id: string
  priorStatus: JobStatus
  priorSaved: boolean
}

interface BulkStatusEntry {
  kind: 'bulkStatus'
  items: { id: string; priorStatus: JobStatus }[]
}

interface BulkSavedEntry {
  kind: 'bulkSaved'
  items: { id: string; priorSaved: boolean }[]
}

interface BulkDismissEntry {
  kind: 'bulkDismiss'
  items: { id: string; priorStatus: JobStatus; priorSaved: boolean }[]
}

type HistoryEntry =
  | SingleStatusEntry
  | SingleSavedEntry
  | SingleDismissEntry
  | BulkStatusEntry
  | BulkSavedEntry
  | BulkDismissEntry

const UNDO_STACK_CAP = 20
const BULK_CONCURRENCY = 5

/**
 * Revert a single workflow-status entry to its prior value. With the sparse
 * model: priorStatus='new' means "no workflow stage" — backend interprets it
 * as `UPDATE ... SET status = NULL`. Saved state is preserved separately.
 *
 * 'saved' should never be a stored status under the sparse model, but it's
 * still in the JobStatus union (as a TAB key). Treat it as 'new' if
 * encountered — saved-revert is handled by setJobSaved separately.
 */
async function revertStatus(id: string, priorStatus: JobStatus): Promise<void> {
  if (priorStatus === JOB_STATUS.DISMISSED) {
    return
  }
  if (priorStatus === JOB_STATUS.SAVED) {
    await setJobStatus(id, JOB_STATUS.NEW)
    return
  }
  await setJobStatus(id, priorStatus)
}

async function runWithLimit<T, R>(
  items: T[],
  limit: number,
  fn: (item: T) => Promise<R>
): Promise<PromiseSettledResult<R>[]> {
  const results: PromiseSettledResult<R>[] = new Array<PromiseSettledResult<R>>(items.length)
  let cursor = 0
  async function worker() {
    while (cursor < items.length) {
      const i = cursor++
      const item = items[i]
      if (item === undefined) continue
      try {
        const v = await fn(item)
        results[i] = { status: 'fulfilled', value: v }
      } catch (err) {
        results[i] = { status: 'rejected', reason: err }
      }
    }
  }
  const workers = Array.from({ length: Math.min(limit, items.length) }, () => worker())
  await Promise.all(workers)
  return results
}

/**
 * Invalidate every cache potentially affected by a per-user-job mutation:
 * jobs list + counts + dashboard + insights.
 */
function invalidateUserData(queryClient: QueryClient): Promise<unknown> {
  return Promise.all([
    queryClient.invalidateQueries({ queryKey: ['jobs'] }),
    queryClient.invalidateQueries({ queryKey: ['dashboard'] }),
    queryClient.invalidateQueries({ queryKey: ['insights'] }),
  ])
}

interface Snapshots {
  snapshots: { queryKey: readonly unknown[]; data: unknown }[]
}

interface OptimisticStatusContext extends Snapshots {
  priorStatus: JobStatus
}

interface OptimisticSavedContext extends Snapshots {
  priorSaved: boolean
}

interface OptimisticDismissContext extends Snapshots {
  priorStatus: JobStatus
  priorSaved: boolean
}

/**
 * Snapshot every jobs-list cache and patch each matching row via `patch`.
 * Used for optimistic updates with rollback on mutation error.
 */
function snapshotAndPatch(
  queryClient: QueryClient,
  id: string,
  patch: (job: Job) => Job
): Snapshots {
  const entries = queryClient.getQueriesData<{ pages: JobsPage[] }>({
    predicate: (q) => isJobListKey(q.queryKey),
  })
  const snapshots: { queryKey: readonly unknown[]; data: unknown }[] = []
  for (const [key, data] of entries) {
    if (!data?.pages) continue
    snapshots.push({ queryKey: key, data })
    const nextPages = data.pages.map((page) => {
      if (!page.items.some((j: Job) => j.id === id)) return page
      return {
        ...page,
        items: page.items.map((j: Job) => (j.id === id ? patch(j) : j)),
      }
    })
    queryClient.setQueryData(key, { ...data, pages: nextPages })
  }
  return { snapshots }
}

function restoreSnapshots(queryClient: QueryClient, ctx: Snapshots) {
  for (const snap of ctx.snapshots) {
    queryClient.setQueryData(snap.queryKey, snap.data)
  }
}

export function useJobActions() {
  const queryClient = useQueryClient()
  const historyRef = useRef<HistoryEntry[]>([])
  const showUndoToast = useUndoToast()

  const invalidateJobs = useCallback(() => invalidateUserData(queryClient), [queryClient])

  /**
   * Find the current workflow status of a job by scanning every jobs list
   * cache. Returns NEW as a last-resort fallback for first-paint races where
   * a mutation lands before any list query has populated the cache.
   */
  const lookupStatus = useCallback(
    (id: string): JobStatus => {
      const caches = queryClient.getQueriesData<{ pages: JobsPage[] }>({
        predicate: (q) => isJobListKey(q.queryKey),
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

  /** Find the current saved flag of a job from cache. Defaults to false. */
  const lookupSaved = useCallback(
    (id: string): boolean => {
      const caches = queryClient.getQueriesData<{ pages: JobsPage[] }>({
        predicate: (q) => isJobListKey(q.queryKey),
      })
      for (const [, data] of caches) {
        if (!data?.pages) continue
        for (const page of data.pages) {
          const match = page.items.find((j: Job) => j.id === id)
          if (match) return match.user_state?.saved_at != null
        }
      }
      return false
    },
    [queryClient]
  )

  const pushHistory = useCallback((entry: HistoryEntry) => {
    historyRef.current.push(entry)
    if (historyRef.current.length > UNDO_STACK_CAP) {
      historyRef.current.splice(0, historyRef.current.length - UNDO_STACK_CAP)
    }
  }, [])

  const undoLast = useCallback(async () => {
    const entry = historyRef.current.pop()
    if (!entry) {
      toast.warning('Nothing to undo')
      return
    }
    try {
      switch (entry.kind) {
        case 'status':
          await revertStatus(entry.id, entry.priorStatus)
          break
        case 'saved':
          await setJobSaved(entry.id, entry.priorSaved)
          break
        case 'dismiss':
          await revertStatus(entry.id, entry.priorStatus)
          // Saved is orthogonal — restore separately if it had been set.
          await setJobSaved(entry.id, entry.priorSaved)
          break
        case 'bulkStatus':
          await runWithLimit(entry.items, BULK_CONCURRENCY, (i) =>
            revertStatus(i.id, i.priorStatus)
          )
          break
        case 'bulkSaved':
          await runWithLimit(entry.items, BULK_CONCURRENCY, (i) => setJobSaved(i.id, i.priorSaved))
          break
        case 'bulkDismiss':
          await runWithLimit(entry.items, BULK_CONCURRENCY, async (i) => {
            await revertStatus(i.id, i.priorStatus)
            await setJobSaved(i.id, i.priorSaved)
          })
          break
      }
      toast.success('Undone')
      invalidateJobs()
    } catch {
      toast.error('Undo failed')
    }
  }, [invalidateJobs])

  // ── setStatus — WORKFLOW only ─────────────────────────────────────────────
  // Sends `status` to the backend; never touches saved_at. Use for moving a
  // job through the applied → interviewing → offer → rejected funnel, or
  // back to 'new' (clears workflow status, leaves saved_at intact).
  const setStatus = useMutation({
    mutationFn: async ({ id, status }: { id: string; status: SettableStatus }) => {
      await setJobStatus(id, status)
      return { id }
    },
    onMutate: async ({ id, status }) => {
      await queryClient.cancelQueries({ predicate: (q) => isJobListKey(q.queryKey) })
      const priorStatus = lookupStatus(id)
      const { snapshots } = snapshotAndPatch(queryClient, id, (job) => ({
        ...job,
        status,
        user_state: { ...job.user_state, status },
      }))
      return { snapshots, priorStatus }
    },
    onSuccess: ({ id }, { status }, ctx) => {
      const priorStatus =
        (ctx as OptimisticStatusContext | undefined)?.priorStatus ?? JOB_STATUS.NEW
      pushHistory({ kind: 'status', id, priorStatus })
      showUndoToast(
        status === JOB_STATUS.NEW ? 'Moved back to New' : `Moved to ${status}`,
        undoLast
      )
      invalidateJobs()
    },
    onError: (_err, _vars, ctx) => {
      if (ctx) restoreSnapshots(queryClient, ctx as OptimisticStatusContext)
      toast.error('Failed to update status')
    },
  })

  // ── setSaved — ORTHOGONAL flag ────────────────────────────────────────────
  // Sends `saved_at = NOW()` (or NULL) to the backend; never touches status.
  // A saved job stays in whatever workflow stage it was in.
  const setSaved = useMutation({
    mutationFn: async ({ id, saved }: { id: string; saved: boolean }) => {
      await setJobSaved(id, saved)
      return { id }
    },
    onMutate: async ({ id, saved }) => {
      await queryClient.cancelQueries({ predicate: (q) => isJobListKey(q.queryKey) })
      const priorSaved = lookupSaved(id)
      const now = new Date().toISOString()
      const { snapshots } = snapshotAndPatch(queryClient, id, (job) => ({
        ...job,
        user_state: {
          ...job.user_state,
          saved_at: saved ? now : null,
        },
      }))
      return { snapshots, priorSaved }
    },
    onSuccess: ({ id }, { saved }, ctx) => {
      const priorSaved = (ctx as OptimisticSavedContext | undefined)?.priorSaved ?? false
      pushHistory({ kind: 'saved', id, priorSaved })
      showUndoToast(saved ? 'Saved' : 'Removed from saved', undoLast)
      invalidateJobs()
    },
    onError: (_err, _vars, ctx) => {
      if (ctx) restoreSnapshots(queryClient, ctx as OptimisticSavedContext)
      toast.error('Failed to update saved state')
    },
  })

  const dismiss = useMutation({
    mutationFn: async ({ id, reason, note }: { id: string; reason: string; note?: string }) => {
      await dismissJob(id, reason, note)
      return { id }
    },
    onMutate: async ({ id }) => {
      await queryClient.cancelQueries({ predicate: (q) => isJobListKey(q.queryKey) })
      const priorStatus = lookupStatus(id)
      const priorSaved = lookupSaved(id)
      const { snapshots } = snapshotAndPatch(queryClient, id, (job) => ({
        ...job,
        status: JOB_STATUS.DISMISSED,
        user_state: {
          ...job.user_state,
          status: JOB_STATUS.DISMISSED,
          dismissed_at: new Date().toISOString(),
        },
      }))
      return { snapshots, priorStatus, priorSaved }
    },
    onSuccess: ({ id }, _vars, ctx) => {
      const c = ctx as OptimisticDismissContext | undefined
      pushHistory({
        kind: 'dismiss',
        id,
        priorStatus: c?.priorStatus ?? JOB_STATUS.NEW,
        priorSaved: c?.priorSaved ?? false,
      })
      showUndoToast('Job dismissed', undoLast)
      invalidateJobs()
    },
    onError: (_err, _vars, ctx) => {
      if (ctx) restoreSnapshots(queryClient, ctx as OptimisticDismissContext)
      toast.error('Failed to dismiss job')
    },
  })

  const bulkDismiss = useMutation({
    mutationFn: async (ids: string[]) => {
      const items = ids.map((id) => ({
        id,
        priorStatus: lookupStatus(id),
        priorSaved: lookupSaved(id),
      }))
      const result = await bulkDismissJobs(ids)
      return { result, items }
    },
    onSuccess: ({ result, items }) => {
      pushHistory({ kind: 'bulkDismiss', items })
      showUndoToast(
        `${result.dismissed} job${result.dismissed !== 1 ? 's' : ''} dismissed`,
        undoLast
      )
      invalidateJobs()
    },
    onError: () => toast.error('Failed to dismiss jobs'),
  })

  // ── bulkSave — orthogonal saved flag, never touches workflow status ───────
  const bulkSave = useMutation({
    mutationFn: async (ids: string[]) => {
      const items = ids.map((id) => ({ id, priorSaved: lookupSaved(id) }))
      const results = await runWithLimit(ids, BULK_CONCURRENCY, (id) => setJobSaved(id, true))
      const succeeded = items.filter((_, i) => results[i]?.status === 'fulfilled')
      const failed = results.length - succeeded.length
      return { succeeded, failed, total: results.length }
    },
    onSuccess: ({ succeeded, failed, total }) => {
      pushHistory({ kind: 'bulkSaved', items: succeeded })
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
      const results = await runWithLimit(ids, BULK_CONCURRENCY, (id) =>
        setJobStatus(id, JOB_STATUS.APPLIED)
      )
      const succeeded = items.filter((_, i) => results[i]?.status === 'fulfilled')
      const failed = results.length - succeeded.length
      return { succeeded, failed, total: results.length }
    },
    onSuccess: ({ succeeded, failed, total }) => {
      pushHistory({ kind: 'bulkStatus', items: succeeded })
      const message =
        failed > 0
          ? `${succeeded.length} of ${total} marked applied (${failed} failed)`
          : `${succeeded.length} job${succeeded.length !== 1 ? 's' : ''} marked applied`
      showUndoToast(message, undoLast)
      invalidateJobs()
    },
    onError: () => toast.error('Failed to bulk mark applied'),
  })

  return { setStatus, setSaved, dismiss, bulkDismiss, bulkSave, bulkApplied, undoLast }
}
