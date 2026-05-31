import { describe, it, expect } from 'vitest'
import { routes } from '@/config/routes'

describe('routes — static top-level', () => {
  it.each([
    [routes.dashboard(), '/dashboard'],
    [routes.insights(), '/insights'],
    [routes.settings(), '/settings'],
    [routes.account(), '/account'],
    [routes.login(), '/login'],
    [routes.resumeLibrary(), '/admin/resumes'],
    [routes.applyQueue(), '/admin/apply-queue'],
    [routes.adminCompanies(), '/admin/companies'],
    [routes.adminUsers(), '/admin/users'],
    [routes.adminAudit(), '/admin/audit'],
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

  it('resumeDetail embeds the id', () => {
    expect(routes.resumeDetail('r1')).toBe('/admin/resumes/r1')
  })
})

describe('routes.resumeEdit — param remapping', () => {
  it('returns the bare edit path when no params', () => {
    expect(routes.resumeEdit('r1')).toBe('/admin/resumes/r1/edit')
  })

  it('remaps jobId→job, queue→queue, returnTo→return', () => {
    expect(routes.resumeEdit('r1', { jobId: 'j1', queue: 'all', returnTo: '/jobs' })).toBe(
      '/admin/resumes/r1/edit?job=j1&queue=all&return=%2Fjobs'
    )
  })

  it('drops absent optional params', () => {
    expect(routes.resumeEdit('r1', { jobId: 'j1' })).toBe('/admin/resumes/r1/edit?job=j1')
  })
})

describe('routes.applyQueue — filter params', () => {
  it('composes a state filter', () => {
    expect(routes.applyQueue({ state: 'needs-resume' })).toBe(
      '/admin/apply-queue?state=needs-resume'
    )
  })

  it('composes state + edit', () => {
    expect(routes.applyQueue({ state: 'draft', edit: 'e1' })).toBe(
      '/admin/apply-queue?state=draft&edit=e1'
    )
  })
})
