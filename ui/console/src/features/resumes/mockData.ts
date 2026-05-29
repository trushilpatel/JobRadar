// Stub data for the resumes mockup. Will be deleted once the
// `/admin/resumes/*` endpoints are wired and useResumes/useResume call the
// real API.

import type {
  Resume,
  ResumeVersion,
  ResumeJobAttachment,
  ApplyQueueItem,
  JobLifecycleStatus,
} from './types'

const USER_ID = 'admin-user-1'

const now = Date.now()
const dayAgo = (d: number) => new Date(now - d * 86400_000).toISOString()

export const MOCK_RESUMES: Resume[] = [
  {
    id: 'res-1',
    user_id: USER_ID,
    name: 'Backend Senior — Go heavy',
    label: 'backend-sr-go',
    tags: [
      { axis: 'role', value: 'backend' },
      { axis: 'seniority', value: 'senior' },
      { axis: 'stack', value: 'go' },
    ],
    active_version_id: 'ver-1-4',
    version_count: 4,
    attached_job_count: 6,
    submitted_count: 2,
    created_at: dayAgo(40),
    updated_at: dayAgo(2),
    archived_at: null,
  },
  {
    id: 'res-2',
    user_id: USER_ID,
    name: 'Platform / Infra Staff',
    label: 'platform-staff',
    tags: [
      { axis: 'role', value: 'platform' },
      { axis: 'seniority', value: 'staff' },
      { axis: 'stack', value: 'go' },
      { axis: 'stack', value: 'kubernetes' },
    ],
    active_version_id: 'ver-2-3',
    version_count: 3,
    attached_job_count: 4,
    submitted_count: 1,
    created_at: dayAgo(35),
    updated_at: dayAgo(5),
    archived_at: null,
  },
  {
    id: 'res-3',
    user_id: USER_ID,
    name: 'Fullstack — TS heavy',
    label: 'fullstack-ts',
    tags: [
      { axis: 'role', value: 'fullstack' },
      { axis: 'seniority', value: 'senior' },
      { axis: 'stack', value: 'typescript' },
      { axis: 'stack', value: 'react' },
    ],
    active_version_id: 'ver-3-2',
    version_count: 2,
    attached_job_count: 3,
    submitted_count: 0,
    created_at: dayAgo(20),
    updated_at: dayAgo(8),
    archived_at: null,
  },
  {
    id: 'res-4',
    user_id: USER_ID,
    name: 'Engineering Manager — IC↔Mgmt',
    label: 'em-pivot',
    tags: [
      { axis: 'role', value: 'mgmt' },
      { axis: 'seniority', value: 'lead' },
    ],
    active_version_id: 'ver-4-2',
    version_count: 2,
    attached_job_count: 1,
    submitted_count: 0,
    created_at: dayAgo(14),
    updated_at: dayAgo(14),
    archived_at: null,
  },
  {
    id: 'res-5',
    user_id: USER_ID,
    name: 'ML Engineer (aspirational)',
    label: 'ml-aspirational',
    tags: [
      { axis: 'role', value: 'ml' },
      { axis: 'seniority', value: 'senior' },
      { axis: 'stack', value: 'python' },
    ],
    active_version_id: 'ver-5-1',
    version_count: 1,
    attached_job_count: 0,
    submitted_count: 0,
    created_at: dayAgo(3),
    updated_at: dayAgo(3),
    archived_at: null,
  },
  {
    id: 'res-6',
    user_id: USER_ID,
    name: 'Backend — EU format',
    label: 'backend-eu',
    tags: [
      { axis: 'role', value: 'backend' },
      { axis: 'geo', value: 'eu' },
      { axis: 'stack', value: 'go' },
    ],
    active_version_id: 'ver-6-2',
    version_count: 2,
    attached_job_count: 2,
    submitted_count: 1,
    created_at: dayAgo(25),
    updated_at: dayAgo(10),
    archived_at: null,
  },
  {
    id: 'res-7',
    user_id: USER_ID,
    name: 'Fintech-targeted Backend',
    label: 'backend-fintech',
    tags: [
      { axis: 'role', value: 'backend' },
      { axis: 'industry', value: 'fintech' },
      { axis: 'seniority', value: 'senior' },
    ],
    active_version_id: 'ver-7-3',
    version_count: 3,
    attached_job_count: 5,
    submitted_count: 3,
    created_at: dayAgo(45),
    updated_at: dayAgo(1),
    archived_at: null,
  },
  {
    id: 'res-8',
    user_id: USER_ID,
    name: 'Return-to-IC (post-EM)',
    label: 'return-to-ic',
    tags: [
      { axis: 'role', value: 'backend' },
      { axis: 'seniority', value: 'staff' },
    ],
    active_version_id: 'ver-8-1',
    version_count: 1,
    attached_job_count: 0,
    submitted_count: 0,
    created_at: dayAgo(7),
    updated_at: dayAgo(7),
    archived_at: null,
  },
]

