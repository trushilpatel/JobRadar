import { useEffect, useState, useMemo, useRef } from 'react'
import {
  DndContext,
  DragOverlay,
  PointerSensor,
  useSensor,
  useSensors,
} from '@dnd-kit/core'
import type { DragEndEvent, DragStartEvent } from '@dnd-kit/core'
import { useDroppable, useDraggable } from '@dnd-kit/core'
import { X, ExternalLink } from 'lucide-react'
import type { ApplicationWithJob, ApplicationStage } from '@/types'
import { fetchAllApplications, updateApplicationStage, updateApplicationNote } from '@/api/applications'
import { cn } from '@/lib/utils'

// ─── Constants ────────────────────────────────────────────────────────────────

const STAGES: ApplicationStage[] = [
  'applied',
  'phone_screen',
  'technical',
  'final_round',
  'offer',
  'rejected',
]

const STAGE_LABELS: Record<string, string> = {
  applied: 'Applied',
  phone_screen: 'Phone Screen',
  technical: 'Technical',
  final_round: 'Final Round',
  offer: 'Offer',
  rejected: 'Rejected',
}

const STAGE_COLORS: Record<string, string> = {
  applied: 'text-blue-400',
  phone_screen: 'text-violet-400',
  technical: 'text-amber-400',
  final_round: 'text-orange-400',
  offer: 'text-emerald-400',
  rejected: 'text-red-400',
}

const STAGE_DOT_COLORS: Record<string, string> = {
  applied: 'bg-blue-400',
  phone_screen: 'bg-violet-400',
  technical: 'bg-amber-400',
  final_round: 'bg-orange-400',
  offer: 'bg-emerald-400',
  rejected: 'bg-red-400',
}

// ─── Helpers ──────────────────────────────────────────────────────────────────

function formatShortDate(dateStr: string): string {
  const d = new Date(dateStr)
  return d.toLocaleDateString('en-US', { month: 'short', day: 'numeric' })
}

// ─── Application Card ─────────────────────────────────────────────────────────

interface AppCardProps {
  app: ApplicationWithJob
  onClick: () => void
  isDragging?: boolean
}

function AppCard({ app, onClick, isDragging = false }: AppCardProps) {
  const { attributes, listeners, setNodeRef, transform } = useDraggable({ id: app.id })

  const style = transform
    ? { transform: `translate3d(${transform.x}px, ${transform.y}px, 0)` }
    : undefined

  return (
    <div
      ref={setNodeRef}
      style={style}
      {...listeners}
      {...attributes}
      onClick={onClick}
      className={cn(
        'bg-white/[0.04] border border-white/[0.07] rounded-xl p-3 cursor-pointer hover:bg-white/[0.07] hover:border-white/10 transition-colors select-none',
        isDragging && 'opacity-40'
      )}
    >
      <p className="text-sm font-medium text-white leading-snug">{app.company_name}</p>
      <p className="text-xs text-slate-400 mt-0.5 leading-snug">{app.job_title}</p>
      <p className="text-xs text-slate-500 mt-1.5">{formatShortDate(app.stage_at)}</p>
      {app.note && (
        <p className="text-xs text-slate-500 italic mt-1 line-clamp-2">{app.note}</p>
      )}
    </div>
  )
}

// ─── Application Card (overlay version, no dnd hooks) ────────────────────────

function AppCardOverlay({ app }: { app: ApplicationWithJob }) {
  return (
    <div className="bg-white/[0.08] border border-violet-500/40 rounded-xl p-3 shadow-2xl w-[220px] rotate-2 cursor-grabbing">
      <p className="text-sm font-medium text-white leading-snug">{app.company_name}</p>
      <p className="text-xs text-slate-400 mt-0.5 leading-snug">{app.job_title}</p>
      <p className="text-xs text-slate-500 mt-1.5">{formatShortDate(app.stage_at)}</p>
    </div>
  )
}

// ─── Column ───────────────────────────────────────────────────────────────────

interface ColumnProps {
  stage: ApplicationStage
  apps: ApplicationWithJob[]
  isDraggingAny: boolean
  onCardClick: (app: ApplicationWithJob) => void
  activeId: number | null
}

