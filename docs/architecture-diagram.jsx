import { useState } from 'react'

const COLORS = {
  worker: { bg: '#1e3a5f', border: '#3b82f6', text: '#93c5fd', badge: '#2563eb' },
  service: { bg: '#1a3a2a', border: '#22c55e', text: '#86efac', badge: '#16a34a' },
  kafka: { bg: '#3b2200', border: '#f97316', text: '#fdba74', badge: '#ea580c' },
  db: { bg: '#2d1b4e', border: '#a855f7', text: '#d8b4fe', badge: '#9333ea' },
  external: { bg: '#1e2a2a', border: '#14b8a6', text: '#5eead4', badge: '#0d9488' },
  frontend: { bg: '#1a1a3a', border: '#818cf8', text: '#c7d2fe', badge: '#6366f1' },
  infra: { bg: '#1e1e2e', border: '#64748b', text: '#94a3b8', badge: '#475569' },
}

const Arrow = ({ label, vertical = false, color = '#64748b', dashed = false }) => {
  if (vertical) {
    return (
      <div
        style={{
          display: 'flex',
          flexDirection: 'column',
          alignItems: 'center',
          gap: 4,
          padding: '4px 0',
        }}
      >
        <div style={{ width: 2, height: 20, background: color, opacity: 0.6 }} />
        <div
          style={{
            fontSize: 10,
            color: '#94a3b8',
            whiteSpace: 'nowrap',
            background: '#0f172a',
            padding: '1px 6px',
            borderRadius: 4,
          }}
        >
          {label}
        </div>
        <div
          style={{
            width: 0,
            height: 0,
            borderLeft: '5px solid transparent',
            borderRight: '5px solid transparent',
            borderTop: `8px solid ${color}`,
            opacity: 0.6,
          }}
        />
      </div>
    )
  }
  return (
    <div style={{ display: 'flex', alignItems: 'center', gap: 4, minWidth: 60 }}>
      <div
        style={{
          flex: 1,
          height: 2,
          background: dashed ? 'none' : color,
          backgroundImage: dashed
            ? `repeating-linear-gradient(90deg, ${color} 0, ${color} 6px, transparent 6px, transparent 12px)`
            : 'none',
          opacity: 0.6,
        }}
      />
      <div style={{ fontSize: 9, color: '#94a3b8', whiteSpace: 'nowrap' }}>{label}</div>
      <div
        style={{
          width: 0,
          height: 0,
          borderTop: '5px solid transparent',
          borderBottom: '5px solid transparent',
          borderLeft: `8px solid ${color}`,
          opacity: 0.6,
        }}
      />
    </div>
  )
}

const Box = ({ title, subtitle, items = [], type = 'worker', width = 180, onClick, selected }) => {
  const c = COLORS[type]
  return (
    <div
      onClick={onClick}
      style={{
        width,
        background: c.bg,
        border: `2px solid ${selected ? '#fff' : c.border}`,
        borderRadius: 10,
        padding: '10px 12px',
        cursor: onClick ? 'pointer' : 'default',
        boxShadow: selected ? `0 0 0 3px ${c.border}55, 0 4px 24px #0008` : '0 2px 12px #0006',
        transition: 'all 0.15s',
        position: 'relative',
      }}
    >
      <div
        style={{
          fontSize: 11,
          fontWeight: 700,
          color: c.text,
          letterSpacing: 0.3,
          marginBottom: 2,
        }}
      >
        {title}
      </div>
      {subtitle && <div style={{ fontSize: 9, color: '#64748b', marginBottom: 6 }}>{subtitle}</div>}
      {items.map((item, i) => (
        <div
          key={i}
          style={{
            fontSize: 9,
            color: '#94a3b8',
            marginTop: 3,
            display: 'flex',
            alignItems: 'center',
            gap: 4,
          }}
        >
          <div
            style={{ width: 5, height: 5, borderRadius: '50%', background: c.badge, flexShrink: 0 }}
          />
          {item}
        </div>
      ))}
    </div>
  )
}

