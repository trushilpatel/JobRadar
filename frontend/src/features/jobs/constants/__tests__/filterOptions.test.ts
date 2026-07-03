import { describe, it, expect } from 'vitest'
import { pickSingleSelectValue, EXP_OPTIONS, VISA_OPTIONS } from '../filterOptions'

describe('pickSingleSelectValue', () => {
  it('switches to a newly clicked value that differs from current', () => {
    expect(pickSingleSelectValue(['remote'], 'any', 'any')).toBe('remote')
  })

  it('switches directly from one active value to a different one', () => {
    expect(pickSingleSelectValue(['hybrid'], 'remote', 'any')).toBe('hybrid')
  })

  it('clears to the fallback when the only entry is the current value (toggle off)', () => {
    // FilterPopover toggle logic removes the current value from `next` when
    // clicked again, leaving an array that no longer contains it — but if a
    // caller passes an array that STILL contains current alongside nothing
    // else different, there's no "chosen" entry different from current, so
    // fallback applies.
    expect(pickSingleSelectValue(['remote'], 'remote', 'any')).toBe('any')
  })

  it('falls back when next is an empty array', () => {
    expect(pickSingleSelectValue([], 'remote', 'any')).toBe('any')
  })

  it('picks the first differing value when next has multiple entries', () => {
    expect(pickSingleSelectValue(['remote', 'hybrid'], 'remote', 'any')).toBe('hybrid')
  })
})

describe('EXP_OPTIONS', () => {
  it('carries value/label pairs including the "5+ yrs" label', () => {
    expect(EXP_OPTIONS).toEqual([
      { value: '0-2', label: '0-2 yrs' },
      { value: '2-5', label: '2-5 yrs' },
      { value: '5+', label: '5+ yrs' },
    ])
  })
})

describe('VISA_OPTIONS', () => {
  it('defines sponsorship offered/not-offered options', () => {
    expect(VISA_OPTIONS).toEqual([
      { value: 'true', label: 'Sponsorship offered' },
      { value: 'false', label: 'No sponsorship' },
    ])
  })
})