function KanbanColumn({ stage, apps, isDraggingAny, onCardClick, activeId }: ColumnProps) {
  const { setNodeRef, isOver } = useDroppable({ id: stage })

  return (
    <div className="flex flex-col w-[250px] shrink-0">
      {/* Column header */}
      <div className="flex items-center gap-2 mb-3 px-0.5">
        <span className={cn('w-2 h-2 rounded-full shrink-0', STAGE_DOT_COLORS[stage])} />
        <span className={cn('text-xs font-semibold uppercase tracking-wider', STAGE_COLORS[stage])}>
          {STAGE_LABELS[stage]}
        </span>
        <span className="ml-auto text-xs text-slate-500 bg-white/[0.06] px-1.5 py-0.5 rounded-md">
          {apps.length}
        </span>
      </div>

      {/* Card list */}
      <div
        ref={setNodeRef}
        className={cn(
          'flex-1 flex flex-col gap-2 min-h-[120px] rounded-xl p-2 transition-colors',
          isOver
            ? 'bg-violet-500/10 border border-violet-500/30'
            : 'bg-white/[0.02] border border-white/[0.04]'
        )}
      >
        {apps.map(app => (
          <AppCard
            key={app.id}
            app={app}
            onClick={() => onCardClick(app)}
            isDragging={activeId === app.id}
          />
        ))}

        {/* Empty drop target hint */}
        {apps.length === 0 && isDraggingAny && (
          <div className="flex-1 flex items-center justify-center border-2 border-dashed border-white/[0.10] rounded-lg min-h-[80px]">
            <span className="text-xs text-slate-600">Drop here</span>
          </div>
        )}
      </div>
    </div>
  )
}

// ─── Application Detail Drawer ────────────────────────────────────────────────

interface DrawerProps {
  app: ApplicationWithJob | null
  onClose: () => void
  onNoteChange: (appId: number, note: string) => void
}

function ApplicationDetailDrawer({ app, onClose, onNoteChange }: DrawerProps) {
  const [noteValue, setNoteValue] = useState('')
  const prevAppId = useRef<number | null>(null)

  // Sync note when app changes
  useEffect(() => {
    if (app && app.id !== prevAppId.current) {
      setNoteValue(app.note ?? '')
      prevAppId.current = app.id
    }
  }, [app])

  if (!app) return null

  function handleNoteBlur() {
    if (!app) return
    if (noteValue === (app.note ?? '')) return
    onNoteChange(app.id, noteValue)
  }

  return (
    <>
      {/* Backdrop */}
      <div
        className="fixed inset-0 z-20 bg-black/30"
        onClick={onClose}
      />

      {/* Drawer */}
      <div className="fixed inset-y-0 right-0 z-30 flex flex-col w-full max-w-md bg-[#0d0d14] border-l border-white/[0.07] shadow-2xl">
        {/* Header */}
        <div className="flex items-start justify-between px-5 py-4 border-b border-white/[0.07]">
          <div className="min-w-0 flex-1">
            <p className="text-base font-semibold text-white leading-tight truncate">{app.job_title}</p>
            <p className="text-sm text-slate-400 mt-0.5">{app.company_name}</p>
          </div>
          <button
            onClick={onClose}
            className="shrink-0 ml-3 p-1.5 text-slate-400 hover:text-white hover:bg-white/[0.06] rounded-lg transition-colors"
          >
            <X size={15} />
          </button>
        </div>

        {/* Body */}
        <div className="flex-1 overflow-y-auto px-5 py-4 space-y-5">
          {/* Stage badge */}
          <div>
            <p className="text-xs font-medium text-slate-400 uppercase tracking-wider mb-2">Stage</p>
            <span className={cn(
              'inline-flex items-center gap-1.5 text-sm font-medium px-3 py-1.5 rounded-lg bg-white/[0.05] border border-white/[0.07]',
              STAGE_COLORS[app.stage]
            )}>
              <span className={cn('w-1.5 h-1.5 rounded-full', STAGE_DOT_COLORS[app.stage])} />
              {STAGE_LABELS[app.stage]}
            </span>
          </div>

          {/* Stage date */}
          <div>
            <p className="text-xs font-medium text-slate-400 uppercase tracking-wider mb-1">Stage since</p>
            <p className="text-sm text-slate-300">{formatShortDate(app.stage_at)}</p>
          </div>

          {/* Notes */}
          <div>
            <p className="text-xs font-medium text-slate-400 uppercase tracking-wider mb-2">Notes</p>
            <textarea
              value={noteValue}
              onChange={e => setNoteValue(e.target.value)}
              onBlur={handleNoteBlur}
              placeholder="Add notes about this application…"
              rows={5}
              className="w-full bg-white/5 border border-white/10 rounded-lg px-3 py-2.5 text-sm text-white placeholder-slate-600 focus:outline-none focus:border-violet-500 transition-colors resize-none"
            />
            <p className="text-xs text-slate-600 mt-1">Auto-saves on blur</p>
          </div>
        </div>

        {/* Footer */}
        <div className="px-5 py-4 border-t border-white/[0.07]">
          <a
            href={`/jobs/${app.job_result_id}`}
            className="flex items-center gap-1.5 text-sm text-slate-300 hover:text-violet-300 bg-white/5 border border-white/10 px-3 py-2 rounded-lg transition-colors w-fit"
          >
            <ExternalLink size={13} />
            View job
          </a>
        </div>
      </div>
    </>
  )
}

