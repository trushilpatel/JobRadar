import type { KeySpec } from './types'
import { isMac } from './platform'

/**
 * Maps a single normalized token to its display glyph. Modifiers become
 * glyphs; lowercase letters stay as typed. `mod` resolves per-platform.
 */
function glyphForToken(token: string): string {
  switch (token) {
    case 'mod':
      return isMac() ? '⌘' : 'Ctrl'
    case 'meta':
    case 'cmd':
    case 'command':
      return '⌘'
    case 'ctrl':
    case 'control':
      return 'Ctrl'
    case 'alt':
    case 'option':
      return isMac() ? '⌥' : 'Alt'
    case 'shift':
      return '⇧'
    case 'enter':
    case 'return':
      return '⏎'
    case 'escape':
    case 'esc':
      return 'esc'
    case 'left':
      return '←'
    case 'right':
      return '→'
    case 'up':
      return '↑'
    case 'down':
      return '↓'
    case 'slash':
      return '/'
    case 'space':
      return '␣'
    default:
      return token
  }
}

/**
 * Renders a key spec into display glyph groups — one inner array per chord
 * step (chords split on `>`), each containing the rendered tokens for that
 * step. Returning `string[][]` lets the help sheet and palette reuse their
 * existing chord-group rendering.
 */
export function formatKeys(spec: KeySpec): string[][] {
  return spec
    .split('>')
    .map((chord) => chord.split('+').map((token) => glyphForToken(token.trim().toLowerCase())))
}
