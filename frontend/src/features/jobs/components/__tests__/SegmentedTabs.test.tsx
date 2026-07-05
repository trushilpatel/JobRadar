import { describe, it, expect, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { SegmentedTabs } from '../SegmentedTabs'

const ITEMS = [
  { key: 'new', label: 'New' },
  { key: 'saved', label: 'Saved', count: 3 },
]

describe('SegmentedTabs', () => {
  it('renders every item label', () => {
    render(<SegmentedTabs items={ITEMS} value="new" onChange={() => {}} />)
    expect(screen.getByRole('tab', { name: /New/ })).toBeInTheDocument()
    expect(screen.getByRole('tab', { name: /Saved/ })).toBeInTheDocument()
  })

  it('marks the active item as selected', () => {
    render(<SegmentedTabs items={ITEMS} value="saved" onChange={() => {}} />)
    expect(screen.getByRole('tab', { name: /Saved/ })).toHaveAttribute('aria-selected', 'true')
    expect(screen.getByRole('tab', { name: /New/ })).toHaveAttribute('aria-selected', 'false')
  })

  it('calls onChange with the clicked item key', async () => {
    const onChange = vi.fn()
    render(<SegmentedTabs items={ITEMS} value="new" onChange={onChange} />)
    await userEvent.click(screen.getByRole('tab', { name: /Saved/ }))
    expect(onChange).toHaveBeenCalledWith('saved')
  })

  it('renders a count badge when provided, formatting large counts as 999+', () => {
    const items = [
      { key: 'new', label: 'New', count: 5 },
      { key: 'saved', label: 'Saved', count: 1500 },
    ]
    render(<SegmentedTabs items={items} value="new" onChange={() => {}} />)
    expect(screen.getByText('5')).toBeInTheDocument()
    expect(screen.getByText('999+')).toBeInTheDocument()
  })

  it('omits the count badge when count is undefined', () => {
    render(<SegmentedTabs items={ITEMS} value="new" onChange={() => {}} />)
    expect(screen.getByRole('tab', { name: 'New' }).textContent).toBe('New')
  })

  it('uses a custom render override instead of a plain button when provided', () => {
    const onSelectSpy = vi.fn()
    const items = [
      { key: 'new', label: 'New' },
      {
        key: 'tracking',
        label: 'Tracking',
        render: ({ active, onSelect }: { active: boolean; onSelect: () => void }) => (
          <button type="button" data-testid="custom-tracking" onClick={onSelect}>
            {active ? 'Tracking (active)' : 'Tracking'}
          </button>
        ),
      },
    ]
    render(<SegmentedTabs items={items} value="tracking" onChange={onSelectSpy} />)
    const custom = screen.getByTestId('custom-tracking')
    expect(custom).toHaveTextContent('Tracking (active)')
    // The override owns click handling — clicking it should NOT auto-invoke
    // the top-level onChange; only the override's own onSelect() call does.
  })

  it("calls the custom render override's onSelect through to onChange", async () => {
    const onChange = vi.fn()
    const items = [
      {
        key: 'tracking',
        label: 'Tracking',
        render: ({ onSelect }: { active: boolean; onSelect: () => void }) => (
          <button type="button" data-testid="custom-tracking" onClick={onSelect}>
            Tracking
          </button>
        ),
      },
    ]
    render(<SegmentedTabs items={items} value="new" onChange={onChange} />)
    await userEvent.click(screen.getByTestId('custom-tracking'))
    expect(onChange).toHaveBeenCalledWith('tracking')
  })
})
