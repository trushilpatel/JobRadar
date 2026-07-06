import { describe, it, expect } from 'vitest'
import { matchResume, pickBestResume } from '../resumeJobMatch'

describe('matchResume', () => {
  it('counts case-insensitive whole-word matches', () => {
    const result = matchResume(
      ['Go', 'PostgreSQL'],
      'Experienced backend engineer using go and PostgreSQL daily.'
    )
    expect(result).toEqual({ matched: 2, total: 2 })
  })

  it('does not match a keyword as a substring of a longer word', () => {
    const result = matchResume(['Go'], 'I use Google Cloud extensively.')
    expect(result).toEqual({ matched: 0, total: 1 })
  })

  it('matches multi-word keywords as a whole phrase', () => {
    const result = matchResume(['machine learning'], 'Background in machine learning and NLP.')
    expect(result).toEqual({ matched: 1, total: 1 })
  })

  it('correctly escapes regex special characters in keywords like C++ and Node.js', () => {
    const result = matchResume(
      ['C++', 'Node.js'],
      'Built services in C++ and Node.js for five years.'
    )
    expect(result).toEqual({ matched: 2, total: 2 })
  })

  it('returns total 0 for an empty keyword list', () => {
    expect(matchResume([], 'some resume text')).toEqual({ matched: 0, total: 0 })
  })

  it('returns matched 0 for empty resume text', () => {
    expect(matchResume(['Go'], '')).toEqual({ matched: 0, total: 1 })
  })
})

describe('pickBestResume', () => {
  const resumes = [
    { id: 'r1', label: 'Weak', extractedText: 'Go and Python developer.' },
    { id: 'r2', label: 'Strong', extractedText: 'Go, PostgreSQL, and Kubernetes expert.' },
  ]

  it('picks the resume with the highest match count', () => {
    const best = pickBestResume(['Go', 'PostgreSQL', 'Kubernetes'], resumes)
    expect(best).toEqual({ id: 'r2', label: 'Strong' })
  })

  it('returns null when there are no job keywords', () => {
    expect(pickBestResume([], resumes)).toBeNull()
  })

  it('returns null when there are no resumes', () => {
    expect(pickBestResume(['Go'], [])).toBeNull()
  })

  it('returns null when every resume scores zero', () => {
    expect(pickBestResume(['Rust'], resumes)).toBeNull()
  })

  it('breaks ties by keeping the first-encountered (oldest-uploaded) resume', () => {
    const tied = [
      { id: 'r-first', label: 'First', extractedText: 'Go and PostgreSQL.' },
      { id: 'r-second', label: 'Second', extractedText: 'Go and PostgreSQL.' },
    ]
    expect(pickBestResume(['Go', 'PostgreSQL'], tied)).toEqual({ id: 'r-first', label: 'First' })
  })
})