// ─── Main Page ────────────────────────────────────────────────────────────────

export function ApplicationsPage() {
  const [apps, setApps] = useState<ApplicationWithJob[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [drawerApp, setDrawerApp] = useState<ApplicationWithJob | null>(null)
  const [activeId, setActiveId] = useState<number | null>(null)

  const sensors = useSensors(
    useSensor(PointerSensor, { activationConstraint: { distance: 8 } })
  )

  useEffect(() => {
    load()
  }, [])

  async function load() {
    setLoading(true)
    setError(null)
    try {
      const data = await fetchAllApplications()
      setApps(data ?? [])
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Failed to load applications')
    } finally {
      setLoading(false)
    }
  }

  const grouped = useMemo(() => {
    const map: Record<string, ApplicationWithJob[]> = {}
    for (const s of STAGES) map[s] = []
    for (const a of apps) {
      if (map[a.stage]) map[a.stage].push(a)
    }
    return map
  }, [apps])

  const inProgressCount = useMemo(() => {
    return apps.filter(a => a.stage !== 'rejected' && a.stage !== 'offer').length
  }, [apps])

  function handleDragStart(event: DragStartEvent) {
    setActiveId(event.active.id as number)
  }

  function handleDragEnd(event: DragEndEvent) {
    const { active, over } = event
    setActiveId(null)
    if (!over) return
    const appId = active.id as number
    const newStage = over.id as string
    const app = apps.find(a => a.id === appId)
    if (!app || app.stage === newStage) return

    // Optimistic update
    setApps(prev =>
      prev.map(a => a.id === appId ? { ...a, stage: newStage as ApplicationStage } : a)
    )
    // If drawer is open for this app, update it too
    if (drawerApp?.id === appId) {
      setDrawerApp(d => d ? { ...d, stage: newStage as ApplicationStage } : d)
    }

    updateApplicationStage(app.job_result_id, appId, newStage).catch(() => {
      // Revert on failure
      setApps(prev => prev.map(a => a.id === appId ? { ...a, stage: app.stage } : a))
      if (drawerApp?.id === appId) {
        setDrawerApp(d => d ? { ...d, stage: app.stage } : d)
      }
    })
  }

  function handleNoteChange(appId: number, note: string) {
    const app = apps.find(a => a.id === appId)
    if (!app) return
    setApps(prev => prev.map(a => a.id === appId ? { ...a, note } : a))
    if (drawerApp?.id === appId) setDrawerApp(d => d ? { ...d, note } : d)
    updateApplicationNote(app.job_result_id, appId, note).catch(() => {
      // Revert
      setApps(prev => prev.map(a => a.id === appId ? { ...a, note: app.note } : a))
    })
  }

  const activeApp = activeId != null ? apps.find(a => a.id === activeId) : null

  return (
    <div className="flex-1 flex flex-col min-h-0">
      {/* Header */}
      <div className="flex items-center justify-between px-6 py-5 border-b border-white/[0.07]">
        <div>
          <h1 className="text-xl font-semibold text-white">Applications</h1>
          <p className="text-sm text-slate-400 mt-0.5">
            {apps.length} total · {inProgressCount} in progress
          </p>
        </div>
      </div>

      {/* Body */}
      {loading ? (
        <div className="flex-1 flex items-center justify-center">
          <div className="w-6 h-6 border-2 border-violet-500 border-t-transparent rounded-full animate-spin" />
        </div>
      ) : error ? (
        <div className="flex-1 flex items-center justify-center text-slate-500">
          {error}
        </div>
      ) : apps.length === 0 ? (
        <div className="flex-1 flex items-center justify-center px-6">
          <div className="text-center">
            <p className="text-slate-400 font-medium">No applications yet</p>
            <p className="text-slate-500 text-sm mt-1">
              Mark a job as applied to start tracking
            </p>
          </div>
        </div>
      ) : (
        <div className="flex-1 overflow-x-auto overflow-y-hidden">
          <DndContext
            sensors={sensors}
            onDragStart={handleDragStart}
            onDragEnd={handleDragEnd}
          >
            <div className="flex gap-4 px-6 py-5 h-full min-h-[calc(100vh-120px)]">
              {STAGES.map(stage => (
                <KanbanColumn
                  key={stage}
                  stage={stage}
                  apps={grouped[stage] ?? []}
                  isDraggingAny={activeId != null}
                  onCardClick={setDrawerApp}
                  activeId={activeId}
                />
              ))}
            </div>

            <DragOverlay>
              {activeApp ? <AppCardOverlay app={activeApp} /> : null}
            </DragOverlay>
          </DndContext>
        </div>
      )}

      {/* Detail Drawer */}
      <ApplicationDetailDrawer
        app={drawerApp}
        onClose={() => setDrawerApp(null)}
        onNoteChange={handleNoteChange}
      />
    </div>
  )
}
