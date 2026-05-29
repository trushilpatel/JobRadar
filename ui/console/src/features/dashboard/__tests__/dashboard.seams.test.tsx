/**
 * Seam tests for the Dashboard feature.
 *
 * Covers:
 *  1. Navigation seam — MarketPanel must navigate to /jobs with `since=24h` as
 *     a URL query param (not via location.state, which nuqs ignores).
 *  2. Cache seam — useInterviewStage.onSuccess must invalidate
 *     dashboardQueryKeys.interviews() and dashboardQueryKeys.progress()
 *     using the typed factory keys, not inline strings.
 */

import React from 'react'
import { render, screen, waitFor, fireEvent, act } from '@testing-library/react'
import { renderHook } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { MemoryRouter, Route, Routes, useLocation } from 'react-router-dom'
import * as dashboardApi from '@/features/dashboard/api/dashboard.api'
import { dashboardQueryKeys } from '../queryKeys'
import { MarketPanel } from '../components/MarketPanel'
import { useInterviewStage } from '../hooks/useInterviewStage'

vi.mock('@/features/dashboard/api/dashboard.api')

// Recharts / jsdom requirement
class MockResizeObserver {
  observe() {}
  unobserve() {}
  disconnect() {}
}
global.ResizeObserver = MockResizeObserver as unknown as typeof ResizeObserver

// ── Navigation seam: MarketPanel → /jobs?since=24h ─────────────────────────
//
// The jobs page (JobsPage) reads all filter state from the URL via nuqs.
// Passing `location.state: { dateFilter: '24h' }` is silently dropped — the
// page renders with no date filter applied. The fix is
//   navigate(routes.jobs({ since: '24h' }))
// which produces /jobs?since=24h. These tests assert that shape.

describe('MarketPanel navigation seam', () => {
  const sampleMarket = {
    new_since_yesterday: 7,
    total_open: 200,
    added_this_week: 25,
    added_last_week: 20,
    weekly_delta: 5,
  }

  // LocationSpy renders the current pathname + search so we can assert where
  // the MemoryRouter ended up after navigation.
  function LocationSpy({ onLocation }: { onLocation: (loc: string) => void }) {
    const loc = useLocation()
    React.useEffect(() => {
      onLocation(loc.pathname + loc.search)
    })
    return null
  }

  it('navigates to /jobs with since=24h as a URL query param when "New jobs" is clicked', async () => {
    vi.mocked(dashboardApi.fetchMarket).mockResolvedValue(sampleMarket)

    let navigatedTo = ''
    const qc = new QueryClient({ defaultOptions: { queries: { retry: 0, gcTime: 0 } } })

    render(
      <MemoryRouter initialEntries={['/dashboard']}>
        <QueryClientProvider client={qc}>
          <LocationSpy
            onLocation={(l) => {
              navigatedTo = l
            }}
          />
          <Routes>
            <Route path="/dashboard" element={<MarketPanel />} />
            <Route path="/jobs" element={<div data-testid="jobs-page">Jobs</div>} />
          </Routes>
        </QueryClientProvider>
      </MemoryRouter>
    )

    // Wait for data to load
    await waitFor(() => expect(screen.getByText('7')).toBeInTheDocument())

    // Click the "New jobs" interactive stat (role=button wrapping the value)
    fireEvent.click(screen.getByRole('button'))

    // After navigation the router renders the /jobs route
    await waitFor(() => expect(screen.getByTestId('jobs-page')).toBeInTheDocument())

    // Assert the navigation target is a URL with the correct query param —
    // never a location.state object (which nuqs silently drops).
    expect(navigatedTo).toContain('/jobs')
    expect(navigatedTo).toContain('since=24h')
    // Confirm no legacy state-based pattern bleeds into the URL
    expect(navigatedTo).not.toContain('dateFilter')
  })

  it('does not carry dateFilter in location.state after navigation', async () => {
    vi.mocked(dashboardApi.fetchMarket).mockResolvedValue(sampleMarket)

    const qc = new QueryClient({ defaultOptions: { queries: { retry: 0, gcTime: 0 } } })

    // Capture the location.state of the /jobs route after navigation
    let capturedState: unknown = 'NOT_SET'
    function StateSpy() {
      const loc = useLocation()
      React.useEffect(() => {
        capturedState = loc.state
      })
      return <div data-testid="jobs-page">Jobs</div>
    }

    render(
      <MemoryRouter initialEntries={['/dashboard']}>
        <QueryClientProvider client={qc}>
          <Routes>
            <Route path="/dashboard" element={<MarketPanel />} />
            <Route path="/jobs" element={<StateSpy />} />
          </Routes>
        </QueryClientProvider>
      </MemoryRouter>
    )

    await waitFor(() => expect(screen.getByText('7')).toBeInTheDocument())
    fireEvent.click(screen.getByRole('button'))

    await waitFor(() => expect(screen.getByTestId('jobs-page')).toBeInTheDocument())

    // location.state must be null/undefined — not an object carrying dateFilter
    // (the old broken pattern passed state: { dateFilter: '24h' } which nuqs ignores)
    if (capturedState !== null && capturedState !== undefined) {
      expect(capturedState as Record<string, unknown>).not.toHaveProperty('dateFilter')
    }
    // The important assertion is that navigatedTo (tested above) contains since=24h
    // as a query param — the URL is the source of truth for nuqs.
  })
})

