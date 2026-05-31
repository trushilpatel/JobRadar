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

import {
  fetchAdminUsers,
  setUserRoles,
  setUserStatus,
  deleteAdminUser,
} from '../api/adminUsers.api'

function user(overrides: Record<string, unknown> = {}) {
  return {
    id: 'u1',
    email: 'a@b.com',
    display_name: null,
    status: 'active',
    created_at: '2026-01-01T00:00:00Z',
    roles: ['user'],
    ...overrides,
  }
}

function listResult(overrides: Record<string, unknown> = {}) {
  return { data: [user()], total: 1, page: 1, limit: 25, ...overrides }
}

beforeEach(() => {
  get.mockReset()
  patch.mockReset()
  del.mockReset()
})

afterEach(() => {
  vi.clearAllMocks()
})

describe('fetchAdminUsers', () => {
  it('parses a populated list result', async () => {
    get.mockResolvedValue(listResult())
    const result = await fetchAdminUsers({})
    expect(result.data).toHaveLength(1)
    expect(result.total).toBe(1)
  })

  it('parses an empty list result', async () => {
    get.mockResolvedValue(listResult({ data: [], total: 0 }))
    const result = await fetchAdminUsers({})
    expect(result.data).toEqual([])
  })

  it('uses default page/limit and omits filters when not provided', async () => {
    get.mockResolvedValue(listResult())
    await fetchAdminUsers({})
    const url = get.mock.calls[0]![0] as string
    expect(url).toContain('page=1')
    expect(url).toContain('limit=25')
    expect(url).not.toContain('status=')
    expect(url).not.toContain('email=')
  })

  it('includes status and email filters when provided', async () => {
    get.mockResolvedValue(listResult())
    await fetchAdminUsers({ status: 'suspended', email: 'x@y.com' }, 2, 10)
    const url = get.mock.calls[0]![0] as string
    expect(url).toContain('page=2')
    expect(url).toContain('limit=10')
    expect(url).toContain('status=suspended')
    expect(url).toContain('email=x%40y.com')
  })

  it('omits empty-string status filter', async () => {
    get.mockResolvedValue(listResult())
    await fetchAdminUsers({ status: '' })
    const url = get.mock.calls[0]![0] as string
    expect(url).not.toContain('status=')
  })

  it('throws when the server payload violates the schema', async () => {
    get.mockResolvedValue({ data: [user({ status: 'bogus' })], total: 1, page: 1, limit: 25 })
    await expect(fetchAdminUsers({})).rejects.toThrow()
  })

  it('propagates apiClient errors', async () => {
    get.mockRejectedValue(new Error('boom'))
    await expect(fetchAdminUsers({})).rejects.toThrow('boom')
  })
})

describe('setUserRoles', () => {
  it('PATCHes the roles endpoint with role_ids', async () => {
    patch.mockResolvedValue(undefined)
    await setUserRoles('u1', ['admin', 'user'])
    expect(patch).toHaveBeenCalledWith('/admin/users/u1/roles', {
      role_ids: ['admin', 'user'],
    })
  })

  it('propagates errors', async () => {
    patch.mockRejectedValue(new Error('fail'))
    await expect(setUserRoles('u1', [])).rejects.toThrow('fail')
  })
})

describe('setUserStatus', () => {
  it('PATCHes the status endpoint', async () => {
    patch.mockResolvedValue(undefined)
    await setUserStatus('u1', 'suspended')
    expect(patch).toHaveBeenCalledWith('/admin/users/u1/status', { status: 'suspended' })
  })

  it('propagates errors', async () => {
    patch.mockRejectedValue(new Error('fail'))
    await expect(setUserStatus('u1', 'active')).rejects.toThrow('fail')
  })
})

describe('deleteAdminUser', () => {
  it('DELETEs the user endpoint', async () => {
    del.mockResolvedValue(undefined)
    await deleteAdminUser('u1')
    expect(del).toHaveBeenCalledWith('/admin/users/u1')
  })

  it('propagates errors', async () => {
    del.mockRejectedValue(new Error('fail'))
    await expect(deleteAdminUser('u1')).rejects.toThrow('fail')
  })
})
