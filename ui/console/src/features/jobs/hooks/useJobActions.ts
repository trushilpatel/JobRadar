import { useCallback, useRef } from 'react'
import { useMutation, useQueryClient, type QueryClient } from '@tanstack/react-query'
import { toast } from 'sonner'
import { setJobStatus, setJobSaved, dismissJob, bulkDismissJobs, type JobsPage } from '@/api/jobs'
import type { Job, JobStatus } from '@/types'
import { JOB_STATUS } from '@/constants/jobStatus'
import { useUndoToast } from '@/lib/use-undo-toast'
import { isJobListKey } from '@/features/jobs/queryKeys'

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

const UNDO_STACK_CAP = 20
const BULK_CONCURRENCY = 5

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

/**
 * Run `fn` for each item in `items` with at most `limit` calls in flight at
 * once. Returns a Promise.allSettled-shaped result array. Avoids the 100-row
 * 429-storm pattern of `Promise.allSettled(ids.map(fn))`.
 */
async function runWithLimit<T, R>(
  items: T[],
  limit: number,
  fn: (item: T) => Promise<R>
): Promise<PromiseSettledResult<R>[]> {
  const results: PromiseSettledResult<R>[] = new Array(items.length)
  let cursor = 0
  async function worker() {
    while (cursor < items.length) {
      const i = cursor++
      try {
        const v = await fn(items[i])
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
 * Invalidate all caches that may have moved as a result of a user-job mutation:
 * jobs list + counts + detail, plus dashboard and insights (which aggregate
 * user_job_state). This is the single fan-out point — never invalidate
 * `['jobs']` alone, because the dashboard's funnel and the insights panels go
 * stale otherwise.
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

interface OptimisticContext extends Snapshots {
  priorStatus: JobStatus
}

/**
 * Snapshot every jobs-LIST cache so a failing mutation can roll back. Patch
 * each cache page's matching item via `patch`. Returns the snapshot list.
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
   * Find the current status of a job by scanning every `['jobs', ...]` list
   * cache. Returns NEW as a last-resort fallback when nothing is cached — that
   * fallback exists for first-paint races where a mutation lands before any
   * list query has populated the cache. The previous implementation scanned
   * `['jobs', 'list']` exclusively, which the actual writer never produced,
   * so every undo silently reverted to NEW.
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

  const pushHistory = useCallback((entry: HistoryEntry) => {
    historyRef.current.push(entry)
    // Session-scoped, capacity-bounded undo stack. Not persisted across page
    // reloads (intentional — see Designs/jobs-page-fixes.md).
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
      if (entry.kind === 'status' || entry.kind === 'dismiss') {
        await revertOne(entry.id, entry.priorStatus)
      } else {
        await runWithLimit(entry.items, BULK_CONCURRENCY, (i) => revertOne(i.id, i.priorStatus))
      }
      toast.success('Undone')
      invalidateJobs()
    } catch {
      toast.error('Undo failed')
    }
  }, [invalidateJobs])

  const setStatus = useMutation({
    mutationFn: async ({ id, status }: { id: string; status: SettableStatus }) => {
      if (status === JOB_STATUS.NEW) {
        await setJobSaved(id, false)
      } else {
        await setJobStatus(id, status)
      }
      return { id }
    },
    // onMutate runs BEFORE mutationFn. Capture priorStatus here, BEFORE the
    // optimistic patch overwrites the cache — otherwise lookupStatus inside
    // mutationFn would read back our own optimistic write.
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
      const priorStatus = (ctx as OptimisticContext | undefined)?.priorStatus ?? JOB_STATUS.NEW
      pushHistory({ kind: 'status', id, priorStatus })
      showUndoToast(
        status === JOB_STATUS.NEW ? 'Moved back to New' : `Moved to ${status}`,
        undoLast
      )
      invalidateJobs()
    },
    onError: (_err, _vars, ctx) => {
      if (ctx) restoreSnapshots(queryClient, ctx as OptimisticContext)
      toast.error('Failed to update status')
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
      const { snapshots } = snapshotAndPatch(queryClient, id, (job) => ({
        ...job,
        status: JOB_STATUS.DISMISSED,
        user_state: {
          ...job.user_state,
          status: JOB_STATUS.DISMISSED,
          dismissed_at: new Date().toISOString(),
        },
      }))
      return { snapshots, priorStatus }
    },
    onSuccess: ({ id }, _vars, ctx) => {
      const priorStatus = (ctx as OptimisticContext | undefined)?.priorStatus ?? JOB_STATUS.NEW
      pushHistory({ kind: 'dismiss', id, priorStatus })
      showUndoToast('Job dismissed', undoLast)
      invalidateJobs()
    },
    onError: (_err, _vars, ctx) => {
      if (ctx) restoreSnapshots(queryClient, ctx as OptimisticContext)
      toast.error('Failed to dismiss job')
    },
  })

  const bulkDismiss = useMutation({
    mutationFn: async (ids: string[]) => {
      const items = ids.map((id) => ({ id, priorStatus: lookupStatus(id) }))
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

  const bulkSave = useMutation({
    mutationFn: async (ids: string[]) => {
      const items = ids.map((id) => ({ id, priorStatus: lookupStatus(id) }))
      const results = await runWithLimit(ids, BULK_CONCURRENCY, (id) =>
        setJobStatus(id, JOB_STATUS.SAVED)
      )
      const succeeded = items.filter((_, i) => results[i].status === 'fulfilled')
      const failed = results.length - succeeded.length
      return { succeeded, failed, total: results.length }
    },
    onSuccess: ({ succeeded, failed, total }) => {
      pushHistory({ kind: 'bulkStatus', items: succeeded })
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
      const succeeded = items.filter((_, i) => results[i].status === 'fulfilled')
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

  return { setStatus, dismiss, bulkDismiss, bulkSave, bulkApplied, undoLast }
}
