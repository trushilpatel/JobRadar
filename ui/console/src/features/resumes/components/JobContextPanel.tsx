import { ExternalLink, MapPin, Briefcase, DollarSign } from 'lucide-react'
import type { ApplyQueueItem } from '@/features/resumes/types'

interface Props {
  job: ApplyQueueItem['job']
}

// Right pane shown next to the resume editor when ?job=:jobId is present. It
// is intentionally read-only — encourages user to tailor the resume against
// the JD without context-switching to another tab.
export function JobContextPanel({ job }: Props) {
  return (
    <div className="rounded-xl border border-white/[0.06] bg-[#141420] flex flex-col min-h-0">
      <div className="px-4 py-3 border-b border-white/[0.05]">
        <div className="flex items-start justify-between gap-3">
          <div className="min-w-0">
            <div className="text-[11px] text-slate-500 uppercase tracking-wide mb-1">
              Editing against
            </div>
            <h2 className="text-base font-semibold text-white truncate">{job.title}</h2>
            <div className="text-sm text-slate-400 mt-0.5">{job.company}</div>
          </div>
          <a
            href={job.url}
            target="_blank"
            rel="noreferrer"
            className="text-violet-300 hover:text-violet-200 shrink-0 mt-0.5"
            title="Open job posting"
          >
            <ExternalLink size={14} />
          </a>
        </div>

        <div className="flex flex-wrap gap-x-4 gap-y-1 mt-3 text-xs text-slate-400">
          <span className="inline-flex items-center gap-1">
            <MapPin size={11} />
            {job.location}
          </span>
          {job.salary && (
            <span className="inline-flex items-center gap-1">
              <DollarSign size={11} />
              {job.salary}
            </span>
          )}
          <span className="inline-flex items-center gap-1">
            <Briefcase size={11} />
            Saved
          </span>
        </div>
      </div>

      <div className="flex-1 overflow-y-auto p-4 space-y-4 text-sm">
        <Section title="Responsibilities">
          <ul className="list-disc list-inside space-y-1 text-slate-300">
            <li>Design and operate distributed payment systems at scale</li>
            <li>Own services end-to-end: design, build, deploy, on-call</li>
            <li>Drive technical decisions across multiple teams</li>
            <li>Mentor senior engineers and influence team direction</li>
          </ul>
        </Section>
        <Section title="Requirements">
          <ul className="list-disc list-inside space-y-1 text-slate-300">
            <li>8+ years building backend systems in Go (preferred) or similar</li>
            <li>Experience with Postgres, Kafka, distributed transactions</li>
            <li>Strong written communication; comfortable in async-first teams</li>
            <li>Track record of shipping high-availability systems</li>
          </ul>
        </Section>
        <Section title="Tech stack">
          <div className="flex flex-wrap gap-1.5">
            {['Go', 'Postgres', 'Kafka', 'Kubernetes', 'gRPC', 'OpenTelemetry'].map((t) => (
              <span
                key={t}
                className="text-[11px] px-2 py-0.5 rounded bg-emerald-500/10 text-emerald-300"
              >
                {t}
              </span>
            ))}
          </div>
        </Section>
        <div className="text-[10px] text-slate-500 italic pt-3 border-t border-white/[0.05]">
          Mocked JD content — would be sourced from <code>jobs.responsibilities</code> and{' '}
          <code>jobs.role_brief</code> populated by the analyzer.
        </div>
      </div>
    </div>
  )
}

function Section({ title, children }: { title: string; children: React.ReactNode }) {
  return (
    <div>
      <div className="text-[11px] text-slate-500 uppercase tracking-wide mb-1.5">{title}</div>
      {children}
    </div>
  )
}
