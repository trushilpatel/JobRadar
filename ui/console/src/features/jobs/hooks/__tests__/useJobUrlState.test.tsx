import { describe, it, expect, vi } from 'vitest'
import { renderHook, act, waitFor } from '@testing-library/react'
import { withNuqsTestingAdapter } from 'nuqs/adapters/testing'
import { useJobUrlState } from '@/features/jobs/hooks/useJobUrlState'

function makeWrapper(initialSearch = '', onUrlUpdate?: (e: { queryString: string }) => void) {
  return withNuqsTestingAdapter({
    searchParams: initialSearch,
    hasMemory: true,
    onUrlUpdate,
  })
}

describe('useJobUrlState', () => {
  it('returns the documented defaults when no URL state is present', () => {
    const { result } = renderHook(() => useJobUrlState(), { wrapper: makeWrapper() })
    expect(result.current.tab).toBe('new')
    expect(result.current.openJobId).toBeNull()
    expect(result.current.q).toBe('')
    expect(result.current.companies).toEqual([])
    expect(result.current.loc).toBe('any')
    expect(result.current.exp).toEqual([])
    expect(result.current.since).toBe('all')
    expect(result.current.sort).toBe('')
    expect(result.current.dir).toBe('asc')
    expect(result.current.cursor).toBe('')
    expect(result.current.limit).toBe(20)
  })

  it('hydrates state from a populated query string', () => {
    const { result } = renderHook(() => useJobUrlState(), {
      wrapper: makeWrapper(
        '?tab=saved&job=abc-123&q=engineer&companies=stripe,vercel&loc=remote&exp=2-5&since=7d&sort=title&dir=desc&cursor=cur1&limit=50'
      ),
    })
    expect(result.current.tab).toBe('saved')
    expect(result.current.openJobId).toBe('abc-123')
    expect(result.current.q).toBe('engineer')
    expect(result.current.companies).toEqual(['stripe', 'vercel'])
    expect(result.current.loc).toBe('remote')
    expect(result.current.exp).toEqual(['2-5'])
    expect(result.current.since).toBe('7d')
    expect(result.current.sort).toBe('title')
    expect(result.current.dir).toBe('desc')
    expect(result.current.cursor).toBe('cur1')
    expect(result.current.limit).toBe(50)
  })

  it('setTab updates the URL and clears job + cursor', async () => {
    const onUrlUpdate = vi.fn()
    const { result } = renderHook(() => useJobUrlState(), {
      wrapper: makeWrapper('?tab=new&job=abc&cursor=cur1', onUrlUpdate),
    })
    act(() => {
      result.current.setTab('saved')
    })
    await waitFor(() => expect(onUrlUpdate).toHaveBeenCalled())
    await waitFor(() => expect(result.current.tab).toBe('saved'))
    expect(result.current.openJobId).toBeNull()
    expect(result.current.cursor).toBe('')
  })

  it('setOpenJobId writes the id and clearing it removes the param', async () => {
    const { result } = renderHook(() => useJobUrlState(), { wrapper: makeWrapper() })
    await act(async () => {
      await result.current.setOpenJobId('job-7')
    })
    expect(result.current.openJobId).toBe('job-7')
    await act(async () => {
      await result.current.setOpenJobId(null)
    })
    expect(result.current.openJobId).toBeNull()
  })

  it('array filters (companies, exp) round-trip values', async () => {
    const { result } = renderHook(() => useJobUrlState(), { wrapper: makeWrapper() })
    act(() => {
      result.current.setCompanies(['stripe', 'anthropic'])
      result.current.setExp(['0-2', '2-5'])
    })
    await waitFor(() => expect(result.current.companies).toEqual(['stripe', 'anthropic']))
    expect(result.current.exp).toEqual(['0-2', '2-5'])
  })

  it('resetFilters clears every filter but preserves tab and openJobId', async () => {
    const { result } = renderHook(() => useJobUrlState(), {
      wrapper: makeWrapper(
        '?tab=applied&job=keep-me&q=hello&companies=x,y&loc=remote&exp=5%2B&since=24h&sort=title&dir=desc&cursor=cur1'
      ),
    })
    expect(result.current.tab).toBe('applied')
    expect(result.current.openJobId).toBe('keep-me')

    act(() => {
      result.current.resetFilters()
    })

    await waitFor(() => expect(result.current.q).toBe(''))
    expect(result.current.companies).toEqual([])
    expect(result.current.loc).toBe('any')
    expect(result.current.exp).toEqual([])
    expect(result.current.since).toBe('all')
    expect(result.current.sort).toBe('')
    expect(result.current.dir).toBe('asc')
    expect(result.current.cursor).toBe('')
    // Preserved:
    expect(result.current.tab).toBe('applied')
    expect(result.current.openJobId).toBe('keep-me')
  })

  it('setting limit clears the cursor (page must restart at 1)', async () => {
    const { result } = renderHook(() => useJobUrlState(), {
      wrapper: makeWrapper('?cursor=abc'),
    })
    expect(result.current.cursor).toBe('abc')
    act(() => {
      result.current.setLimit(50)
    })
    await waitFor(() => expect(result.current.limit).toBe(50))
    expect(result.current.cursor).toBe('')
  })
})
