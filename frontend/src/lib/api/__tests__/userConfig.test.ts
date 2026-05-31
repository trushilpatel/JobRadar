import { describe, it, expect, vi, beforeEach } from 'vitest'

vi.mock('@/lib/core/api-client', () => ({
  apiClient: { get: vi.fn(), patch: vi.fn() },
}))

import { apiClient } from '@/lib/core/api-client'
import { getUserConfig, patchUserConfig } from '@/lib/api/userConfig'

const getMock = vi.mocked(apiClient.get)
const patchMock = vi.mocked(apiClient.patch)

beforeEach(() => {
  vi.clearAllMocks()
})

describe('getUserConfig', () => {
  it('parses a valid config', async () => {
    getMock.mockResolvedValue({ show_keyboard_shortcuts: true })
    await expect(getUserConfig()).resolves.toEqual({ show_keyboard_shortcuts: true })
    expect(getMock).toHaveBeenCalledWith('/me/config')
  })

  it('defaults show_keyboard_shortcuts to false when the field is missing', async () => {
    getMock.mockResolvedValue({})
    await expect(getUserConfig()).resolves.toEqual({ show_keyboard_shortcuts: false })
  })

  it('propagates the parse error on a malformed boolean field', async () => {
    getMock.mockResolvedValue({ show_keyboard_shortcuts: 'yes' })
    await expect(getUserConfig()).rejects.toThrow()
  })

  it('propagates an apiClient error', async () => {
    getMock.mockRejectedValue(new Error('Network error'))
    await expect(getUserConfig()).rejects.toThrow('Network error')
  })
})

describe('patchUserConfig', () => {
  it('sends the partial and parses the response', async () => {
    patchMock.mockResolvedValue({ show_keyboard_shortcuts: true })
    await expect(patchUserConfig({ show_keyboard_shortcuts: true })).resolves.toEqual({
      show_keyboard_shortcuts: true,
    })
    expect(patchMock).toHaveBeenCalledWith('/me/config', { show_keyboard_shortcuts: true })
  })

  it('defaults missing fields in the response to false', async () => {
    patchMock.mockResolvedValue({})
    await expect(patchUserConfig({})).resolves.toEqual({ show_keyboard_shortcuts: false })
  })

  it('propagates an apiClient error', async () => {
    patchMock.mockRejectedValue(new Error('boom'))
    await expect(patchUserConfig({ show_keyboard_shortcuts: true })).rejects.toThrow('boom')
  })
})
