import type { ApplicationWithJob } from '@/types'

const BASE = '/api'

export async function fetchAllApplications(): Promise<ApplicationWithJob[]> {
  const res = await fetch(`${BASE}/applications`)
  if (!res.ok) throw new Error('Failed to fetch applications')
  return res.json()
}

export async function updateApplicationStage(
  jobResultId: string,
  appId: number,
  stage: string
): Promise<void> {
  const res = await fetch(`${BASE}/jobs/${jobResultId}/applications/${appId}/stage`, {
    method: 'PATCH',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ stage }),
  })
  if (!res.ok) throw new Error('Failed to update stage')
}

export async function updateApplicationNote(
  jobResultId: string,
  appId: number,
  note: string
): Promise<void> {
  const res = await fetch(`${BASE}/jobs/${jobResultId}/applications/${appId}/note`, {
    method: 'PATCH',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ note }),
  })
  if (!res.ok) throw new Error('Failed to update note')
}
