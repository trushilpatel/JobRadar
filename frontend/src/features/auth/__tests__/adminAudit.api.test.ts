import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

const get = vi.fn()

vi.mock('@/lib/core/api-client', () => ({
  apiClient: {
    get: (...args: unknown[]) => get(...args),
  },
}))

import { fetchAuditLog } from '../api/adminAudit.api'

function entry(overrides: Record<string, unknown> = {}) {
  return {
    id: 'a1',
    actor_user_id: 'u1',
    actor_email: 'a@b.com',
    action: 'user.suspend',
    target_type: 'user',
    target_id: 'u2',
    payload: { reason: 'spam' },
    created_at: '2026-01-01T00:00:00Z',
    ...overrides,
  }
}

function listResult(overrides: Record<string, unknown> = {}) {
  return { data: [entry()], total: 1, page: 1, limit: 50, ...overrides }
}

beforeEach(() => {
  get.mockReset()
})

afterEach(() => {
  vi.clearAllMocks()
})

describe('fetchAuditLog', () => {
  it('parses a populated list result', async () => {
    get.mockResolvedValue(listResult())
    const result = await fetchAuditLog({})
    expect(result.data).toHaveLength(1)
    expect(result.data[0]!.action).toBe('user.suspend')
  })

  it('parses an empty list result', async () => {
    get.mockResolvedValue(listResult({ data: [], total: 0 }))
    const result = await fetchAuditLog({})
    expect(result.data).toEqual([])
  })

  it('accepts nullable actor/target fields and unknown payload', async () => {
    get.mockResolvedValue(
      listResult({
        data: [
          entry({
            actor_user_id: null,
            actor_email: null,
            target_type: null,
            target_id: null,
            payload: null,
          }),
        ],
      })
    )
    const result = await fetchAuditLog({})
    expect(result.data[0]!.actor_user_id).toBeNull()
    expect(result.data[0]!.payload).toBeNull()
  })

  it('accepts a row missing the actor_email key entirely (Go omitempty on a nil pointer)', async () => {
    const rowWithoutActorEmail = entry({ actor_user_id: null }) as Record<string, unknown>
    delete rowWithoutActorEmail.actor_email
    get.mockResolvedValue(listResult({ data: [rowWithoutActorEmail] }))
    const result = await fetchAuditLog({})
    expect(result.data[0]!.actor_email).toBeUndefined()
  })

  it('uses default page/limit and omits filters when not provided', async () => {
    get.mockResolvedValue(listResult())
    await fetchAuditLog({})
    const url = get.mock.calls[0]![0] as string
    expect(url).toContain('page=1')
    expect(url).toContain('limit=50')
    expect(url).not.toContain('actor_user_id=')
    expect(url).not.toContain('action=')
  })

  it('includes actor_user_id and action filters when provided', async () => {
    get.mockResolvedValue(listResult())
    await fetchAuditLog({ actor_user_id: 'u9', action: 'user.delete' }, 3, 20)
    const url = get.mock.calls[0]![0] as string
    expect(url).toContain('page=3')
    expect(url).toContain('limit=20')
    expect(url).toContain('actor_user_id=u9')
    expect(url).toContain('action=user.delete')
  })

  it('throws when the server payload violates the schema', async () => {
    get.mockResolvedValue({ data: [entry({ action: 123 })], total: 1, page: 1, limit: 50 })
    await expect(fetchAuditLog({})).rejects.toThrow()
  })

  it('propagates apiClient errors', async () => {
    get.mockRejectedValue(new Error('boom'))
    await expect(fetchAuditLog({})).rejects.toThrow('boom')
  })
})
