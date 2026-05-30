import { describe, it, expect } from 'vitest'
import { JOBS_TOKENS } from '@/config/tokens/jobs.tokens'
import fs from 'fs'
import path from 'path'

// Read the source file to verify no raw single-char shortcut strings remain hardcoded.
// This test is a static analysis check to enforce AC-12: all shortcut characters must
// come from JOBS_TOKENS, not be hardcoded literal strings.
const SOURCE_PATH = path.resolve(__dirname, '../useJobKeyboard.ts')
const source = fs.readFileSync(SOURCE_PATH, 'utf-8')

// Single-char shortcuts that must NOT appear as bare string literals
// (e.g. useHotkeys('s', ...) should be useHotkeys(JOBS_TOKENS['jobs.save'].shortcut, ...))
const DISALLOWED_RAW_SHORTCUTS = ['s', 'd', 'a', 'i', 'o', 'r', 'u']

describe('useJobKeyboard token compliance', () => {
  it('all single-char status shortcut bindings come from JOBS_TOKENS, not hardcoded strings', () => {
    for (const ch of DISALLOWED_RAW_SHORTCUTS) {
      // Match patterns like useHotkeys('s', or useHotkeys("s",
      // but NOT inside JOBS_TOKENS references or comments
      const rawPattern = new RegExp(`useHotkeys\\(['"]${ch}['"]\\s*,`)
      expect(
        rawPattern.test(source),
        `Found hardcoded shortcut '${ch}' in useJobKeyboard.ts — must come from JOBS_TOKENS`
      ).toBe(false)
    }
  })

  it('JOBS_TOKENS contains all required action shortcuts', () => {
    expect(JOBS_TOKENS['jobs.save'].shortcut).toBe('s')
    expect(JOBS_TOKENS['jobs.dismiss'].shortcut).toBe('d')
    expect(JOBS_TOKENS['jobs.apply'].shortcut).toBe('a')
    expect(JOBS_TOKENS['jobs.interviewing'].shortcut).toBe('i')
    expect(JOBS_TOKENS['jobs.offer'].shortcut).toBe('o')
    expect(JOBS_TOKENS['jobs.rejected'].shortcut).toBe('r')
    expect(JOBS_TOKENS['jobs.undo'].shortcut).toBe('u')
  })

  it('JOBS_TOKENS contains navigation and chord shortcuts', () => {
    expect(JOBS_TOKENS['jobs.nav.next'].shortcut).toBe('j')
    expect(JOBS_TOKENS['jobs.nav.prev'].shortcut).toBe('k')
    expect(JOBS_TOKENS['jobs.nav.first-row'].shortcut).toBe('g>g')
    expect(JOBS_TOKENS['jobs.nav.last-row'].shortcut).toBe('shift+g')
  })
})