const KafkaTopic = ({ name, subtitle }) => (
  <div
    style={{
      background: COLORS.kafka.bg,
      border: `2px solid ${COLORS.kafka.border}`,
      borderRadius: 8,
      padding: '8px 14px',
      minWidth: 170,
    }}
  >
    <div
      style={{
        fontSize: 9,
        color: COLORS.kafka.badge,
        fontWeight: 700,
        textTransform: 'uppercase',
        letterSpacing: 1,
        marginBottom: 2,
      }}
    >
      Kafka Topic
    </div>
    <div style={{ fontSize: 11, fontWeight: 700, color: COLORS.kafka.text }}>{name}</div>
    {subtitle && <div style={{ fontSize: 9, color: '#94a3b8', marginTop: 2 }}>{subtitle}</div>}
  </div>
)

const DBTable = ({ name, role, color = COLORS.db }) => (
  <div
    style={{
      background: color.bg,
      border: `1.5px solid ${color.border}`,
      borderRadius: 6,
      padding: '5px 10px',
      fontSize: 9,
    }}
  >
    <div style={{ fontWeight: 700, color: color.text }}>{name}</div>
    {role && <div style={{ color: '#64748b', marginTop: 1 }}>{role}</div>}
  </div>
)

const Section = ({ title, color, children, style = {} }) => (
  <div
    style={{
      border: `1px dashed ${color}33`,
      borderRadius: 12,
      padding: '14px 16px',
      position: 'relative',
      ...style,
    }}
  >
    <div
      style={{
        position: 'absolute',
        top: -10,
        left: 14,
        background: '#0f172a',
        padding: '0 8px',
        fontSize: 10,
        color,
        fontWeight: 700,
        textTransform: 'uppercase',
        letterSpacing: 1,
      }}
    >
      {title}
    </div>
    {children}
  </div>
)