const SAMPLE_CONTENT = `# Senior Backend Engineer

## Summary
Senior backend engineer with 8 years building distributed systems in Go.
Pipeline ownership end-to-end: Kafka/NATS, Postgres, Kubernetes.

## Experience
**Staff Engineer, Acme Co** — 2022–present
- Designed event-sourced order pipeline handling 12k msg/s
- Cut p99 latency 340ms → 85ms via batched commits + sharded consumers
- Mentored 4 engineers; led platform on-call rotation

**Senior Engineer, BetaCorp** — 2019–2022
- Built billing service: Stripe + idempotency layer, zero double-charges in 2y
- Migrated monolith → service-oriented Go; cut deploy time 22m → 4m

## Skills
Go · Postgres · Kubernetes · Kafka · NATS · OpenTelemetry · Redis
`

export const MOCK_VERSIONS: Record<string, ResumeVersion[]> = {
  'res-1': [
    {
      id: 'ver-1-1',
      resume_id: 'res-1',
      version_number: 1,
      content_text: SAMPLE_CONTENT,
      file_path: 'resumes/admin-user-1/res-1/v1.pdf',
      file_format: 'pdf',
      size_bytes: 142_318,
      change_summary: 'Initial upload',
      created_at: dayAgo(40),
      is_archived: false,
    },
    {
      id: 'ver-1-2',
      resume_id: 'res-1',
      version_number: 2,
      content_text: SAMPLE_CONTENT,
      file_path: 'resumes/admin-user-1/res-1/v2.pdf',
      file_format: 'pdf',
      size_bytes: 144_002,
      change_summary: 'Tightened Acme bullets, added p99 metric',
      created_at: dayAgo(20),
      is_archived: false,
    },
    {
      id: 'ver-1-3',
      resume_id: 'res-1',
      version_number: 3,
      content_text: SAMPLE_CONTENT,
      file_path: 'resumes/admin-user-1/res-1/v3.pdf',
      file_format: 'pdf',
      size_bytes: 145_100,
      change_summary: 'Added Kafka throughput numbers for fintech apps',
      created_at: dayAgo(10),
      is_archived: false,
    },
    {
      id: 'ver-1-4',
      resume_id: 'res-1',
      version_number: 4,
      content_text: SAMPLE_CONTENT,
      file_path: 'resumes/admin-user-1/res-1/v4.pdf',
      file_format: 'pdf',
      size_bytes: 146_540,
      change_summary: 'Reordered skills — Go first for the Stripe role',
      created_at: dayAgo(2),
      is_archived: false,
    },
  ],
  'res-2': [
    {
      id: 'ver-2-1',
      resume_id: 'res-2',
      version_number: 1,
      content_text: SAMPLE_CONTENT,
      file_path: 'resumes/admin-user-1/res-2/v1.pdf',
      file_format: 'pdf',
      size_bytes: 138_220,
      change_summary: 'Forked from Backend Senior — emphasised k8s',
      created_at: dayAgo(35),
      is_archived: false,
    },
    {
      id: 'ver-2-2',
      resume_id: 'res-2',
      version_number: 2,
      content_text: SAMPLE_CONTENT,
      file_path: 'resumes/admin-user-1/res-2/v2.pdf',
      file_format: 'pdf',
      size_bytes: 139_100,
      change_summary: 'Added observability platform work',
      created_at: dayAgo(18),
      is_archived: false,
    },
    {
      id: 'ver-2-3',
      resume_id: 'res-2',
      version_number: 3,
      content_text: SAMPLE_CONTENT,
      file_path: 'resumes/admin-user-1/res-2/v3.pdf',
      file_format: 'pdf',
      size_bytes: 140_002,
      change_summary: 'Promoted Staff-level scope phrasing',
      created_at: dayAgo(5),
      is_archived: false,
    },
  ],
}

