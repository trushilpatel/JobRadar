import { describe, it, expect } from 'vitest'
import { matchResume, matchResumeDetailed, pickBestResume } from '../resumeJobMatch'

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

describe('matchResumeDetailed', () => {
  it('partitions keywords into matched and missing', () => {
    const result = matchResumeDetailed(
      ['Go', 'PostgreSQL', 'Kubernetes'],
      'Backend engineer using Go and PostgreSQL daily.'
    )
    expect(result).toEqual({
      matchedKeywords: ['Go', 'PostgreSQL'],
      missingKeywords: ['Kubernetes'],
    })
  })

  it('preserves input order within each partition', () => {
    const result = matchResumeDetailed(
      ['Kubernetes', 'Go', 'Rust', 'PostgreSQL'],
      'Go and PostgreSQL developer.'
    )
    expect(result).toEqual({
      matchedKeywords: ['Go', 'PostgreSQL'],
      missingKeywords: ['Kubernetes', 'Rust'],
    })
  })

  it('returns all-missing for empty resume text', () => {
    const result = matchResumeDetailed(['Go', 'PostgreSQL'], '')
    expect(result).toEqual({ matchedKeywords: [], missingKeywords: ['Go', 'PostgreSQL'] })
  })

  it('returns empty partitions for an empty keyword list', () => {
    expect(matchResumeDetailed([], 'some resume text')).toEqual({
      matchedKeywords: [],
      missingKeywords: [],
    })
  })

  it("agrees with matchResume's counts", () => {
    const jobKeywords = ['Go', 'PostgreSQL', 'Kubernetes']
    const resumeText = 'Go and PostgreSQL developer.'
    const { matched, total } = matchResume(jobKeywords, resumeText)
    const { matchedKeywords, missingKeywords } = matchResumeDetailed(jobKeywords, resumeText)
    expect(matchedKeywords.length).toBe(matched)
    expect(matchedKeywords.length + missingKeywords.length).toBe(total)
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