const PANEL_DATA = {
  scheduler: {
    title: 'Company Scan Scheduler',
    color: COLORS.worker,
    sections: [
      { label: 'Role', value: 'Finds companies due for a scan every 1 minute and enqueues them' },
      { label: 'Trigger', value: 'Timer — runs every 60s' },
      { label: 'Reads', value: 'companies WHERE last_scan_at IS NULL OR stale' },
      { label: 'Writes', value: 'company_scan_queue (status=queued)' },
      { label: 'Binary', value: 'cmd/company_scan_scheduler' },
      { label: 'Metrics port', value: ':9101' },
    ],
  },
  crawler: {
    title: 'Job Listing Crawler',
    color: COLORS.worker,
    sections: [
      {
        label: 'Role',
        value:
          'Polls scan queue, fetches careers pages via ATS adapters, deduplicates, publishes new jobs',
      },
      {
        label: 'ATS Adapters',
        value: 'Greenhouse, Lever, Ashby, Workday, Workable, Chromedp, Google, Netflix, Uber, etc.',
      },
      { label: 'Reads', value: 'company_scan_queue, job_pipeline_ledger (dedup)' },
      {
        label: 'Writes',
        value: 'Kafka: job_listings.raw · DB: job_scan_log, company_scan_log, job_pipeline_ledger',
      },
      { label: 'Binary', value: 'cmd/job_listing_crawler' },
      { label: 'Metrics port', value: ':9102 (1 GB memory — runs headless Chromedp)' },
    ],
  },
  filter: {
    title: 'Job Listing Relevance Filter',
    color: COLORS.worker,
    sections: [
      {
        label: 'Role',
        value: 'Batch LLM scoring — routes relevant jobs forward, marks irrelevant ones',
      },
      { label: 'LLM', value: 'Google Gemini (batch of 50-100 titles)' },
      { label: 'Reads', value: 'Kafka: job_listings.raw' },
      {
        label: 'Writes',
        value:
          'Kafka: job_listings.filtered (relevant) · DB: job_pipeline_ledger (irrelevant → reason=llm_irrelevant)',
      },
      { label: 'Binary', value: 'cmd/job_listing_relevance_filter' },
      { label: 'Metrics port', value: ':9103' },
    ],
  },
  analyzer: {
    title: 'Job Post Analyzer',
    color: COLORS.worker,
    sections: [
      {
        label: 'Role',
        value: 'Fetches full job page, extracts structured data with LLM, writes to job_feed',
      },
      {
        label: 'LLM Extracts',
        value:
          'experience_years, tech_stack[], salary_range, sponsorship, citizenship, security_clearance, role_brief, responsibilities',
      },
      { label: 'Reads', value: 'Kafka: job_listings.filtered' },
      { label: 'Writes', value: 'DB: job_feed, job_analytics, job_pipeline_ledger' },
      { label: 'Binary', value: 'cmd/job_post_analyzer' },
      { label: 'Metrics port', value: ':9104' },
    ],
  },
  api: {
    title: 'REST API Server',
    color: COLORS.service,
    sections: [
      { label: 'Port', value: ':8080' },
      { label: 'Router', value: 'Chi v5' },
      { label: 'Services', value: 'company · job · scan · application · dashboard · insights' },
      {
        label: 'Key endpoints',
        value:
          'GET /jobs, PATCH /jobs/:id/save, DELETE /jobs/:id, GET /dashboard/stats, GET /insights/tech-stack, POST /companies/:id/scan, …',
      },
      { label: 'Binary', value: 'cmd/api' },
    ],
  },
  frontend: {
    title: 'Frontend (React SPA)',
    color: COLORS.frontend,
    sections: [
      { label: 'Port', value: ':3000 (Nginx)' },
      {
        label: 'Stack',
        value: 'React · TypeScript · TanStack Query · Zustand · Radix UI · Tailwind',
      },
      { label: 'Pages', value: 'Jobs feed · Companies · Dashboard · Insights' },
      { label: 'State', value: 'Server: TanStack Query · UI: Zustand · Local: useState' },
    ],
  },
  kafka: {
    title: 'Kafka (Redpanda)',
    color: COLORS.kafka,
    sections: [
      { label: 'Image', value: 'redpandadata/redpanda:v24.3.1' },
      { label: 'Internal port', value: ':29092 (inter-service)' },
      { label: 'External port', value: ':9092' },
      { label: 'Topics', value: 'job_listings.raw · job_listings.filtered' },
      {
        label: 'Note',
        value: 'No dead-letter topic — failures recorded in DB via job_pipeline_ledger',
      },
    ],
  },
  postgres: {
    title: 'PostgreSQL 16',
    color: COLORS.db,
    sections: [
      { label: 'Port', value: ':5432' },
      { label: 'Migrations', value: '27 goose SQL files in backend/migrations/' },
      {
        label: 'Key tables',
        value:
          'companies · company_scan_queue · company_scan_log · job_pipeline_ledger · job_feed · job_analytics · job_application_tracker · job_dismissal_log',
      },
      {
        label: 'Pattern',
        value: 'Append-only scan/application logs; job_feed holds active + dismissed rows',
      },
    ],
  },
  gemini: {
    title: 'Google Gemini API',
    color: COLORS.external,
    sections: [
      {
        label: 'Used by',
        value: 'Filter worker (batch scoring) · Analyzer worker (detail extraction)',
      },
      { label: 'Filter call', value: 'Input: job titles batch → Output: relevance scores' },
      {
        label: 'Analyzer call',
        value: 'Input: full job HTML text → Output: structured JSON (tech, salary, visa, etc.)',
      },
    ],
  },
  observability: {
    title: 'Observability Stack',
    color: COLORS.infra,
    sections: [
      {
        label: 'Tracing',
        value: 'Jaeger 1.57 · all-in-one · port :16686 UI, :4318 OTLP · OTel SDK in every worker',
      },
      { label: 'Metrics', value: 'Prometheus 2.51 · scrapes /metrics on each worker · port :9090' },
      {
        label: 'Logging',
        value: 'zerolog (JSON) · infra/logger.Logger · structured fields from logger/fields.go',
      },
    ],
  },
}

