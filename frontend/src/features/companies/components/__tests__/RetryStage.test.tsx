import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, fireEvent, within } from '@testing-library/react'
import type { RetryTierCounts } from '@/types'
import { useRetryTier } from '@/features/companies/hooks/useRetryTier'
import { RetryStage } from '../RetryStage'

vi.mock('@/features/companies/hooks/useRetryTier')

const mutate = vi.fn()

function mockHook(isPending = false) {
  vi.mocked(useRetryTier).mockReturnValue({
    mutate,
    isPending,
  } as unknown as ReturnType<typeof useRetryTier>)
}

const counts = (over: Partial<RetryTierCounts> = {}): RetryTierCounts => ({
  '1m': 0,
  '10m': 0,
  '1h': 0,
  dlq: 0,
  ...over,
})

beforeEach(() => {
  vi.clearAllMocks()
  mockHook()
})

describe('RetryStage — total badge', () => {
  it('renders 0 with muted styling when all tiers are empty', () => {
    render(<RetryStage label="Filter Retrying" worker="filter" counts={counts()} />)
    const total = screen.getByText('0')
    expect(total).toBeInTheDocument()
    expect(total.className).toContain('text-white/25')
  })

  it('sums all tiers and uses the active (amber) styling when non-zero', () => {
    render(
      <RetryStage
        label="Filter Retrying"
        worker="filter"
        counts={counts({ '1m': 1, '10m': 2, '1h': 3, dlq: 4 })}
      />
    )
    const total = screen.getByText('10')
    expect(total.className).toContain('text-amber-400')
  })

  it('splits a multi-word label into stacked word spans', () => {
    render(<RetryStage label="Filter Retrying" worker="filter" counts={counts()} />)
    expect(screen.getByText('Filter')).toBeInTheDocument()
    expect(screen.getByText('Retrying')).toBeInTheDocument()
  })
})

describe('RetryStage — tooltip retry buttons', () => {
  function openTooltip() {
    // Radix Tooltip opens on focus of the trigger.
    const trigger = screen.getByText('Filter').closest('div[class*="cursor-default"]')!
    fireEvent.focus(trigger)
  }

  it('fires retry mutation for a tier that has jobs', async () => {
    render(<RetryStage label="Filter Retrying" worker="filter" counts={counts({ '1m': 2 })} />)
    openTooltip()

    // Radix renders the tooltip content twice (visible + a11y mirror), so there
    // are two Retry buttons per tier. The first four map to one rendered copy.
    const retryButtons = await screen.findAllByRole('button', { name: 'Retry' })
    expect(retryButtons.length % 4).toBe(0)

    // The first tier (1m) has 2 jobs → enabled.
    expect(retryButtons[0]).not.toBeDisabled()
    fireEvent.click(retryButtons[0]!)
    expect(mutate).toHaveBeenCalledWith({ worker: 'filter', tier: '1m' })
  })

  it('disables retry buttons for empty tiers', async () => {
    render(<RetryStage label="Filter Retrying" worker="filter" counts={counts({ '1m': 5 })} />)
    openTooltip()

    const retryButtons = await screen.findAllByRole('button', { name: 'Retry' })
    // 1m enabled, the other three (10m/1h/dlq) are zero → disabled.
    expect(retryButtons[1]).toBeDisabled()
    expect(retryButtons[2]).toBeDisabled()
    expect(retryButtons[3]).toBeDisabled()
  })

  it('disables every retry button while a retry is pending', async () => {
    mockHook(true)
    render(
      <RetryStage
        label="Enricher Retrying"
        worker="enricher"
        counts={counts({ '1m': 5, dlq: 3 })}
      />
    )
    const trigger = screen.getByText('Enricher').closest('div[class*="cursor-default"]')!
    fireEvent.focus(trigger)

    const retryButtons = await screen.findAllByRole('button', { name: 'Retry' })
    retryButtons.forEach((b) => expect(b).toBeDisabled())
    fireEvent.click(retryButtons[0]!)
    expect(mutate).not.toHaveBeenCalled()
  })

  it('shows the per-tier counts inside the tooltip', async () => {
    render(
      <RetryStage
        label="Filter Retrying"
        worker="filter"
        counts={counts({ '1m': 1, '10m': 0, '1h': 0, dlq: 9 })}
      />
    )
    openTooltip()

    const content = (await screen.findAllByRole('button', { name: 'Retry' }))[0]!.closest(
      'div[class*="space-y"]'
    ) as HTMLElement
    // Scoped to a single rendered tooltip copy, each tier label/count is unique.
    expect(within(content).getByText('DLQ')).toBeInTheDocument()
    expect(within(content).getByText('9')).toBeInTheDocument()
    expect(within(content).getAllByText('1m').length).toBeGreaterThanOrEqual(1)
  })
})
