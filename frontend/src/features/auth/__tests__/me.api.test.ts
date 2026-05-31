import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

const get = vi.fn()
const patch = vi.fn()
const del = vi.fn()

vi.mock('@/lib/core/api-client', () => ({
  apiClient: {
    get: (...args: unknown[]) => get(...args),
    patch: (...args: unknown[]) => patch(...args),
    delete: (...args: unknown[]) => del(...args),
  },
}))

import { fetchMe, updateMe, deleteMe } from '../api/me.api'

function identity(overrides: Record<string, unknown> = {}) {
  return {
    user_id: 'u1',
    email: 'a@b.com',
    display_name: 'Ada',
    status: 'active',
    roles: ['user', 'admin'],
    ...overrides,
  }
}

beforeEach(() => {
  get.mockReset()
  patch.mockReset()
  del.mockReset()
})

afterEach(() => {
  vi.clearAllMocks()
})

describe('fetchMe', () => {
  it('parses a valid identity', async () => {
    get.mockResolvedValue(identity())
    const result = await fetchMe()
    expect(result.user_id).toBe('u1')
    expect(result.roles).toEqual(['user', 'admin'])
    expect(get).toHaveBeenCalledWith('/me')
  })

  it('parses an identity with empty roles', async () => {
    get.mockResolvedValue(identity({ roles: [] }))
    const result = await fetchMe()
    expect(result.roles).toEqual([])
  })

  it('throws when status is invalid', async () => {
    get.mockResolvedValue(identity({ status: 'pending' }))
    await expect(fetchMe()).rejects.toThrow()
  })

  it('throws when a required field is missing', async () => {
    const partial = identity()
    delete (partial as Record<string, unknown>).email
    get.mockResolvedValue(partial)
    await expect(fetchMe()).rejects.toThrow()
  })

  it('propagates apiClient errors', async () => {
    get.mockRejectedValue(new Error('network'))
    await expect(fetchMe()).rejects.toThrow('network')
  })
})

describe('updateMe', () => {
  it('PATCHes /me with display_name', async () => {
    patch.mockResolvedValue(undefined)
    await updateMe('New Name')
    expect(patch).toHaveBeenCalledWith('/me', { display_name: 'New Name' })
  })

  it('propagates errors', async () => {
    patch.mockRejectedValue(new Error('fail'))
    await expect(updateMe('x')).rejects.toThrow('fail')
  })
})

describe('deleteMe', () => {
  it('DELETEs /me', async () => {
    del.mockResolvedValue(undefined)
    await deleteMe()
    expect(del).toHaveBeenCalledWith('/me')
  })

  it('propagates errors', async () => {
    del.mockRejectedValue(new Error('fail'))
    await expect(deleteMe()).rejects.toThrow('fail')
  })
})
