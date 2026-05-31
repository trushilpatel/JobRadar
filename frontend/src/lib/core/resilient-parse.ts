import type { z } from 'zod'

// Resilient response parsing — the runtime safety net for frontend↔backend
// contract drift.
//
// The backend is the source of truth for response shapes; the frontend's zod
// schemas mirror that contract. When they drift, we must NEVER let a single
// unexpected/missing field throw and blank an entire list (the failure mode
// that took down the jobs + companies pages). Instead: parse each item
// independently, keep the valid ones, and log + drop the rest.
//
// Drift is caught LOUDLY at build time by the strict contract tests (which
// parse real recorded payloads against these same schemas). This module is the
// belt-and-suspenders for the gap between deploys (version skew) or an
// unforeseen change — it degrades, it does not detonate.

type DriftLogger = (context: string, index: number, issues: string) => void

const defaultLogger: DriftLogger = (context, index, issues) => {
  console.warn(`[schema-drift] dropped ${context}[${index}]: ${issues}`)
}

let driftLogger: DriftLogger = defaultLogger

// Swap the drift logger (e.g. to forward to telemetry in production, or to
// assert in tests). Returns the previous logger so callers can restore it.
export function setSchemaDriftLogger(fn: DriftLogger): DriftLogger {
  const prev = driftLogger
  driftLogger = fn
  return prev
}

function formatIssues(error: z.ZodError): string {
  return error.issues.map((i) => `${i.path.join('.') || '(root)'}: ${i.message}`).join('; ')
}

/**
 * Parse an array of unknown items against `schema`, keeping the valid ones and
 * dropping (with a logged warning) any that fail. Never throws.
 *
 * @param schema  the per-item schema
 * @param items   the raw items (already known to be an array)
 * @param context a label for drift logs, e.g. "jobs" or "companies"
 */
export function parseItemsResilient<S extends z.ZodTypeAny>(
  schema: S,
  items: readonly unknown[],
  context: string
): z.infer<S>[] {
  const out: z.infer<S>[] = []
  items.forEach((item, index) => {
    const result = schema.safeParse(item)
    if (result.success) {
      out.push(result.data)
    } else {
      driftLogger(context, index, formatIssues(result.error))
    }
  })
  return out
}
