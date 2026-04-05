import { useEffect, useState, useMemo, useCallback } from 'react'
import * as Dialog from '@radix-ui/react-dialog'
import {
  Search, X, Bookmark, BookmarkCheck, Send,
  Trash2, ExternalLink, MapPin, Wifi, Briefcase,
  ChevronRight, RefreshCw, DollarSign,
} from 'lucide-react'
import type { Job, SalaryRange } from '@/types'
import {
  fetchJobs, setJobSaved, setJobStatus, dismissJob,
} from '@/api/jobs'
import { formatRelativeTime, cn } from '@/lib/utils'

// ─── Dismiss reasons ────────────────────────────────────────────────────────
// The API only accepts "user_dismissed" | "user_not_relevant" (model/constants.go)
const DISMISS_REASON_OPTIONS = [
  { value: 'user_dismissed',    label: 'Not relevant' },
  { value: 'user_not_relevant', label: 'Too junior / senior' },
  { value: 'user_not_relevant', label: 'Bad location' },
  { value: 'user_not_relevant', label: 'Compensation too low' },
  { value: 'user_dismissed',    label: 'Company not a fit' },
  { value: 'user_dismissed',    label: 'Other' },
]

type TabKey = 'new' | 'saved' | 'applied'

// ─── Helpers ─────────────────────────────────────────────────────────────────

function formatSalary(salary: SalaryRange | null | undefined): string | null {
  if (!salary) return null
  const currency = salary.currency ?? '$'
  const fmt = (n: number) =>
    n >= 1000 ? `${currency}${Math.round(n / 1000)}k` : `${currency}${n}`
  if (salary.min && salary.max) return `${fmt(salary.min)} – ${fmt(salary.max)}`
  if (salary.min) return `${fmt(salary.min)}+`
  if (salary.max) return `up to ${fmt(salary.max)}`
  return null
}

function companyInitial(name: string): string {
  return name ? name[0].toUpperCase() : '?'
}

const AVATAR_COLORS = [
  'bg-violet-600', 'bg-blue-600', 'bg-emerald-600',
  'bg-amber-600',  'bg-rose-600', 'bg-cyan-600',
  'bg-indigo-600', 'bg-teal-600',
]
function avatarColor(name: string): string {
  let hash = 0
  for (let i = 0; i < name.length; i++) hash = name.charCodeAt(i) + ((hash << 5) - hash)
  return AVATAR_COLORS[Math.abs(hash) % AVATAR_COLORS.length]
}

// ─── Sub-components ───────────────────────────────────────────────────────────

function CompanyAvatar({ name, size = 'md' }: { name: string; size?: 'sm' | 'md' }) {
  const sz = size === 'sm' ? 'w-7 h-7 text-xs' : 'w-9 h-9 text-sm'
  return (
    <div className={cn('rounded-lg flex items-center justify-center font-semibold text-white shrink-0', sz, avatarColor(name))}>
      {companyInitial(name)}
    </div>
  )
}

function TechTag({ tag }: { tag: string }) {
  return (
    <span className="text-xs bg-white/[0.07] text-slate-300 px-2 py-0.5 rounded-md font-mono">
      {tag}
    </span>
  )
}

// ─── Dismiss modal ────────────────────────────────────────────────────────────

interface DismissModalProps {
  open: boolean
  jobTitle: string
  onConfirm: (reason: string) => void
  onCancel: () => void
}

