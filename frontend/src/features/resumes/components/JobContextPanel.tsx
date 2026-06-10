import { ExternalLink, MapPin, Briefcase, DollarSign } from 'lucide-react'
import type { ApplyQueueItem } from '../types'

interface Props {
  job: ApplyQueueItem['job']
}

// Right pane shown next to the resume editor when ?job=:jobId is present. It
// is intentionally read-only — encourages user to tailor the resume against
// the JD without context-switching to another tab.
export function JobContextPanel({ job }: Props) {
  const hasEnrichedContent =
    (job.responsibilities && job.responsibilities.length > 0) ||
    (job.requirements && job.requirements.length > 0) ||
    (job.tech_stack && job.tech_stack.length > 0) ||
    !!job.role_brief

  return (
    <div className="rounded-xl border border-[var(--border)] bg-panel flex flex-col min-h-0">
      <div className="px-4 py-3 border-b border-[var(--border)]">
        <div className="flex items-start justify-between gap-3">
          <div className="min-w-0">
            <div className="text-[11px] text-muted uppercase tracking-wide mb-1">
              Editing against
            </div>
            <h2 className="text-base font-semibold text-primary truncate">{job.title}</h2>
            <div className="text-sm text-muted mt-0.5">{job.company}</div>
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

        <div className="flex flex-wrap gap-x-4 gap-y-1 mt-3 text-xs text-muted">
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

      <div className="flex-1 overflow-y-auto p-4 text-sm">
        {hasEnrichedContent ? (
          <div className="space-y-4">
            {job.role_brief && (
              <Section title="Overview">
                <p className="text-secondary leading-relaxed">{job.role_brief}</p>
              </Section>
            )}
            {job.responsibilities && job.responsibilities.length > 0 && (
              <Section title="Responsibilities">
                <ul className="list-disc list-inside space-y-1 text-secondary">
                  {job.responsibilities.map((item, i) => (
                    <li key={i}>{item}</li>
                  ))}
                </ul>
              </Section>
            )}
            {job.requirements && job.requirements.length > 0 && (
              <Section title="Requirements">
                <ul className="list-disc list-inside space-y-1 text-secondary">
                  {job.requirements.map((item, i) => (
                    <li key={i}>{item}</li>
                  ))}
                </ul>
              </Section>
            )}
            {job.tech_stack && job.tech_stack.length > 0 && (
              <Section title="Tech stack">
                <div className="flex flex-wrap gap-1.5">
                  {job.tech_stack.map((t) => (
                    <span
                      key={t}
                      className="text-[11px] px-2 py-0.5 rounded bg-emerald-500/10 text-emerald-300"
                    >
                      {t}
                    </span>
                  ))}
                </div>
              </Section>
            )}
          </div>
        ) : (
          <div className="flex flex-col items-center justify-center h-full text-center py-12 px-6">
            <p className="text-sm text-muted">Job description not yet enriched.</p>
            <p className="text-xs mt-1.5 text-placeholder max-w-xs">
              The analyzer will populate responsibilities and requirements once this job is
              processed.
            </p>
          </div>
        )}
      </div>
    </div>
  )
}

function Section({ title, children }: { title: string; children: React.ReactNode }) {
  return (
    <div>
      <div className="text-[11px] text-muted uppercase tracking-wide mb-1.5">{title}</div>
      {children}
    </div>
  )
}
