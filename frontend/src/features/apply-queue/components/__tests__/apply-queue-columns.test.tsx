import { describe, it, expect, vi } from 'vitest'
import { buildApplyQueueColumns } from '../apply-queue-columns'

describe('buildApplyQueueColumns', () => {
  const actions = {
    resumeOptions: [{ id: 'resume-1', label: 'Resume One' }],
    onResumeChange: vi.fn(),
    isResumeUpdatePending: vi.fn().mockReturnValue(false),
  }

  it('builds exactly the job, status, and resume columns', () => {
    const columns = buildApplyQueueColumns(actions)
    expect(columns.map((c) => c.id)).toEqual(['job', 'status', 'resume'])
  })

  it('disables sorting on the status and resume columns', () => {
    const columns = buildApplyQueueColumns(actions)
    const status = columns.find((c) => c.id === 'status')
    const resume = columns.find((c) => c.id === 'resume')
    expect(status?.enableSorting).toBe(false)
    expect(resume?.enableSorting).toBe(false)
  })
})