export const MOCK_ATTACHMENTS: ResumeJobAttachment[] = [
  {
    id: 'att-1',
    resume_id: 'res-1',
    version_id: 'ver-1-4',
    job_id: 'job-1001',
    state: 'final',
    notes: 'Recruiter referral — emphasise Stripe billing experience.',
    attached_at: dayAgo(3),
    finalized_at: dayAgo(1),
    submitted_at: null,
  },
  {
    id: 'att-2',
    resume_id: 'res-1',
    version_id: 'ver-1-3',
    job_id: 'job-1002',
    state: 'submitted',
    notes: null,
    attached_at: dayAgo(8),
    finalized_at: dayAgo(7),
    submitted_at: dayAgo(6),
  },
  {
    id: 'att-3',
    resume_id: 'res-1',
    version_id: 'ver-1-4',
    job_id: 'job-1003',
    state: 'draft',
    notes: 'Need to add the Kafka throughput story.',
    attached_at: dayAgo(2),
    finalized_at: null,
    submitted_at: null,
  },
  {
    id: 'att-4',
    resume_id: 'res-2',
    version_id: 'ver-2-3',
    job_id: 'job-1004',
    state: 'final',
    notes: 'Platform IC4 — already tailored.',
    attached_at: dayAgo(4),
    finalized_at: dayAgo(2),
    submitted_at: null,
  },
  {
    id: 'att-5',
    resume_id: 'res-7',
    version_id: 'ver-7-3',
    job_id: 'job-1005',
    state: 'submitted',
    notes: null,
    attached_at: dayAgo(15),
    finalized_at: dayAgo(14),
    submitted_at: dayAgo(13),
  },
  {
    id: 'att-6',
    resume_id: 'res-7',
    version_id: 'ver-7-3',
    job_id: 'job-1006',
    state: 'draft',
    notes: null,
    attached_at: dayAgo(1),
    finalized_at: null,
    submitted_at: null,
  },
]

const MOCK_JOBS: Record<string, ApplyQueueItem['job']> = {
  'job-1001': {
    id: 'job-1001',
    title: 'Senior Backend Engineer — Payments',
    company: 'Stripe',
    location: 'Remote (US)',
    url: 'https://stripe.com/jobs/listing/123',
    salary: '$220k–$280k',
    saved_at: dayAgo(4),
    lifecycle_status: 'saved',
  },
  'job-1002': {
    id: 'job-1002',
    title: 'Backend Engineer, Core Platform',
    company: 'Linear',
    location: 'Remote',
    url: 'https://linear.app/careers/job/abc',
    salary: '$200k–$250k',
    saved_at: dayAgo(9),
    lifecycle_status: 'applied',
  },
  'job-1003': {
    id: 'job-1003',
    title: 'Senior Software Engineer — Data Plane',
    company: 'Confluent',
    location: 'Remote (US)',
    url: 'https://confluent.io/careers/dp',
    salary: '$210k–$260k',
    saved_at: dayAgo(3),
    lifecycle_status: 'saved',
  },
  'job-1004': {
    id: 'job-1004',
    title: 'Staff Platform Engineer',
    company: 'Vercel',
    location: 'Remote',
    url: 'https://vercel.com/careers/staff-platform',
    salary: '$260k–$320k',
    saved_at: dayAgo(5),
    lifecycle_status: 'saved',
  },
  'job-1005': {
    id: 'job-1005',
    title: 'Senior Engineer — Risk Platform',
    company: 'Mercury',
    location: 'Remote (US)',
    url: 'https://mercury.com/careers/risk',
    salary: '$215k–$265k',
    saved_at: dayAgo(16),
    lifecycle_status: 'interviewing',
  },
  'job-1006': {
    id: 'job-1006',
    title: 'Senior Backend — Capital Markets',
    company: 'Brex',
    location: 'Remote',
    url: 'https://brex.com/careers/capital',
    salary: '$210k–$255k',
    saved_at: dayAgo(2),
    lifecycle_status: 'saved',
  },
  'job-1007': {
    id: 'job-1007',
    title: 'Software Engineer, Backend',
    company: 'Plaid',
    location: 'Remote (US)',
    url: 'https://plaid.com/careers/be',
    salary: '$190k–$230k',
    saved_at: dayAgo(1),
    lifecycle_status: 'saved',
  },
  'job-1008': {
    id: 'job-1008',
    title: 'Backend Engineer — Trading Systems',
    company: 'Robinhood',
    location: 'Remote (US)',
    url: 'https://robinhood.com/careers/trading',
    salary: '$225k–$275k',
    saved_at: dayAgo(30),
    lifecycle_status: 'interviewing',
  },
  'job-1009': {
    id: 'job-1009',
    title: 'Senior Software Engineer — Search',
    company: 'Algolia',
    location: 'Remote',
    url: 'https://algolia.com/careers/search',
    salary: '$200k–$240k',
    saved_at: dayAgo(22),
    lifecycle_status: 'applied',
  },
}

