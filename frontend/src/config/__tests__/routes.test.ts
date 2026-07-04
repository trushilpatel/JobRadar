import { describe, it, expect } from 'vitest'
import { routes } from '@/config/routes'

describe('routes — static top-level', () => {
  it.each([
    [routes.account(), '/account'],
    [routes.login(), '/login'],
    [routes.resumeLibrary(), '/resumes'],
    [routes.adminCompanies(), '/admin/companies'],
    [routes.adminUsers(), '/admin/users'],
    [routes.adminAudit(), '/admin/audit'],
    [routes.adminPipeline(), '/admin/pipeline'],
  ])('returns %s', (actual, expected) => {
    expect(actual).toBe(expected)
  })
})

describe('routes.jobs — query composition', () => {
  it('returns the bare path with no params', () => {
    expect(routes.jobs()).toBe('/jobs')
  })

  it('composes a single query param', () => {
    expect(routes.jobs({ since: '24h' })).toBe('/jobs?since=24h')
  })

  it('composes multiple query params', () => {
    expect(routes.jobs({ tab: 'saved', since: '7d' })).toBe('/jobs?tab=saved&since=7d')
  })

  it('drops undefined / empty values', () => {
    expect(routes.jobs({ job: undefined, tab: '', since: '30d' })).toBe('/jobs?since=30d')
  })
})

describe('routes.jobOpen', () => {
  it('always sets the job param', () => {
    expect(routes.jobOpen('abc')).toBe('/jobs?job=abc')
  })

  it('merges additional params with the job id', () => {
    expect(routes.jobOpen('abc', { tab: 'new' })).toBe('/jobs?tab=new&job=abc')
  })
})

describe('routes — id-based builders', () => {
  it('jobMobileRoute embeds the id', () => {
    expect(routes.jobMobileRoute('xyz')).toBe('/jobs/xyz')
  })
})