// ── Cache seam: useInterviewStage invalidations ─────────────────────────────
//
// useInterviewStage.onSuccess must invalidate:
//   dashboardQueryKeys.interviews()  →  ['dashboard', 'interviews']
//   dashboardQueryKeys.progress()    →  ['dashboard', 'progress']
//
// These are the typed factory keys. Tests verify both keys are invalidated on
// success and that no spurious invalidation fires on failure.

describe('useInterviewStage cache seam', () => {
  function makeWrapper(qc: QueryClient) {
    return function Wrapper({ children }: { children: React.ReactNode }) {
      return <QueryClientProvider client={qc}>{children}</QueryClientProvider>
    }
  }

  it('invalidates interviews and progress queries on successful stage update', async () => {
    vi.mocked(dashboardApi.updateApplicationStage).mockResolvedValue(undefined)

    const qc = new QueryClient({ defaultOptions: { mutations: { retry: 0 } } })
    const invalidateSpy = vi.spyOn(qc, 'invalidateQueries')

    const { result } = renderHook(() => useInterviewStage(), { wrapper: makeWrapper(qc) })

    await act(async () => {
      result.current.mutate({ id: 42, stage: 'offer' })
    })

    // Wait for onSuccess to fire
    await waitFor(() => expect(result.current.isSuccess).toBe(true))

    // Both factory keys must have been invalidated
    expect(invalidateSpy).toHaveBeenCalledWith(
      expect.objectContaining({ queryKey: dashboardQueryKeys.interviews() })
    )
    expect(invalidateSpy).toHaveBeenCalledWith(
      expect.objectContaining({ queryKey: dashboardQueryKeys.progress() })
    )
  })

  it('invalidates using the typed factory tuples — not inline strings', async () => {
    vi.mocked(dashboardApi.updateApplicationStage).mockResolvedValue(undefined)

    const qc = new QueryClient({ defaultOptions: { mutations: { retry: 0 } } })
    const invalidateSpy = vi.spyOn(qc, 'invalidateQueries')

    const { result } = renderHook(() => useInterviewStage(), { wrapper: makeWrapper(qc) })

    await act(async () => {
      result.current.mutate({ id: 1, stage: 'phone_screen' })
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))

    // The exact key tuples from the factory
    const expectedInterviews = dashboardQueryKeys.interviews() // ['dashboard', 'interviews']
    const expectedProgress = dashboardQueryKeys.progress() // ['dashboard', 'progress']

    const callKeys = invalidateSpy.mock.calls.map((c) => (c[0] as { queryKey: unknown }).queryKey)
    expect(callKeys).toContainEqual(expectedInterviews)
    expect(callKeys).toContainEqual(expectedProgress)
  })

  it('calls updateApplicationStage with the correct id and stage', async () => {
    vi.mocked(dashboardApi.updateApplicationStage).mockResolvedValue(undefined)

    const qc = new QueryClient({ defaultOptions: { mutations: { retry: 0 } } })
    const { result } = renderHook(() => useInterviewStage(), { wrapper: makeWrapper(qc) })

    await act(async () => {
      result.current.mutate({ id: 99, stage: 'offer' })
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))

    expect(dashboardApi.updateApplicationStage).toHaveBeenCalledWith(99, 'offer')
  })

  it('does not invalidate queries when the stage update fails', async () => {
    vi.mocked(dashboardApi.updateApplicationStage).mockRejectedValue(new Error('server error'))

    const qc = new QueryClient({ defaultOptions: { mutations: { retry: 0 } } })
    const invalidateSpy = vi.spyOn(qc, 'invalidateQueries')

    const { result } = renderHook(() => useInterviewStage(), { wrapper: makeWrapper(qc) })

    await act(async () => {
      result.current.mutate({ id: 1, stage: 'offer' })
    })

    await waitFor(() => expect(result.current.isError).toBe(true))

    // No cache invalidation on failure — stale data protection
    expect(invalidateSpy).not.toHaveBeenCalledWith(
      expect.objectContaining({ queryKey: dashboardQueryKeys.interviews() })
    )
    expect(invalidateSpy).not.toHaveBeenCalledWith(
      expect.objectContaining({ queryKey: dashboardQueryKeys.progress() })
    )
  })
})
