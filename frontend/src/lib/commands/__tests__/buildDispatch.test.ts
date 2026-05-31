import { describe, it, expect, beforeEach } from 'vitest'
import { buildDispatch } from '../buildDispatch'
import type { CommandRegistry } from '../types'

function stubPlatform(mac: boolean) {
  Object.defineProperty(window.navigator, 'platform', {
    value: mac ? 'MacIntel' : 'Win32',
    configurable: true,
  })
}

describe('buildDispatch', () => {
  beforeEach(() => {
    stubPlatform(true)
  })

  it('expands mod+k to meta+k on Mac', () => {
    stubPlatform(true)
    const reg: CommandRegistry = { 'a.b': { label: 'X', keys: ['mod+k'] } }
    const { lookup } = buildDispatch(reg)
    expect(lookup.get('meta+k')).toBe('a.b')
    expect(lookup.has('ctrl+k')).toBe(false)
  })

  it('expands mod+k to ctrl+k on non-Mac', () => {
    stubPlatform(false)
    const reg: CommandRegistry = { 'a.b': { label: 'X', keys: ['mod+k'] } }
    const { lookup } = buildDispatch(reg)
    expect(lookup.get('ctrl+k')).toBe('a.b')
    expect(lookup.has('meta+k')).toBe(false)
  })

  it('fans out an alias array to two map entries pointing at the same id', () => {
    const reg: CommandRegistry = { 'nav.next': { label: 'Next', keys: ['j', 'down'] } }
    const { lookup } = buildDispatch(reg)
    expect(lookup.get('j')).toBe('nav.next')
    expect(lookup.get('down')).toBe('nav.next')
  })

  it('throws on duplicate key within the same scope, naming both ids', () => {
    const reg: CommandRegistry = {
      'a.one': { label: 'One', keys: ['x'] },
      'a.two': { label: 'Two', keys: ['x'] },
    }
    expect(() => buildDispatch(reg)).toThrowError(/a\.one.*a\.two|a\.two.*a\.one/)
  })

  it('does NOT throw when the same key is used in two different scopes', () => {
    const reg: CommandRegistry = {
      'a.global': { label: 'G', keys: ['enter'], scope: 'global' },
      'a.list': { label: 'L', keys: ['enter'], scope: 'list' },
    }
    expect(() => buildDispatch(reg)).not.toThrow()
  })

  it('throws on malformed keys', () => {
    expect(() => buildDispatch({ a: { label: 'A', keys: [''] } })).toThrow()
    expect(() => buildDispatch({ a: { label: 'A', keys: ['mod+'] } })).toThrow()
    expect(() => buildDispatch({ a: { label: 'A', keys: ['g>'] } })).toThrow()
  })

  it('throws when a bare key is the prefix of another command chord (g vs g>g)', () => {
    const reg: CommandRegistry = {
      'a.bare': { label: 'Bare', keys: ['g'] },
      'a.chord': { label: 'Chord', keys: ['g>g'] },
    }
    expect(() => buildDispatch(reg)).toThrow()
  })

  it('produces no map entry and no scope contribution for keys: []', () => {
    const reg: CommandRegistry = { 'a.empty': { label: 'Empty', keys: [] } }
    const { lookup, globalKeys, listKeys, useKeyKeys } = buildDispatch(reg)
    expect(lookup.size).toBe(0)
    expect(globalKeys).toBe('')
    expect(listKeys).toBe('')
    expect(useKeyKeys).toBe('')
  })

  it('routes list-scope keys into the list bucket and global into global', () => {
    const reg: CommandRegistry = {
      'a.g': { label: 'G', keys: ['j'], scope: 'global' },
      'a.l': { label: 'L', keys: ['left'], scope: 'list' },
    }
    const { globalKeys, listKeys } = buildDispatch(reg)
    expect(globalKeys.split(',').map((s) => s.trim())).toContain('j')
    expect(listKeys.split(',').map((s) => s.trim())).toContain('left')
    expect(globalKeys).not.toContain('left')
    expect(listKeys).not.toContain('j')
  })

  it('routes useKey commands into the useKey bucket (kept out of global)', () => {
    const reg: CommandRegistry = {
      'a.search': { label: 'Search', keys: ['/'], useKey: true },
      'a.next': { label: 'Next', keys: ['j'] },
    }
    const { useKeyKeys, globalKeys, lookup } = buildDispatch(reg)
    expect(useKeyKeys.split(',').map((s) => s.trim())).toContain('slash')
    expect(globalKeys).not.toContain('slash')
    expect(lookup.get('slash')).toBe('a.search')
  })

  it('applies overrides over the registry keys', () => {
    const reg: CommandRegistry = { 'a.next': { label: 'Next', keys: ['j'] } }
    const { lookup } = buildDispatch(reg, { 'a.next': ['n'] })
    expect(lookup.get('n')).toBe('a.next')
    expect(lookup.has('j')).toBe(false)
  })

  it('normalizes modifier order so meta+shift+r and shift+meta+r collide', () => {
    const reg: CommandRegistry = {
      'a.one': { label: 'One', keys: ['meta+shift+r'] },
      'a.two': { label: 'Two', keys: ['shift+meta+r'] },
    }
    expect(() => buildDispatch(reg)).toThrow()
  })

  it('registers a chord by prefix + completion key and records the sequence in chords', () => {
    const reg: CommandRegistry = {
      'a.gg': { label: 'Top', keys: ['g>g'] },
      'a.gn': { label: 'New', keys: ['g>n'] },
    }
    const { globalKeys, chords, chordPrefixes, lookup } = buildDispatch(reg)
    // react-hotkeys-hook has no native sequences, so BOTH the prefix `g` and
    // every completion key (`g`, `n`) are registered; the engine resolves the
    // two-step sequence itself.
    const registered = new Set(globalKeys.split(',').map((s) => s.trim()))
    expect(registered.has('g')).toBe(true)
    expect(registered.has('n')).toBe(true)
    expect(chordPrefixes.has('g')).toBe(true)
    expect(chords.get('g')?.get('g')).toBe('a.gg')
    expect(chords.get('g')?.get('n')).toBe('a.gn')
    // Chord commands are NOT in the single-combo lookup.
    expect(lookup.size).toBe(0)
  })

  it('canonicalises ? to shift+slash so it does not duplicate-throw with shift+slash for the same id', () => {
    const reg: CommandRegistry = {
      'a.help': { label: 'Help', keys: ['?', 'shift+slash'], useKey: true },
    }
    expect(() => buildDispatch(reg)).not.toThrow()
    const { lookup } = buildDispatch(reg)
    expect(lookup.get('shift+slash')).toBe('a.help')
  })
})