// Adds extra attachments for the new lifecycle examples (interviewing, applied).
// Concatenated with MOCK_ATTACHMENTS in the queue builder.
const EXTRA_ATTACHMENTS: ResumeJobAttachment[] = [
  {
    id: 'att-7',
    resume_id: 'res-1',
    version_id: 'ver-1-4',
    job_id: 'job-1008',
    state: 'submitted',
    notes: 'Submitted via referral. Phone screen scheduled.',
    attached_at: dayAgo(31),
    finalized_at: dayAgo(30),
    submitted_at: dayAgo(28),
  },
  {
    id: 'att-8',
    resume_id: 'res-7',
    version_id: 'ver-7-3',
    job_id: 'job-1009',
    state: 'submitted',
    notes: null,
    attached_at: dayAgo(23),
    finalized_at: dayAgo(22),
    submitted_at: dayAgo(21),
  },
]

// Saved jobs that currently have NO resume attached — shown in Apply Queue as
// rows with a "Select resume" dropdown.
export const MOCK_UNATTACHED_JOB_IDS = ['job-1007']

// Helper: lifecycle status of a job (looked up from MOCK_JOBS).
export function lifecycleStatusFor(jobId: string): JobLifecycleStatus | undefined {
  return MOCK_JOBS[jobId]?.lifecycle_status
}

// Builds the full apply queue view-model. Rules:
//   - Every job with an attachment appears (regardless of lifecycle_status —
//     applied / interviewing / submitted are surfaced for application tracking)
//   - Saved jobs WITHOUT an attachment also appear (so the user can pick a
//     resume from the queue itself)
//   - applied / interviewing jobs WITHOUT an attachment never appear here —
//     they belong in the Jobs feed, not the resume workflow.
//   - Dismissed / new are excluded.
export function buildApplyQueue(): ApplyQueueItem[] {
  const allAttachments = [...MOCK_ATTACHMENTS, ...EXTRA_ATTACHMENTS]

  const attached: ApplyQueueItem[] = allAttachments.flatMap((att) => {
    const resume = MOCK_RESUMES.find((r) => r.id === att.resume_id)
    const job = MOCK_JOBS[att.job_id]
    if (!resume || !job) return []
    const versions = MOCK_VERSIONS[att.resume_id] ?? []
    const ver = versions.find((v) => v.id === att.version_id)
    return [
      {
        attachment: att,
        job,
        resume: {
          id: resume.id,
          name: resume.name,
          version_number: ver?.version_number ?? 1,
        },
      },
    ]
  })

  const unattachedSaved: ApplyQueueItem[] = MOCK_UNATTACHED_JOB_IDS.flatMap((id) => {
    const job = MOCK_JOBS[id]
    if (!job || job.lifecycle_status !== 'saved') return []
    return [{ attachment: null, job, resume: null }]
  })

  return [...attached, ...unattachedSaved]
}

export function getMockJob(id: string): ApplyQueueItem['job'] | undefined {
  return MOCK_JOBS[id]
}

// Map of jobId → attachment (or null) for quick lookup by JobsTable / JobDetailPanel
// when surfacing the paperclip indicator.
export function getMockAttachmentForJob(jobId: string): ResumeJobAttachment | null {
  return [...MOCK_ATTACHMENTS, ...EXTRA_ATTACHMENTS].find((a) => a.job_id === jobId) ?? null
}