const DetailPanel = ({ id, onClose }) => {
  const data = PANEL_DATA[id]
  if (!data) return null
  const c = data.color
  return (
    <div
      style={{
        position: 'fixed',
        top: 0,
        right: 0,
        height: '100vh',
        width: 360,
        background: '#0f172a',
        borderLeft: `2px solid ${c.border}`,
        padding: '24px 20px',
        overflowY: 'auto',
        zIndex: 100,
        boxShadow: '-8px 0 40px #0008',
      }}
    >
      <div
        style={{
          display: 'flex',
          justifyContent: 'space-between',
          alignItems: 'flex-start',
          marginBottom: 20,
        }}
      >
        <div>
          <div
            style={{
              fontSize: 9,
              color: c.border,
              fontWeight: 700,
              textTransform: 'uppercase',
              letterSpacing: 1,
              marginBottom: 4,
            }}
          >
            Component Detail
          </div>
          <div style={{ fontSize: 16, fontWeight: 700, color: c.text }}>{data.title}</div>
        </div>
        <button
          onClick={onClose}
          style={{
            background: 'none',
            border: 'none',
            color: '#64748b',
            cursor: 'pointer',
            fontSize: 20,
            padding: 0,
            lineHeight: 1,
          }}
        >
          ✕
        </button>
      </div>
      {data.sections.map((s, i) => (
        <div
          key={i}
          style={{ marginBottom: 14, borderLeft: `3px solid ${c.border}33`, paddingLeft: 12 }}
        >
          <div
            style={{
              fontSize: 10,
              color: c.border,
              fontWeight: 700,
              textTransform: 'uppercase',
              letterSpacing: 0.5,
              marginBottom: 3,
            }}
          >
            {s.label}
          </div>
          <div style={{ fontSize: 12, color: '#cbd5e1', lineHeight: 1.5 }}>{s.value}</div>
        </div>
      ))}
    </div>
  )
}

