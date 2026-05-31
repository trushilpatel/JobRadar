import { describe, it, expect, afterEach, vi } from 'vitest'
import { formatKeys } from '../formatKeys'

function stubPlatform(mac: boolean) {
  Object.defineProperty(window.navigator, 'platform', {
    value: mac ? 'MacIntel' : 'Win32',
    configurable: true,
  })
}

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('formatKeys', () => {
  it('renders mod+enter as ⌘ ⏎ on Mac', () => {
    stubPlatform(true)
    expect(formatKeys('mod+enter')).toEqual([['⌘', '⏎']])
  })

  it('renders mod+enter as Ctrl ⏎ on non-Mac', () => {
    stubPlatform(false)
    expect(formatKeys('mod+enter')).toEqual([['Ctrl', '⏎']])
  })

  it('splits a chord g>g into two groups', () => {
    stubPlatform(true)
    expect(formatKeys('g>g')).toEqual([['g'], ['g']])
  })

  it('renders shift+slash and ? with expected glyphs', () => {
    stubPlatform(true)
    expect(formatKeys('shift+slash')).toEqual([['⇧', '/']])
    expect(formatKeys('?')).toEqual([['?']])
  })

  it('renders arrows and escape glyphs', () => {
    stubPlatform(true)
    expect(formatKeys('left')).toEqual([['←']])
    expect(formatKeys('right')).toEqual([['→']])
    expect(formatKeys('up')).toEqual([['↑']])
    expect(formatKeys('down')).toEqual([['↓']])
    expect(formatKeys('escape')).toEqual([['esc']])
  })

  it('renders shift+g keeping the lowercase letter as typed', () => {
    stubPlatform(true)
    expect(formatKeys('shift+g')).toEqual([['⇧', 'g']])
  })

  it('renders meta/ctrl/alt explicitly across platforms', () => {
    stubPlatform(true)
    expect(formatKeys('meta+k')).toEqual([['⌘', 'k']])
    expect(formatKeys('alt+1')).toEqual([['⌥', '1']])
    stubPlatform(false)
    expect(formatKeys('meta+k')).toEqual([['⌘', 'k']])
    expect(formatKeys('ctrl+k')).toEqual([['Ctrl', 'k']])
    expect(formatKeys('alt+1')).toEqual([['Alt', '1']])
  })
})
