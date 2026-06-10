import { describe, it, expect, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { ToggleGroup, ToggleGroupItem } from '@/components/ui/toggle-group'

describe('ToggleGroup', () => {
  it('renders items and reflects the selected value', () => {
    render(
      <ToggleGroup type="single" value="a" onValueChange={() => {}}>
        <ToggleGroupItem value="a" aria-label="Option A">
          A
        </ToggleGroupItem>
        <ToggleGroupItem value="b" aria-label="Option B">
          B
        </ToggleGroupItem>
      </ToggleGroup>
    )
    expect(screen.getByRole('radio', { name: 'Option A' })).toHaveAttribute('data-state', 'on')
    expect(screen.getByRole('radio', { name: 'Option B' })).toHaveAttribute('data-state', 'off')
  })

  it('fires onValueChange when an item is clicked', async () => {
    const onValueChange = vi.fn()
    render(
      <ToggleGroup type="single" value="a" onValueChange={onValueChange}>
        <ToggleGroupItem value="a" aria-label="Option A">
          A
        </ToggleGroupItem>
        <ToggleGroupItem value="b" aria-label="Option B">
          B
        </ToggleGroupItem>
      </ToggleGroup>
    )
    await userEvent.click(screen.getByRole('radio', { name: 'Option B' }))
    expect(onValueChange).toHaveBeenCalledWith('b')
  })

  it('inherits variant/size from the group context', () => {
    render(
      <ToggleGroup type="single" variant="outline" size="sm">
        <ToggleGroupItem value="a" aria-label="Option A">
          A
        </ToggleGroupItem>
      </ToggleGroup>
    )
    const item = screen.getByRole('radio', { name: 'Option A' })
    expect(item.className).toContain('border-[var(--border)]')
    expect(item.className).toContain('h-9')
  })

  it('falls back to the item-level variant/size when the context is unset', () => {
    render(
      <ToggleGroup type="single">
        <ToggleGroupItem value="a" aria-label="Option A" variant="outline" size="lg">
          A
        </ToggleGroupItem>
      </ToggleGroup>
    )
    const item = screen.getByRole('radio', { name: 'Option A' })
    expect(item.className).toContain('border-[var(--border)]')
    expect(item.className).toContain('h-11')
  })
})