function DismissModal({ open, jobTitle, onConfirm, onCancel }: DismissModalProps) {
  const [selectedIdx, setSelectedIdx] = useState(0)

  return (
    <Dialog.Root open={open} onOpenChange={v => !v && onCancel()}>
      <Dialog.Portal>
        <Dialog.Overlay className="fixed inset-0 bg-black/60 z-40" />
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
                    : 'bg-white/[0.03] border-white/[0.07] text-slate-300 hover:bg-white/[0.06]'
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

// ─── Job detail drawer ────────────────────────────────────────────────────────

interface DrawerProps {
  job: Job | null
  onClose: () => void
  onSave: (job: Job) => void
  onApply: (job: Job) => void
  onDismiss: (job: Job) => void
}

function JobDetailDrawer({ job, onClose, onSave, onApply, onDismiss }: DrawerProps) {
  if (!job) return null

  const salary = formatSalary(job.salary_range)
  const techStack: string[] = Array.isArray(job.tech_stack) ? job.tech_stack : []
  const companyName = job.company?.name ?? job.company_id

  return (
    <div className={cn(
      'fixed inset-y-0 right-0 z-30 flex flex-col w-full max-w-lg bg-[#0d0d14] border-l border-white/[0.07] shadow-2xl transition-transform duration-300',
      job ? 'translate-x-0' : 'translate-x-full'
    )}>
      {/* Drawer header */}
      <div className="flex items-start justify-between px-5 py-4 border-b border-white/[0.07]">
        <div className="flex items-start gap-3 min-w-0">
          <CompanyAvatar name={companyName} />
          <div className="min-w-0">
            <p className="text-base font-semibold text-white leading-tight truncate">
              {job.title}
            </p>
            <p className="text-sm text-slate-400 mt-0.5 truncate">
              {companyName}
            </p>
          </div>
        </div>
        <button
          onClick={onClose}
          className="shrink-0 ml-3 p-1.5 text-slate-400 hover:text-white hover:bg-white/[0.06] rounded-lg transition-colors"
        >
          <X size={15} />
        </button>
      </div>

      {/* Drawer body */}
      <div className="flex-1 overflow-y-auto px-5 py-4 space-y-5">
        {/* Meta row */}
        <div className="flex flex-wrap gap-2">
          {job.location && (
            <span className="flex items-center gap-1 text-xs text-slate-400 bg-white/[0.05] border border-white/[0.07] px-2.5 py-1 rounded-lg">
              <MapPin size={11} /> {job.location}
            </span>
          )}
          {job.remote && (
            <span className="flex items-center gap-1 text-xs text-emerald-400 bg-emerald-500/10 border border-emerald-500/20 px-2.5 py-1 rounded-lg">
              <Wifi size={11} /> Remote
            </span>
          )}
          {salary && (
            <span className="flex items-center gap-1 text-xs text-amber-300 bg-amber-500/10 border border-amber-500/20 px-2.5 py-1 rounded-lg">
              <DollarSign size={11} /> {salary}
            </span>
          )}
          {job.employ_type && (
            <span className="flex items-center gap-1 text-xs text-slate-400 bg-white/[0.05] border border-white/[0.07] px-2.5 py-1 rounded-lg">
              <Briefcase size={11} /> {job.employ_type.replace('_', ' ')}
            </span>
          )}
          {(job.years_exp_min || job.years_exp_max) && (
            <span className="text-xs text-slate-400 bg-white/[0.05] border border-white/[0.07] px-2.5 py-1 rounded-lg">
              {job.years_exp_min ?? 0}–{job.years_exp_max ?? '?'} yrs exp
            </span>
          )}
        </div>

        {/* Posted */}
        <p className="text-xs text-slate-500">
          Added {formatRelativeTime(job.added_at)}
        </p>

        {/* LLM Summary */}
        {job.llm_summary && (
          <div>
            <h3 className="text-xs font-medium text-slate-400 uppercase tracking-wider mb-2">Summary</h3>
            <p className="text-sm text-slate-300 leading-relaxed">{job.llm_summary}</p>
          </div>
        )}

        {/* Tech stack */}
        {techStack.length > 0 && (
          <div>
            <h3 className="text-xs font-medium text-slate-400 uppercase tracking-wider mb-2">Tech stack</h3>
            <div className="flex flex-wrap gap-1.5">
              {techStack.map(t => <TechTag key={t} tag={t} />)}
            </div>
          </div>
        )}

        {/* Notes */}
        {job.notes && (
          <div>
            <h3 className="text-xs font-medium text-slate-400 uppercase tracking-wider mb-2">Notes</h3>
            <p className="text-sm text-slate-300 leading-relaxed">{job.notes}</p>
          </div>
        )}
      </div>

      {/* Drawer footer actions */}
      <div className="px-5 py-4 border-t border-white/[0.07] flex items-center gap-2">
        <a
          href={job.url}
          target="_blank"
          rel="noreferrer"
          className="flex items-center gap-1.5 text-sm text-slate-300 hover:text-violet-300 bg-white/5 border border-white/10 px-3 py-2 rounded-lg transition-colors mr-auto"
        >
          <ExternalLink size={13} />
          View original
        </a>

        {!job.saved && job.status !== 'applied' && (
          <button
            onClick={() => onSave(job)}
            className="flex items-center gap-1.5 text-sm text-white bg-violet-600 hover:bg-violet-500 px-3 py-2 rounded-lg transition-colors"
          >
            <Bookmark size={13} />
            Save
          </button>
        )}

        {job.status !== 'applied' && (
          <button
            onClick={() => onApply(job)}
            className="flex items-center gap-1.5 text-sm text-white bg-emerald-600 hover:bg-emerald-500 px-3 py-2 rounded-lg transition-colors"
          >
            <Send size={13} />
            Mark applied
          </button>
        )}

        {job.status !== 'applied' && (
          <button
            onClick={() => onDismiss(job)}
            className="p-2 text-slate-400 hover:text-red-400 hover:bg-red-500/10 rounded-lg transition-colors"
            title="Dismiss"
          >
            <Trash2 size={14} />
          </button>
        )}
      </div>
    </div>
  )
}

// ─── Job card ─────────────────────────────────────────────────────────────────

interface JobCardProps {
  job: Job
  onClick: () => void
  onSave: () => void
  onApply: () => void
  onDismiss: () => void
}

function JobCard({ job, onClick, onSave, onApply, onDismiss }: JobCardProps) {
  const salary = formatSalary(job.salary_range)
  const techStack: string[] = Array.isArray(job.tech_stack) ? job.tech_stack : []
  const companyName = job.company?.name ?? job.company_id

  return (
    <div
      className="group relative bg-white/[0.04] border border-white/[0.07] rounded-xl p-4 hover:bg-white/[0.06] hover:border-white/10 transition-colors cursor-pointer"
      onClick={onClick}
    >
      <div className="flex items-start gap-3">
        <CompanyAvatar name={companyName} />

        <div className="flex-1 min-w-0">
          {/* Title + company */}
          <p className="text-sm font-semibold text-white group-hover:text-violet-300 transition-colors leading-snug">
            {job.title}
          </p>
          <p className="text-xs text-slate-400 mt-0.5">{companyName}</p>

          {/* Meta badges */}
          <div className="flex flex-wrap items-center gap-1.5 mt-2">
            {job.location && (
              <span className="flex items-center gap-0.5 text-xs text-slate-500">
                <MapPin size={10} /> {job.location}
              </span>
            )}
            {job.remote && (
              <span className="flex items-center gap-0.5 text-xs text-emerald-400 bg-emerald-500/10 px-1.5 py-0.5 rounded-md">
                <Wifi size={10} /> Remote
              </span>
            )}
            {salary && (
              <span className="flex items-center gap-0.5 text-xs text-amber-300">
                <DollarSign size={10} /> {salary}
              </span>
            )}
          </div>

          {/* Tech stack */}
          {techStack.length > 0 && (
            <div className="flex flex-wrap gap-1 mt-2">
              {techStack.slice(0, 5).map(t => <TechTag key={t} tag={t} />)}
              {techStack.length > 5 && (
                <span className="text-xs text-slate-500 px-2 py-0.5">+{techStack.length - 5}</span>
              )}
            </div>
          )}
        </div>

        {/* Right side: time + chevron */}
        <div className="shrink-0 flex flex-col items-end gap-2">
          <span className="text-xs text-slate-500">{formatRelativeTime(job.added_at)}</span>
          <ChevronRight size={14} className="text-slate-600 group-hover:text-slate-400 transition-colors" />
        </div>
      </div>

      {/* Saved badge */}
      {job.saved && (
        <div className="absolute top-3 right-10">
          <BookmarkCheck size={13} className="text-violet-400" />
        </div>
      )}

      {/* Hover actions */}
      <div
        className="flex items-center gap-1 mt-3 pt-3 border-t border-white/[0.05] opacity-0 group-hover:opacity-100 transition-opacity"
        onClick={e => e.stopPropagation()}
      >
        {!job.saved && job.status !== 'applied' && (
          <button
            onClick={onSave}
            className="flex items-center gap-1 text-xs text-slate-400 hover:text-violet-300 hover:bg-violet-500/10 px-2 py-1.5 rounded-lg transition-colors"
          >
            <Bookmark size={12} /> Save
          </button>
        )}
        {job.saved && job.status !== 'applied' && (
          <button
            onClick={onSave}
            className="flex items-center gap-1 text-xs text-violet-400 hover:text-violet-300 hover:bg-violet-500/10 px-2 py-1.5 rounded-lg transition-colors"
          >
            <BookmarkCheck size={12} /> Saved
          </button>
        )}
        {job.status !== 'applied' && (
          <button
            onClick={onApply}
            className="flex items-center gap-1 text-xs text-slate-400 hover:text-emerald-300 hover:bg-emerald-500/10 px-2 py-1.5 rounded-lg transition-colors"
          >
            <Send size={12} /> Mark applied
          </button>
        )}
        {job.status === 'applied' && (
          <span className="flex items-center gap-1 text-xs text-emerald-400 px-2 py-1.5">
            <Send size={12} /> Applied
          </span>
        )}
        {job.status !== 'applied' && (
          <button
            onClick={onDismiss}
            className="flex items-center gap-1 text-xs text-slate-400 hover:text-red-400 hover:bg-red-500/10 px-2 py-1.5 rounded-lg transition-colors ml-auto"
          >
            <Trash2 size={12} /> Dismiss
          </button>
        )}
      </div>
    </div>
  )
}

// ─── Main page ────────────────────────────────────────────────────────────────

const TABS: { key: TabKey; label: string }[] = [
  { key: 'new',     label: 'New' },
  { key: 'saved',   label: 'Saved' },
  { key: 'applied', label: 'Applied' },
]

const PAGE_SIZE = 20

export function JobsPage() {
  const [tab, setTab] = useState<TabKey>('new')
  const [jobs, setJobs] = useState<Job[]>([])
  const [loading, setLoading] = useState(true)
  const [search, setSearch] = useState('')
  const [companyFilter, setCompanyFilter] = useState('all')
  const [remoteOnly, setRemoteOnly] = useState(false)
  const [page, setPage] = useState(1)
  const [drawerJob, setDrawerJob] = useState<Job | null>(null)
  const [dismissTarget, setDismissTarget] = useState<Job | null>(null)
  const [toastMsg, setToastMsg] = useState('')

  const load = useCallback(async () => {
    setLoading(true)
    setPage(1)
    try {
      const data = await fetchJobs(tab)
      setJobs(data ?? [])
    } catch {
      setJobs([])
    } finally {
      setLoading(false)
    }
  }, [tab])

  useEffect(() => {
    load()
  }, [load])

  function toast(msg: string) {
    setToastMsg(msg)
    setTimeout(() => setToastMsg(''), 3000)
  }

  const companies = useMemo(() => {
    const names = Array.from(new Set(jobs.map(j => j.company?.name ?? j.company_id).filter(Boolean)))
    return names.sort()
  }, [jobs])

  const filtered = useMemo(() => {
    return jobs.filter(j => {
      const name = j.company?.name ?? j.company_id ?? ''
      if (search && !j.title.toLowerCase().includes(search.toLowerCase()) && !name.toLowerCase().includes(search.toLowerCase())) return false
      if (companyFilter !== 'all' && name !== companyFilter) return false
      if (remoteOnly && !j.remote) return false
      return true
    })
  }, [jobs, search, companyFilter, remoteOnly])

  const visible = filtered.slice(0, page * PAGE_SIZE)
  const hasMore = visible.length < filtered.length

  async function handleSave(job: Job) {
    const next = !job.saved
    setJobs(js => js.map(j => j.id === job.id ? { ...j, saved: next } : j))
    if (drawerJob?.id === job.id) setDrawerJob(d => d ? { ...d, saved: next } : d)
    try {
      await setJobSaved(job.id, next)
      if (next && tab === 'new') {
        await setJobStatus(job.id, 'saved')
        setJobs(js => js.map(j => j.id === job.id ? { ...j, status: 'saved' } : j))
      }
      toast(next ? 'Job saved' : 'Removed from saved')
    } catch {
      setJobs(js => js.map(j => j.id === job.id ? { ...j, saved: !next } : j))
      toast('Failed to update job')
    }
  }

  async function handleApply(job: Job) {
    setJobs(js => js.map(j => j.id === job.id ? { ...j, status: 'applied' } : j))
    if (drawerJob?.id === job.id) setDrawerJob(d => d ? { ...d, status: 'applied' } : d)
    try {
      await setJobStatus(job.id, 'applied')
      toast('Marked as applied')
    } catch {
      setJobs(js => js.map(j => j.id === job.id ? { ...j, status: job.status } : j))
      toast('Failed to update job')
    }
  }

  async function handleDismissConfirm(reason: string) {
    if (!dismissTarget) return
    const target = dismissTarget
    setDismissTarget(null)
    setJobs(js => js.filter(j => j.id !== target.id))
    if (drawerJob?.id === target.id) setDrawerJob(null)
    try {
      await dismissJob(target.id, reason)
      toast('Job dismissed')
    } catch {
      setJobs(js => [...js, target])
      toast('Failed to dismiss job')
    }
  }

  const tabCounts = useMemo(() => ({
    new: jobs.filter(j => j.status === 'new').length,
    saved: jobs.filter(j => j.saved).length,
    applied: jobs.filter(j => j.status === 'applied').length,
  }), [jobs])

  return (
    <div className="flex-1 flex flex-col min-h-0 relative">
      {/* Header */}
      <div className="flex items-center justify-between px-6 py-5 border-b border-white/[0.07]">
        <div>
          <h1 className="text-xl font-semibold text-white">Jobs</h1>
          <p className="text-sm text-slate-400 mt-0.5">
            {filtered.length} job{filtered.length !== 1 ? 's' : ''} · {tab} tab
          </p>
        </div>
        <button
          onClick={load}
          className="p-2 text-slate-400 hover:text-white bg-white/5 border border-white/10 rounded-lg transition-colors"
          title="Refresh"
        >
          <RefreshCw size={14} className={loading ? 'animate-spin' : ''} />
        </button>
      </div>

      {/* Tabs */}
      <div className="flex items-center gap-0.5 px-6 pt-3 pb-0 border-b border-white/[0.07]">
        {TABS.map(t => (
          <button
            key={t.key}
            onClick={() => setTab(t.key)}
            className={cn(
              'px-4 py-2 text-sm font-medium transition-colors rounded-t-lg border-b-2 -mb-px',
              tab === t.key
                ? 'text-violet-300 border-violet-500 bg-violet-500/5'
                : 'text-slate-400 border-transparent hover:text-white hover:bg-white/[0.03]'
            )}
          >
            {t.label}
            {tabCounts[t.key] > 0 && (
              <span className={cn(
                'ml-1.5 text-xs px-1.5 py-0.5 rounded-full',
                tab === t.key ? 'bg-violet-500/20 text-violet-300' : 'bg-white/[0.06] text-slate-500'
              )}>
                {tabCounts[t.key]}
              </span>
            )}
          </button>
        ))}
      </div>

      {/* Filters */}
      <div className="flex items-center gap-3 px-6 py-3 border-b border-white/[0.07] flex-wrap">
        <div className="relative flex-1 min-w-48 max-w-72">
          <Search size={14} className="absolute left-3 top-1/2 -translate-y-1/2 text-slate-400" />
          <input
            value={search}
            onChange={e => setSearch(e.target.value)}
            placeholder="Search jobs…"
            className="w-full bg-white/5 border border-white/10 rounded-lg pl-8 pr-3 py-2 text-sm text-white placeholder-slate-500 focus:outline-none focus:border-violet-500 transition-colors"
          />
        </div>

        <select
          value={companyFilter}
          onChange={e => setCompanyFilter(e.target.value)}
          className="bg-white/5 border border-white/10 rounded-lg px-3 py-2 text-sm text-slate-300 focus:outline-none focus:border-violet-500 transition-colors"
        >
          <option value="all">All companies</option>
          {companies.map(c => (
            <option key={c} value={c}>{c}</option>
          ))}
        </select>

        <button
          onClick={() => setRemoteOnly(v => !v)}
          className={cn(
            'flex items-center gap-1.5 px-3 py-2 text-sm rounded-lg border transition-colors',
            remoteOnly
              ? 'bg-emerald-500/10 border-emerald-500/30 text-emerald-300'
              : 'bg-white/5 border-white/10 text-slate-400 hover:text-white'
          )}
        >
          <Wifi size={13} />
          Remote only
        </button>
      </div>

      {/* Job list */}
      <div className="flex-1 overflow-auto px-6 py-4">
        {loading ? (
          <div className="flex items-center justify-center h-48">
            <div className="w-6 h-6 border-2 border-violet-500 border-t-transparent rounded-full animate-spin" />
          </div>
        ) : visible.length === 0 ? (
          <div className="text-center py-20 text-slate-500">
            {search || companyFilter !== 'all' || remoteOnly
              ? 'No jobs match your filters'
              : tab === 'new'
                ? 'No new jobs — run a scan to populate the feed'
                : tab === 'saved'
                  ? 'No saved jobs yet'
                  : 'No applied jobs yet'
            }
          </div>
        ) : (
          <div className="space-y-3">
            {visible.map(job => (
              <JobCard
                key={job.id}
                job={job}
                onClick={() => setDrawerJob(job)}
                onSave={() => handleSave(job)}
                onApply={() => handleApply(job)}
                onDismiss={() => setDismissTarget(job)}
              />
            ))}

            {hasMore && (
              <div className="flex justify-center pt-2 pb-4">
                <button
                  onClick={() => setPage(p => p + 1)}
                  className="px-5 py-2.5 text-sm text-slate-300 bg-white/5 border border-white/10 rounded-lg hover:bg-white/[0.08] hover:text-white transition-colors"
                >
                  Load more ({filtered.length - visible.length} remaining)
                </button>
              </div>
            )}
          </div>
        )}
      </div>

      {/* Job detail drawer */}
      <JobDetailDrawer
        job={drawerJob}
        onClose={() => setDrawerJob(null)}
        onSave={handleSave}
        onApply={handleApply}
        onDismiss={j => setDismissTarget(j)}
      />

      {/* Drawer backdrop */}
      {drawerJob && (
        <div
          className="fixed inset-0 z-20 bg-black/30"
          onClick={() => setDrawerJob(null)}
        />
      )}

      {/* Dismiss modal */}
      <DismissModal
        open={!!dismissTarget}
        jobTitle={dismissTarget?.title ?? ''}
        onConfirm={handleDismissConfirm}
        onCancel={() => setDismissTarget(null)}
      />

      {/* Toast */}
      {toastMsg && (
        <div className="fixed bottom-6 left-1/2 -translate-x-1/2 bg-[#1e1e2a] border border-white/10 text-sm text-white px-4 py-2.5 rounded-lg shadow-xl z-50">
          {toastMsg}
        </div>
      )}
    </div>
  )
}
