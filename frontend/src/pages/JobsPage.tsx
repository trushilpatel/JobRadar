import { useEffect, useState, useMemo, useCallback, useRef } from 'react'
import * as Dialog from '@radix-ui/react-dialog'
import {
  X, ExternalLink, MapPin, Wifi, Briefcase,
  RefreshCw, DollarSign, Bookmark, BookmarkCheck, Send, Trash2,
  ChevronDown, Check,
} from 'lucide-react'
import type { Job, SalaryRange } from '@/types'
import {
  fetchJobs, setJobSaved, setJobStatus, dismissJob,
} from '@/api/jobs'
import { formatRelativeTime, cn } from '@/lib/utils'

// ─── Dismiss reasons ──────────────────────────────────────────────────────────
const DISMISS_REASON_OPTIONS = [
  { value: 'user_dismissed', label: 'Not relevant' },
  { value: 'user_not_relevant', label: 'Too junior / senior' },
  { value: 'user_not_relevant', label: 'Bad location' },
  { value: 'user_not_relevant', label: 'Compensation too low' },
  { value: 'user_dismissed', label: 'Company not a fit' },
  { value: 'user_dismissed', label: 'Other' },
]

type TabKey = 'new' | 'saved' | 'applied'

const EMPLOY_TYPE_OPTIONS = [
  { value: 'full_time', label: 'Full-time' },
  // { value: 'part_time',  label: 'Part-time' },
  // { value: 'contract',   label: 'Contract' },
  // { value: 'internship', label: 'Internship' },
]

const TABS: { key: TabKey; label: string }[] = [
  { key: 'new', label: 'New' },
  { key: 'saved', label: 'Saved' },
  { key: 'applied', label: 'Applied' },
]

// ─── Helpers ──────────────────────────────────────────────────────────────────

function formatSalary(salary: SalaryRange | null | undefined): string | null {
  if (!salary) return null
  const currency = salary.currency ?? '$'
  const fmt = (n: number) => n >= 1000 ? `${currency}${Math.round(n / 1000)}k` : `${currency}${n}`
  if (salary.min && salary.max) return `${fmt(salary.min)} – ${fmt(salary.max)}`
  if (salary.min) return `${fmt(salary.min)}+`
  if (salary.max) return `up to ${fmt(salary.max)}`
  return null
}

const AVATAR_COLORS = [
  'bg-violet-600', 'bg-blue-600', 'bg-emerald-600',
  'bg-amber-600', 'bg-rose-600', 'bg-cyan-600',
  'bg-indigo-600', 'bg-teal-600',
]
function avatarColor(name: string): string {
  let hash = 0
  for (let i = 0; i < name.length; i++) hash = name.charCodeAt(i) + ((hash << 5) - hash)
  return AVATAR_COLORS[Math.abs(hash) % AVATAR_COLORS.length]
}