export default function App() {
  const [selected, setSelected] = useState(null)

  const toggle = (id) => setSelected((sel) => (sel === id ? null : id))

  return (
    <div
      style={{
        background: '#0f172a',
        minHeight: '100vh',
        fontFamily: 'system-ui, sans-serif',
        color: '#f1f5f9',
        padding: '28px 32px',
        paddingRight: selected ? 400 : 32,
      }}
    >
      {/* Header */}
      <div style={{ marginBottom: 28 }}>
        <div style={{ fontSize: 22, fontWeight: 800, letterSpacing: -0.5, color: '#f8fafc' }}>
          JobRadar — Architecture
        </div>
        <div style={{ fontSize: 12, color: '#64748b', marginTop: 4 }}>
          Click any component to see details &nbsp;·&nbsp; Single-user automated job tracker
        </div>
      </div>

      {/* Legend */}
      <div style={{ display: 'flex', gap: 16, marginBottom: 24, flexWrap: 'wrap' }}>
        {[
          { label: 'Worker / Binary', color: COLORS.worker.border },
          { label: 'Service (API)', color: COLORS.service.border },
          { label: 'Kafka Topic', color: COLORS.kafka.border },
          { label: 'PostgreSQL', color: COLORS.db.border },
          { label: 'External API', color: COLORS.external.border },
          { label: 'Frontend', color: COLORS.frontend.border },
          { label: 'Infra / Observability', color: COLORS.infra.border },
        ].map((l) => (
          <div
            key={l.label}
            style={{
              display: 'flex',
              alignItems: 'center',
              gap: 6,
              fontSize: 10,
              color: '#94a3b8',
            }}
          >
            <div style={{ width: 10, height: 10, borderRadius: 3, background: l.color }} />
            {l.label}
          </div>
        ))}
      </div>

      {/* ─── PIPELINE ROW ─── */}
      <Section
        title="Automated Job Pipeline"
        color={COLORS.worker.border}
        style={{ marginBottom: 24 }}
      >
        <div style={{ display: 'flex', alignItems: 'center', flexWrap: 'wrap', gap: 0 }}>
          {/* Scheduler */}
          <Box
            title="Scheduler"
            subtitle="every 1 min"
            type="worker"
            width={148}
            selected={selected === 'scheduler'}
            onClick={() => toggle('scheduler')}
            items={['Queries companies', 'Enqueues due scans']}
          />
          <Arrow label="DB poll" color={COLORS.db.border} />

          {/* DB queue */}
          <DBTable name="company_scan_queue" role="work queue" />
          <Arrow label="claim" color={COLORS.db.border} />

          {/* Crawler */}
          <Box
            title="Crawler"
            subtitle="ATS adapters"
            type="worker"
            width={160}
            selected={selected === 'crawler'}
            onClick={() => toggle('crawler')}
            items={['Greenhouse / Lever / Ashby', 'Workday / Chromedp / …', 'Dedup via ledger']}
          />
          <Arrow label="publish" color={COLORS.kafka.border} />

          {/* Kafka raw */}
          <KafkaTopic name="job_listings.raw" subtitle="title · url · company_id" />
          <Arrow label="consume" color={COLORS.kafka.border} />

          {/* Filter */}
          <Box
            title="Relevance Filter"
            subtitle="LLM batch scoring"
            type="worker"
            width={160}
            selected={selected === 'filter'}
            onClick={() => toggle('filter')}
            items={['Batch 50–100 titles', 'Gemini relevance score', 'Routes relevant only']}
          />
          <Arrow label="publish" color={COLORS.kafka.border} />

          {/* Kafka filtered */}
          <KafkaTopic name="job_listings.filtered" subtitle="relevant jobs only" />
          <Arrow label="consume" color={COLORS.kafka.border} />

          {/* Analyzer */}
          <Box
            title="Job Post Analyzer"
            subtitle="LLM enrichment"
            type="worker"
            width={168}
            selected={selected === 'analyzer'}
            onClick={() => toggle('analyzer')}
            items={[
              'Fetches full job page',
              'Extracts: tech · salary · visa',
              'Writes to job_feed',
            ]}
          />
        </div>
      </Section>

      {/* ─── MIDDLE ROW: Gemini + API + DB ─── */}
      <div style={{ display: 'flex', gap: 16, marginBottom: 24, flexWrap: 'wrap' }}>
        {/* Google Gemini */}
        <Section title="External API" color={COLORS.external.border} style={{ flex: '0 0 auto' }}>
          <Box
            title="Google Gemini"
            subtitle="LLM provider"
            type="external"
            width={170}
            selected={selected === 'gemini'}
            onClick={() => toggle('gemini')}
            items={[
              'Filter: batch title scoring',
              'Analyzer: full-page extraction',
              'tech · salary · visa · role',
            ]}
          />
          <div style={{ marginTop: 8, fontSize: 9, color: '#64748b', textAlign: 'center' }}>
            ↑ Filter · Analyzer call ↑
          </div>
        </Section>

        {/* API Server + Services */}
        <Section
          title="API Layer (port :8080)"
          color={COLORS.service.border}
          style={{ flex: 1, minWidth: 320 }}
        >
          <div style={{ display: 'flex', gap: 10, flexWrap: 'wrap', alignItems: 'flex-start' }}>
            <Box
              title="API Server"
              subtitle="cmd/api · Chi router"
              type="service"
              width={160}
              selected={selected === 'api'}
              onClick={() => toggle('api')}
              items={['/jobs · /companies', '/dashboard · /insights', '/applications']}
            />
            <div style={{ display: 'flex', flexDirection: 'column', gap: 8, paddingTop: 4 }}>
              {['company', 'job', 'scan', 'application', 'dashboard', 'insights'].map((s) => (
                <div
                  key={s}
                  style={{
                    background: '#0d2a1a',
                    border: `1px solid ${COLORS.service.border}55`,
                    borderRadius: 5,
                    padding: '3px 10px',
                    fontSize: 9,
                    color: COLORS.service.text,
                    fontWeight: 600,
                  }}
                >
                  {s} service
                </div>
              ))}
            </div>
          </div>
        </Section>

        {/* PostgreSQL tables */}
        <Section
          title="PostgreSQL 16 (port :5432)"
          color={COLORS.db.border}
          style={{ flex: '0 0 auto' }}
        >
          <div
            onClick={() => toggle('postgres')}
            style={{
              cursor: 'pointer',
              padding: '4px 8px',
              borderRadius: 6,
              border: selected === 'postgres' ? '1.5px solid #fff' : '1.5px solid transparent',
            }}
          >
            <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 5 }}>
              {[
                { name: 'companies', role: 'tracked companies' },
                { name: 'company_scan_queue', role: 'work queue' },
                { name: 'company_scan_log', role: 'scan history' },
                { name: 'job_pipeline_ledger', role: 'dedup + blocking' },
                { name: 'job_feed', role: 'active job entries' },
                { name: 'job_analytics', role: 'append-only analytics' },
                { name: 'job_application_tracker', role: 'application stages' },
                { name: 'job_dismissal_log', role: 'dismiss reasons' },
              ].map((t) => (
                <DBTable key={t.name} name={t.name} role={t.role} />
              ))}
            </div>
            <div style={{ marginTop: 6, fontSize: 9, color: '#64748b', textAlign: 'center' }}>
              click to expand
            </div>
          </div>
        </Section>
      </div>

      {/* ─── BOTTOM ROW: Frontend + Kafka + Observability ─── */}
      <div style={{ display: 'flex', gap: 16, flexWrap: 'wrap' }}>
        {/* Frontend */}
        <Section
          title="Frontend (port :3000)"
          color={COLORS.frontend.border}
          style={{ flex: 1, minWidth: 280 }}
        >
          <Box
            title="React SPA"
            subtitle="TypeScript · Nginx"
            type="frontend"
            width="100%"
            selected={selected === 'frontend'}
            onClick={() => toggle('frontend')}
            items={[
              'Pages: Jobs · Companies · Dashboard · Insights',
              'Server state: TanStack Query → REST API',
              'UI state: Zustand · Local: useState',
              'Components: Radix UI · Tailwind · clsx',
            ]}
          />
          <div style={{ marginTop: 8, fontSize: 9, color: '#64748b', textAlign: 'center' }}>
            ↑ HTTP REST calls to :8080 ↑
          </div>
        </Section>

        {/* Kafka */}
        <Section title="Message Broker" color={COLORS.kafka.border} style={{ flex: '0 0 auto' }}>
          <Box
            title="Kafka (Redpanda)"
            subtitle="v24.3.1 · :29092 internal"
            type="kafka"
            width={200}
            selected={selected === 'kafka'}
            onClick={() => toggle('kafka')}
            items={[
              'job_listings.raw  (crawler → filter)',
              'job_listings.filtered  (filter → analyzer)',
              'No dead-letter — errors → DB ledger',
            ]}
          />
        </Section>

        {/* Observability */}
        <Section title="Observability" color={COLORS.infra.border} style={{ flex: '0 0 auto' }}>
          <Box
            title="Jaeger + Prometheus"
            subtitle="OTel tracing · metrics scraping"
            type="infra"
            width={210}
            selected={selected === 'observability'}
            onClick={() => toggle('observability')}
            items={[
              'Jaeger :16686 — distributed traces',
              'Prometheus :9090 — metrics',
              'Workers export /metrics on :910x',
              'zerolog JSON logs everywhere',
            ]}
          />
        </Section>
      </div>

      {/* ─── Data flow key ─── */}
      <div style={{ marginTop: 28, borderTop: '1px solid #1e293b', paddingTop: 16 }}>
        <div
          style={{
            fontSize: 11,
            fontWeight: 700,
            color: '#475569',
            marginBottom: 10,
            textTransform: 'uppercase',
            letterSpacing: 1,
          }}
        >
          Data Flow Summary
        </div>
        <div style={{ display: 'flex', flexWrap: 'wrap', gap: 10 }}>
          {[
            {
              from: 'Scheduler',
              to: 'company_scan_queue',
              via: 'DB INSERT',
              color: COLORS.db.border,
            },
            {
              from: 'Crawler',
              to: 'job_listings.raw',
              via: 'Kafka PUBLISH',
              color: COLORS.kafka.border,
            },
            {
              from: 'Filter',
              to: 'job_listings.filtered',
              via: 'Kafka PUBLISH',
              color: COLORS.kafka.border,
            },
            {
              from: 'Analyzer',
              to: 'job_feed + job_analytics',
              via: 'DB INSERT',
              color: COLORS.db.border,
            },
            { from: 'Frontend', to: 'API :8080', via: 'HTTP REST', color: COLORS.service.border },
            {
              from: 'Filter / Analyzer',
              to: 'Google Gemini',
              via: 'HTTPS API',
              color: COLORS.external.border,
            },
            {
              from: 'All workers',
              to: 'Jaeger + Prometheus',
              via: 'OTel + /metrics',
              color: COLORS.infra.border,
            },
          ].map((f, i) => (
            <div
              key={i}
              style={{
                background: '#1e293b',
                borderRadius: 8,
                padding: '6px 12px',
                fontSize: 10,
                color: '#94a3b8',
                display: 'flex',
                alignItems: 'center',
                gap: 6,
              }}
            >
              <span style={{ color: '#f1f5f9', fontWeight: 600 }}>{f.from}</span>
              <span style={{ color: f.color }}>→ {f.via} →</span>
              <span style={{ color: '#f1f5f9', fontWeight: 600 }}>{f.to}</span>
            </div>
          ))}
        </div>
      </div>

      {/* Detail panel */}
      {selected && <DetailPanel id={selected} onClose={() => setSelected(null)} />}
    </div>
  )
}
