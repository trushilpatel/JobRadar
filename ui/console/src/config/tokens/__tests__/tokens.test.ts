import { describe, it, expect } from 'vitest'
import { TOKENS, getActionTokens } from '@/config/tokens'

describe('getActionTokens', () => {
  it('returns all tokens with a shortcut field', () => {
    const actionTokens = getActionTokens()
    const allWithShortcut = Object.entries(TOKENS).filter(
      ([, v]) => 'shortcut' in v && v.shortcut !== undefined
    )
    expect(actionTokens).toHaveLength(allWithShortcut.length)
  })

  it('does not include tokens without a shortcut field', () => {
    const actionTokens = getActionTokens()
    for (const token of actionTokens) {
      expect(token.shortcut).toBeDefined()
      expect(typeof token.shortcut).toBe('string')
    }
  })

  it('returns tokens with key, copy, and shortcut fields', () => {
    const actionTokens = getActionTokens()
    expect(actionTokens.length).toBeGreaterThan(0)
    for (const token of actionTokens) {
      expect(token).toHaveProperty('key')
      expect(token).toHaveProperty('copy')
      expect(token).toHaveProperty('shortcut')
      expect(typeof token.key).toBe('string')
      expect(typeof token.copy).toBe('string')
      expect(typeof token.shortcut).toBe('string')
    }
  })
})

describe('TOKENS type safety', () => {
  it('jobs.save has copy and shortcut', () => {
    const token = TOKENS['jobs.save']
    expect(token.copy).toBe('Save')
    expect(token.shortcut).toBe('s')
  })

  it('nav.dashboard has copy but no shortcut', () => {
    const token = TOKENS['nav.dashboard']
    expect(token.copy).toBe('Dashboard')
    expect('shortcut' in token).toBe(false)
  })
})