function CompanyAvatar({ name }: { name: string }) {
  return (
    <div className={cn('w-7 h-7 rounded-md flex items-center justify-center text-xs font-semibold text-white shrink-0', avatarColor(name))}>
      {name ? name[0].toUpperCase() : '?'}
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
        <Dialog.Overlay className="fixed inset-0 bg-black/60 z-50" />
        <Dialog.Content className="fixed left-1/2 top-1/2 -translate-x-1/2 -translate-y-1/2 z-50 w-full max-w-sm bg-[#13131e] border border-white/[0.07] rounded-xl p-5 shadow-2xl">
          <Dialog.Title className="text-sm font-medium text-white mb-1">Dismiss job</Dialog.Title>
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
            <button onClick={onCancel} className="flex-1 px-4 py-2 text-sm text-slate-400 bg-white/5 border border-white/10 rounded-lg hover:bg-white/[0.08] transition-colors">
              Cancel
            </button>
            <button onClick={() => onConfirm(DISMISS_REASON_OPTIONS[selectedIdx].value)} className="flex-1 px-4 py-2 text-sm font-medium text-white bg-red-600/80 hover:bg-red-600 rounded-lg transition-colors">
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
    <div className="flex flex-col h-full bg-[#0d0d14]">
      {/* Header: title + company + actions */}
      <div className="px-5 pt-4 pb-3 border-b border-white/[0.07]">
        <div className="flex items-start justify-between gap-3 mb-3">
          <div className="flex items-start gap-3 min-w-0">
            <CompanyAvatar name={companyName} />
            <div className="min-w-0">
              <p className="text-base font-semibold text-white leading-tight">{job.title}</p>
              <p className="text-sm text-slate-400 mt-0.5">{companyName}</p>
            </div>
          </div>
          <button
            onClick={onClose}
            className="shrink-0 p-1.5 text-slate-400 hover:text-white hover:bg-white/[0.06] rounded-lg transition-colors"
          >
            <X size={15} />
          </button>
        </div>

        {/* Actions row */}
        <div className="flex items-center gap-2">
          <a
            href={job.url}
            target="_blank"
            rel="noreferrer"
            className="flex items-center gap-1.5 text-xs text-slate-300 hover:text-violet-300 bg-white/5 border border-white/10 px-2.5 py-1.5 rounded-lg transition-colors"
          >
            <ExternalLink size={12} /> Open posting
          </a>
          {!job.saved && job.status !== 'applied' && (
            <button
              onClick={() => onSave(job)}
              className="flex items-center gap-1.5 text-xs text-white bg-violet-600 hover:bg-violet-500 px-2.5 py-1.5 rounded-lg transition-colors"
            >
              <Bookmark size={12} /> Save
            </button>
          )}
          {job.saved && job.status !== 'applied' && (
            <button
              onClick={() => onSave(job)}
              className="flex items-center gap-1.5 text-xs text-violet-300 bg-violet-500/10 border border-violet-500/20 hover:bg-violet-500/20 px-2.5 py-1.5 rounded-lg transition-colors"
            >
              <BookmarkCheck size={12} /> Saved
            </button>
          )}
          {job.status !== 'applied' && (
            <button
              onClick={() => onApply(job)}
              className="flex items-center gap-1.5 text-xs text-white bg-emerald-600 hover:bg-emerald-500 px-2.5 py-1.5 rounded-lg transition-colors"
            >
              <Send size={12} /> Mark applied
            </button>
          )}
          {job.status !== 'applied' && (
            <button
              onClick={() => onDismiss(job)}
              className="ml-auto p-1.5 text-slate-400 hover:text-red-400 hover:bg-red-500/10 rounded-lg transition-colors"
              title="Dismiss"
            >
              <Trash2 size={13} />
            </button>
          )}
        </div>
      </div>

      {/* Body */}
      <div className="flex-1 overflow-y-auto px-5 py-4 space-y-5">
        {/* Meta badges */}
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
          {job.experience_years != null && (
            <span className="text-xs text-slate-400 bg-white/[0.05] border border-white/[0.07] px-2.5 py-1 rounded-lg">
              {job.experience_years}+ yrs exp
            </span>
          )}
        </div>

        {/* Flag indicators */}
        <div className="flex flex-wrap gap-2">
          <span className={cn(
            'text-xs px-2.5 py-1 rounded-lg border',
            job.sponsorship
              ? 'text-emerald-400 bg-emerald-500/10 border-emerald-500/20'
              : 'text-slate-500 bg-white/[0.03] border-white/[0.06]'
          )}>
            {job.sponsorship ? '✓ Sponsorship' : '✗ No sponsorship'}
          </span>
          {job.requires_citizen && (
            <span className="text-xs text-amber-400 bg-amber-500/10 border border-amber-500/20 px-2.5 py-1 rounded-lg">
              Citizenship required
            </span>
          )}
          {job.security_clearance && (
            <span className="text-xs text-red-400 bg-red-500/10 border border-red-500/20 px-2.5 py-1 rounded-lg">
              Security clearance required
            </span>
          )}
        </div>

        <p className="text-xs text-slate-500">Added {formatRelativeTime(job.added_at)}</p>

        {/* Summary */}
        {(job.role_brief || job.llm_summary) && (
          <div>
            <h3 className="text-xs font-medium text-slate-400 uppercase tracking-wider mb-2">Summary</h3>
            <p className="text-sm text-slate-300 leading-relaxed">{job.role_brief ?? job.llm_summary}</p>
          </div>
        )}

        {/* Responsibilities */}
        {job.responsibilities && (
          <div>
            <h3 className="text-xs font-medium text-slate-400 uppercase tracking-wider mb-2">Responsibilities</h3>
            <p className="text-sm text-slate-300 leading-relaxed whitespace-pre-line">{job.responsibilities}</p>
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
    </div>
  )
}

// ─── Company multi-select ─────────────────────────────────────────────────────

function CompanyMultiSelect({ companies, selected, onChange }: {
  companies: string[]
  selected: string[]
  onChange: (v: string[]) => void
}) {
  const [open, setOpen] = useState(false)
  const [search, setSearch] = useState('')
  const ref = useRef<HTMLDivElement>(null)

  useEffect(() => {
    function onMouseDown(e: MouseEvent) {
      if (ref.current && !ref.current.contains(e.target as Node)) setOpen(false)
    }
    document.addEventListener('mousedown', onMouseDown)
    return () => document.removeEventListener('mousedown', onMouseDown)
  }, [])

  const visible = companies.filter(c => c.toLowerCase().includes(search.toLowerCase()))

  function toggle(c: string) {
    onChange(selected.includes(c) ? selected.filter(v => v !== c) : [...selected, c])
  }

  const label = selected.length === 0
    ? 'All companies'
    : selected.length === 1
      ? selected[0]
      : `${selected.length} companies`

  return (
    <div className="relative" ref={ref}>
      <button
        onClick={() => setOpen(v => !v)}
        className={cn(
          'flex items-center gap-1.5 px-3 py-1.5 text-xs rounded-lg border transition-colors min-w-[120px]',
          selected.length > 0
            ? 'bg-violet-500/15 border-violet-500/30 text-violet-300'
            : 'bg-white/5 border-white/10 text-slate-300 hover:bg-white/[0.08]'
        )}
      >
        <span className="flex-1 text-left truncate max-w-[140px]">{label}</span>
        <ChevronDown size={11} className="shrink-0 text-slate-500" />
      </button>

      {open && (
        <div className="absolute top-full left-0 mt-1 w-56 bg-[#13131e] border border-white/[0.1] rounded-lg shadow-xl z-20 overflow-hidden">
          <div className="p-2 border-b border-white/[0.07]">
            <input
              autoFocus
              value={search}
              onChange={e => setSearch(e.target.value)}
              placeholder="Search…"
              className="w-full bg-white/5 border border-white/10 rounded-md px-2.5 py-1.5 text-xs text-white placeholder-slate-500 focus:outline-none focus:border-violet-500"
            />
          </div>
          <div className="max-h-52 overflow-y-auto py-1">
            {visible.length === 0 ? (
              <p className="text-xs text-slate-500 px-3 py-2">No results</p>
            ) : visible.map(c => (
              <button
                key={c}
                onClick={() => toggle(c)}
                className={cn(
                  'w-full text-left px-3 py-2 text-xs transition-colors flex items-center gap-2',
                  selected.includes(c) ? 'text-violet-300 bg-violet-500/10' : 'text-slate-300 hover:bg-white/[0.05]'
                )}
              >
                <div className={cn(
                  'w-3.5 h-3.5 rounded border flex items-center justify-center shrink-0',
                  selected.includes(c) ? 'bg-violet-500 border-violet-500' : 'border-white/20'
                )}>
                  {selected.includes(c) && <Check size={9} className="text-white" />}
                </div>
                {c}
              </button>
            ))}
          </div>
          {selected.length > 0 && (
            <div className="border-t border-white/[0.07] p-2">
              <button
                onClick={() => { onChange([]); setSearch('') }}
                className="w-full text-xs text-slate-500 hover:text-slate-300 transition-colors py-1"
              >
                Clear selection
              </button>
            </div>
          )}
        </div>
      )}
    </div>
  )
}

// ─── Main page ────────────────────────────────────────────────────────────────

export function JobsPage() {
  const [tab, setTab] = useState<TabKey>('new')
  const [jobs, setJobs] = useState<Job[]>([])
  const [loading, setLoading] = useState(true)
  const [drawerJob, setDrawerJob] = useState<Job | null>(null)
  const [dismissTarget, setDismissTarget] = useState<Job | null>(null)
  const [selectedIds, setSelectedIds] = useState<Set<string>>(new Set())
  const [panelWidth, setPanelWidth] = useState(440)
  const [toastMsg, setToastMsg] = useState('')

  function startDrag(e: React.MouseEvent) {
    e.preventDefault()
    const startX = e.clientX
    const startWidth = panelWidth
    function onMove(e: MouseEvent) {
      setPanelWidth(Math.min(Math.max(startWidth + (startX - e.clientX), 300), 720))
    }
    function onUp() {
      document.removeEventListener('mousemove', onMove)
      document.removeEventListener('mouseup', onUp)
    }
    document.addEventListener('mousemove', onMove)
    document.addEventListener('mouseup', onUp)
  }

  // Filters — defaults per spec
  const [selectedCompanies, setSelectedCompanies] = useState<string[]>([])
  const [remoteFilter, setRemoteFilter] = useState(false)
  const [employTypes, setEmployTypes] = useState<string[]>(['full_time'])
  const [sponsorshipFilter, setSponsorshipFilter] = useState(true)
  const [noRequirementsFilter, setNoRequirementsFilter] = useState(true)
  const [expFilter, setExpFilter] = useState<string[]>(['0-2', '3-5'])
  const [dateFilter, setDateFilter] = useState<'all' | '7d' | '30d'>('all')

  const load = useCallback(async () => {
    setLoading(true)
    setSelectedIds(new Set())
    try {
      const data = await fetchJobs(tab)
      setJobs(data ?? [])
    } catch {
      setJobs([])
    } finally {
      setLoading(false)
    }
  }, [tab])

  useEffect(() => { load() }, [load])

  function toast(msg: string) {
    setToastMsg(msg)
    setTimeout(() => setToastMsg(''), 3000)
  }

  const companies = useMemo(() => {
    const names = Array.from(new Set(jobs.map(j => j.company?.name ?? j.company_id).filter(Boolean)))
    return names.sort()
  }, [jobs])

  const filtered = useMemo(() => {
    const now = Date.now()
    const cutoff = dateFilter === '7d'
      ? now - 7 * 24 * 60 * 60 * 1000
      : dateFilter === '30d'
        ? now - 30 * 24 * 60 * 60 * 1000
        : null

    return jobs.filter(j => {
      const name = j.company?.name ?? j.company_id ?? ''
      if (selectedCompanies.length > 0 && !selectedCompanies.includes(name)) return false
      if (remoteFilter && !j.remote) return false
      if (employTypes.length > 0 && j.employ_type && !employTypes.includes(j.employ_type)) return false
      if (sponsorshipFilter && !j.sponsorship) return false
      if (noRequirementsFilter && (j.requires_citizen || j.security_clearance)) return false
      if (expFilter.length > 0 && j.experience_years != null) {
        const y = j.experience_years
        const inRange = expFilter.some(r =>
          (r === '0-2' && y <= 2) ||
          (r === '3-5' && y >= 3 && y <= 5) ||
          (r === '6-10' && y >= 6 && y <= 10) ||
          (r === '10+' && y >= 10)
        )
        if (!inRange) return false
      }
      if (cutoff && new Date(j.added_at).getTime() < cutoff) return false
      return true
    })
  }, [jobs, selectedCompanies, remoteFilter, employTypes, sponsorshipFilter, noRequirementsFilter, expFilter, dateFilter])

  function toggleEmployType(val: string) {
    setEmployTypes(prev => prev.includes(val) ? prev.filter(v => v !== val) : [...prev, val])
  }

  const tabCounts = useMemo(() => ({
    new: jobs.filter(j => j.status === 'new').length,
    saved: jobs.filter(j => j.saved).length,
    applied: jobs.filter(j => j.status === 'applied').length,
  }), [jobs])

  async function handleSave(job: Job) {
    const next = !job.saved
    const nextStatus = next ? 'saved' : 'new'
    setJobs(js => js.map(j => j.id === job.id ? { ...j, saved: next, status: nextStatus } : j))
    if (drawerJob?.id === job.id) setDrawerJob(d => d ? { ...d, saved: next, status: nextStatus } : d)
    try {
      await setJobSaved(job.id, next)
      toast(next ? 'Job saved' : 'Removed from saved')
    } catch {
      setJobs(js => js.map(j => j.id === job.id ? { ...j, saved: job.saved, status: job.status } : j))
      if (drawerJob?.id === job.id) setDrawerJob(d => d ? { ...d, saved: job.saved, status: job.status } : d)
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

  async function handleBulkDismiss() {
    const ids = Array.from(selectedIds)
    const targets = jobs.filter(j => ids.includes(j.id))
    setSelectedIds(new Set())
    setJobs(js => js.filter(j => !ids.includes(j.id)))
    if (drawerJob && ids.includes(drawerJob.id)) setDrawerJob(null)
    try {
      await Promise.all(ids.map(id => dismissJob(id, 'user_not_relevant')))
      toast(`${ids.length} job${ids.length !== 1 ? 's' : ''} dismissed`)
    } catch {
      setJobs(js => [...js, ...targets])
      toast('Failed to dismiss some jobs')
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

  return (
    <div className="flex-1 flex flex-col min-h-0">
      {/* Header */}
      <div className="flex items-center justify-between px-6 py-4 border-b border-white/[0.07]">
        <div>
          <h1 className="text-xl font-semibold text-white">Jobs</h1>
          <p className="text-sm text-slate-400 mt-0.5">{filtered.length} job{filtered.length !== 1 ? 's' : ''}</p>
        </div>
        <button
          onClick={load}
          className="p-2 text-slate-400 hover:text-white bg-white/5 border border-white/10 rounded-lg transition-colors"
          title="Refresh"
        >
          <RefreshCw size={14} className={loading ? 'animate-spin' : ''} />
        </button>
      </div>

      {/* Status tabs */}
      <div className="flex items-center gap-0.5 px-6 pt-3 border-b border-white/[0.07]">
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

      {/* Filter bar */}
      <div className="flex items-center gap-2 px-6 py-2.5 border-b border-white/[0.07] flex-wrap">
        {/* Company multi-select */}
        <CompanyMultiSelect
          companies={companies}
          selected={selectedCompanies}
          onChange={setSelectedCompanies}
        />

        <div className="w-px h-4 bg-white/[0.08] mx-0.5" />

        {/* Experience years */}
        {(['0-2', '3-5'] as const).map(opt => (
          <button
            key={opt}
            onClick={() => setExpFilter(prev =>
              prev.includes(opt) ? prev.filter(v => v !== opt) : [...prev, opt]
            )}
            className={cn(
              'px-2.5 py-1.5 text-xs rounded-lg border transition-colors',
              expFilter.includes(opt)
                ? 'bg-violet-500/15 border-violet-500/30 text-violet-300'
                : 'bg-white/[0.03] border-white/[0.07] text-slate-400 hover:text-slate-200 hover:bg-white/[0.06]'
            )}
          >
            {opt} yrs
          </button>
        ))}

        <div className="w-px h-4 bg-white/[0.08] mx-0.5" />

        {/* Date added */}
        {(['all', '30d', '7d'] as const).map(d => (
          <button
            key={d}
            onClick={() => setDateFilter(d)}
            className={cn(
              'px-2.5 py-1.5 text-xs rounded-lg border transition-colors',
              dateFilter === d
                ? 'bg-violet-500/15 border-violet-500/30 text-violet-300'
                : 'bg-white/[0.03] border-white/[0.07] text-slate-400 hover:text-slate-200 hover:bg-white/[0.06]'
            )}
          >
            {d === 'all' ? 'All' : d}
          </button>
        ))}

        <div className="w-px h-4 bg-white/[0.08] mx-0.5" />

        {/* Employment type pills */}
        {EMPLOY_TYPE_OPTIONS.map(opt => (
          <button
            key={opt.value}
            onClick={() => toggleEmployType(opt.value)}
            className={cn(
              'px-2.5 py-1.5 text-xs rounded-lg border transition-colors',
              employTypes.includes(opt.value)
                ? 'bg-violet-500/15 border-violet-500/30 text-violet-300'
                : 'bg-white/[0.03] border-white/[0.07] text-slate-400 hover:text-slate-200 hover:bg-white/[0.06]'
            )}
          >
            {opt.label}
          </button>
        ))}

        <div className="w-px h-4 bg-white/[0.08] mx-0.5" />

        {/* Remote */}
        <button
          onClick={() => setRemoteFilter(v => !v)}
          className={cn(
            'px-2.5 py-1.5 text-xs rounded-lg border transition-colors',
            remoteFilter
              ? 'bg-emerald-500/15 border-emerald-500/30 text-emerald-300'
              : 'bg-white/[0.03] border-white/[0.07] text-slate-400 hover:text-slate-200 hover:bg-white/[0.06]'
          )}
        >
          Remote
        </button>

        {/* Sponsorship */}
        <button
          onClick={() => setSponsorshipFilter(v => !v)}
          className={cn(
            'px-2.5 py-1.5 text-xs rounded-lg border transition-colors',
            sponsorshipFilter
              ? 'bg-violet-500/15 border-violet-500/30 text-violet-300'
              : 'bg-white/[0.03] border-white/[0.07] text-slate-400 hover:text-slate-200 hover:bg-white/[0.06]'
          )}
        >
          Sponsorship
        </button>

        {/* No citizenship / clearance required (combined) */}
        <button
          onClick={() => setNoRequirementsFilter(v => !v)}
          className={cn(
            'px-2.5 py-1.5 text-xs rounded-lg border transition-colors',
            noRequirementsFilter
              ? 'bg-violet-500/15 border-violet-500/30 text-violet-300'
              : 'bg-white/[0.03] border-white/[0.07] text-slate-400 hover:text-slate-200 hover:bg-white/[0.06]'
          )}
        >
          No Citizenship Req.
        </button>

      </div>


      {/* Bulk action bar */}
      {selectedIds.size > 0 && (
        <div className="flex items-center gap-3 px-6 py-2 bg-violet-500/10 border-b border-violet-500/20">
          <span className="text-xs text-violet-300 font-medium">
            {selectedIds.size} selected
          </span>
          <button
            onClick={handleBulkDismiss}
            className="flex items-center gap-1.5 text-xs text-white bg-red-600/80 hover:bg-red-600 px-3 py-1.5 rounded-lg transition-colors"
          >
            <Trash2 size={12} /> Mark not relevant
          </button>
          <button
            onClick={() => setSelectedIds(new Set())}
            className="text-xs text-slate-400 hover:text-slate-200 transition-colors ml-auto"
          >
            Clear
          </button>
        </div>
      )}

      {/* Content: table + optional detail panel */}
      <div className="flex-1 flex min-h-0 overflow-hidden">
      <div className="flex-1 overflow-auto min-w-0">
        {loading ? (
          <div className="flex items-center justify-center h-48">
            <div className="w-6 h-6 border-2 border-violet-500 border-t-transparent rounded-full animate-spin" />
          </div>
        ) : filtered.length === 0 ? (
          <div className="text-center py-20 text-slate-500 text-sm">
            {tab === 'new'
              ? 'No new jobs — run a scan to populate the feed'
              : tab === 'saved'
                ? 'No saved jobs yet'
                : 'No applied jobs yet'}
          </div>
        ) : (
          <table className="w-full min-w-[680px] text-sm border-collapse">
            <thead className="sticky top-0 bg-[#0a0a0f] z-10">
              <tr className="border-b border-white/[0.07]">
                <th className="px-4 py-2.5 w-8">
                  <input
                    type="checkbox"
                    checked={filtered.length > 0 && filtered.every(j => selectedIds.has(j.id))}
                    ref={el => { if (el) el.indeterminate = selectedIds.size > 0 && !filtered.every(j => selectedIds.has(j.id)) }}
                    onChange={e => {
                      if (e.target.checked) setSelectedIds(new Set(filtered.map(j => j.id)))
                      else setSelectedIds(new Set())
                    }}
                    className="accent-violet-500 cursor-pointer"
                  />
                </th>
                <th className="text-left text-xs font-medium text-slate-500 uppercase tracking-wide px-4 py-2.5 w-44">Company</th>
                <th className="text-left text-xs font-medium text-slate-500 uppercase tracking-wide px-4 py-2.5">Title</th>
                <th className="text-left text-xs font-medium text-slate-500 uppercase tracking-wide px-4 py-2.5 w-44">Location</th>
                <th className="text-left text-xs font-medium text-slate-500 uppercase tracking-wide px-4 py-2.5 w-36">Salary</th>
                <th className="text-left text-xs font-medium text-slate-500 uppercase tracking-wide px-4 py-2.5 w-24">Added</th>
              </tr>
            </thead>
            <tbody>
              {filtered.map(job => {
                const companyName = job.company?.name ?? job.company_id
                const salary = formatSalary(job.salary_range)
                const isActive = drawerJob?.id === job.id
                const isSelected = selectedIds.has(job.id)

                return (
                  <tr
                    key={job.id}
                    onClick={() => setDrawerJob(job)}
                    className={cn(
                      'border-b border-white/[0.04] cursor-pointer transition-colors',
                      isSelected ? 'bg-violet-500/[0.06]' : isActive ? 'bg-violet-500/[0.08]' : 'hover:bg-white/[0.03]'
                    )}
                  >
                    <td className="px-4 py-3" onClick={e => e.stopPropagation()}>
                      <input
                        type="checkbox"
                        checked={isSelected}
                        onChange={e => {
                          const next = new Set(selectedIds)
                          e.target.checked ? next.add(job.id) : next.delete(job.id)
                          setSelectedIds(next)
                        }}
                        className="accent-violet-500 cursor-pointer"
                      />
                    </td>
                    <td className="px-4 py-3">
                      <span className="text-xs text-slate-300">{companyName}</span>
                    </td>
                    <td className="px-4 py-3">
                      <span className={cn('font-medium', isActive ? 'text-violet-200' : 'text-white')}>
                        {job.title}
                      </span>
                    </td>
                    <td className="px-4 py-3">
                      {job.remote ? (
                        <span className="inline-flex items-center gap-1 text-xs text-emerald-400 bg-emerald-500/10 border border-emerald-500/20 px-2 py-0.5 rounded-md">
                          <Wifi size={10} /> Remote
                        </span>
                      ) : job.location ? (
                        <span className="inline-flex items-center gap-1 text-xs text-slate-400">
                          <MapPin size={10} /> {job.location}
                        </span>
                      ) : (
                        <span className="text-xs text-slate-600">—</span>
                      )}
                    </td>
                    <td className="px-4 py-3">
                      {salary
                        ? <span className="text-xs text-amber-300">{salary}</span>
                        : <span className="text-xs text-slate-600">—</span>
                      }
                    </td>
                    <td className="px-4 py-3">
                      <span className="text-xs text-slate-500">{formatRelativeTime(job.added_at)}</span>
                    </td>
                  </tr>
                )
              })}
            </tbody>
          </table>
        )}
      </div>

        {/* Drag handle + detail panel */}
        {drawerJob && (
          <>
            <div
              onMouseDown={startDrag}
              className="w-1 shrink-0 cursor-col-resize bg-white/[0.06] hover:bg-violet-500/50 transition-colors"
            />
            <div style={{ width: panelWidth }} className="shrink-0 flex flex-col min-h-0 border-l border-white/[0.07] overflow-hidden">
              <JobDetailDrawer
                job={drawerJob}
                onClose={() => setDrawerJob(null)}
                onSave={handleSave}
                onApply={handleApply}
                onDismiss={j => setDismissTarget(j)}
              />
            </div>
          </>
        )}
      </div>

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
