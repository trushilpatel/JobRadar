import { describe, it, expect, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { SegmentedSwitch, type SegmentedOption } from '@/components/ui/segmented-switch'

type Tab = 'all' | 'saved' | 'applied'

const OPTIONS: SegmentedOption<Tab>[] = [
  { value: 'all', label: 'All' },
  { value: 'saved', label: 'Saved', count: 3, accent: 'violet' },
  { value: 'applied', label: 'Applied', accent: 'emerald' },
]

describe('SegmentedSwitch', () => {
  it('renders all option labels', () => {
    render(<SegmentedSwitch options={OPTIONS} value="all" onChange={() => {}} />)
    expect(screen.getByText('All')).toBeInTheDocument()
    expect(screen.getByText('Saved')).toBeInTheDocument()
    expect(screen.getByText('Applied')).toBeInTheDocument()
  })

  it('renders option count badges when provided', () => {
    render(<SegmentedSwitch options={OPTIONS} value="all" onChange={() => {}} />)
    expect(screen.getByText('3')).toBeInTheDocument()
  })

  it('marks the active option with data-state="on"', () => {
    render(<SegmentedSwitch options={OPTIONS} value="saved" onChange={() => {}} />)
    const items = screen.getAllByRole('radio')
    const allItem = items.find((el) => el.textContent?.includes('All'))
    const savedItem = items.find((el) => el.textContent?.includes('Saved'))
    expect(allItem).toHaveAttribute('data-state', 'off')
    expect(savedItem).toHaveAttribute('data-state', 'on')
  })

  it('fires onChange with the clicked option value', async () => {
    const onChange = vi.fn()
    render(<SegmentedSwitch options={OPTIONS} value="all" onChange={onChange} />)
    await userEvent.click(screen.getByText('Saved'))
    expect(onChange).toHaveBeenCalledOnce()
    expect(onChange).toHaveBeenCalledWith('saved')
  })

  it('does not fire onChange when the already-active option is clicked', async () => {
    const onChange = vi.fn()
    render(<SegmentedSwitch options={OPTIONS} value="all" onChange={onChange} />)
    // Click the currently active option — ToggleGroup fires '' which we discard
    await userEvent.click(screen.getByText('All'))
    expect(onChange).not.toHaveBeenCalled()
  })

  it('applies the aria-label to the group element', () => {
    render(
      <SegmentedSwitch
        options={OPTIONS}
        value="all"
        onChange={() => {}}
        aria-label="Job status filter"
      />
    )
    // Radix ToggleGroup type="single" renders role="group" (not radiogroup)
    expect(screen.getByRole('group', { name: 'Job status filter' })).toBeInTheDocument()
  })

  it('renders with a single option', () => {
    const single: SegmentedOption<'only'>[] = [{ value: 'only', label: 'Only' }]
    render(<SegmentedSwitch options={single} value="only" onChange={() => {}} />)
    expect(screen.getByText('Only')).toBeInTheDocument()
  })

  it('disables all options when disabled=true', () => {
    render(<SegmentedSwitch options={OPTIONS} value="all" onChange={() => {}} disabled />)
    const items = screen.getAllByRole('radio')
    for (const item of items) {
      expect(item).toBeDisabled()
    }
  })

  it('renders options with default violet accent when none is specified', () => {
    const opts: SegmentedOption<'a'>[] = [{ value: 'a', label: 'A' }]
    render(<SegmentedSwitch options={opts} value="a" onChange={() => {}} />)
    const item = screen.getByRole('radio')
    // data-[state=on] accent class is violet by default
    expect(item.className).toContain('data-[state=on]:bg-violet-500/20')
  })
})
