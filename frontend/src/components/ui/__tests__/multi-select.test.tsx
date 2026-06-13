import { describe, it, expect, vi } from 'vitest'
import { useState } from 'react'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MultiSelect } from '@/components/ui/multi-select'

const OPTIONS = ['react', 'vue', 'svelte']

// Controlled wrapper so assertions after onChange reflect new state
function Controlled({
  initialSelected = [] as string[],
  onChange,
}: {
  initialSelected?: string[]
  onChange?: (v: string[]) => void
}) {
  const [selected, setSelected] = useState(initialSelected)
  return (
    <MultiSelect
      options={OPTIONS}
      selected={selected}
      onChange={(v) => {
        setSelected(v)
        onChange?.(v)
      }}
      placeholder="Select…"
    />
  )
}

describe('MultiSelect', () => {
  // ── 1. Placeholder ────────────────────────────────────────────────────────
  it('renders trigger with placeholder when nothing is selected', () => {
    render(
      <MultiSelect options={OPTIONS} selected={[]} onChange={() => {}} placeholder="Select…" />
    )
    expect(screen.getByRole('button', { name: 'Select…' })).toBeInTheDocument()
    expect(screen.getByText('Select…')).toBeInTheDocument()
  })

  // ── 2. Count badge ────────────────────────────────────────────────────────
  it('renders trigger showing count when multiple items are selected', () => {
    render(
      <MultiSelect
        options={OPTIONS}
        selected={['react', 'vue']}
        onChange={() => {}}
        placeholder="Select…"
      />
    )
    expect(screen.getByText('2 selected')).toBeInTheDocument()
  })

  it('renders the single selected value when exactly one item is selected', () => {
    render(
      <MultiSelect
        options={OPTIONS}
        selected={['react']}
        onChange={() => {}}
        placeholder="Select…"
      />
    )
    expect(screen.getByText('react')).toBeInTheDocument()
  })

  // ── 3. Opens on trigger click ─────────────────────────────────────────────
  it('opens the popover on trigger click', async () => {
    render(<Controlled />)
    await userEvent.click(screen.getByRole('button', { name: 'Select…' }))
    // CommandInput becomes visible when popover is open
    expect(screen.getByPlaceholderText('Search…')).toBeInTheDocument()
  })

  // ── 4. Renders all options ────────────────────────────────────────────────
  it('renders all option items inside the open dropdown', async () => {
    render(<Controlled />)
    await userEvent.click(screen.getByRole('button', { name: 'Select…' }))
    for (const opt of OPTIONS) {
      expect(screen.getByText(opt)).toBeInTheDocument()
    }
  })

  // ── 5. Selecting an unselected option ────────────────────────────────────
  it('calls onChange with option added when clicking an unselected option', async () => {
    const onChange = vi.fn()
    render(<Controlled onChange={onChange} />)
    await userEvent.click(screen.getByRole('button', { name: 'Select…' }))
    await userEvent.click(screen.getByText('react'))
    expect(onChange).toHaveBeenCalledWith(['react'])
  })

  // ── 6. Deselecting a selected option ─────────────────────────────────────
  it('calls onChange with option removed when clicking a selected option', async () => {
    const onChange = vi.fn()
    render(<Controlled initialSelected={['vue']} onChange={onChange} />)
    // trigger shows 'vue' (single selection label)
    await userEvent.click(screen.getByRole('button', { name: 'vue' }))
    // 'vue' now appears twice: once in trigger, once as an option item
    // click the option item (span with class text-secondary)
    const optionItems = screen.getAllByText('vue')
    // The option span is inside the popover — it has class 'text-secondary'
    const optionSpan = optionItems.find(
      (el) => el.tagName === 'SPAN' && el.className.includes('text-secondary')
    )
    await userEvent.click(optionSpan!)
    expect(onChange).toHaveBeenCalledWith([])
  })

  // ── 7. Search filtering ───────────────────────────────────────────────────
  it('typing in the search input filters the visible options', async () => {
    render(<Controlled />)
    await userEvent.click(screen.getByRole('button', { name: 'Select…' }))
    await userEvent.type(screen.getByPlaceholderText('Search…'), 're')
    // 'react' matches, 'vue' and 'svelte' should not be visible
    expect(screen.getByText('react')).toBeInTheDocument()
    expect(screen.queryByText('vue')).not.toBeInTheDocument()
    expect(screen.queryByText('svelte')).not.toBeInTheDocument()
  })

  // ── 8. No results ─────────────────────────────────────────────────────────
  it('shows "No results" when the search matches nothing', async () => {
    render(<Controlled />)
    await userEvent.click(screen.getByRole('button', { name: 'Select…' }))
    await userEvent.type(screen.getByPlaceholderText('Search…'), 'zzz')
    expect(screen.getByText('No results')).toBeInTheDocument()
  })

  // ── 9. Keyboard Enter selects ─────────────────────────────────────────────
  it('selects a focused item when Enter is pressed on it', async () => {
    const onChange = vi.fn()
    render(<Controlled onChange={onChange} />)
    await userEvent.click(screen.getByRole('button', { name: 'Select…' }))
    // Type to filter so only 'react' is available
    await userEvent.type(screen.getByPlaceholderText('Search…'), 'react')
    // Press Enter — cmdk selects the highlighted (first) item
    await userEvent.keyboard('{Enter}')
    expect(onChange).toHaveBeenCalledWith(['react'])
  })

  // ── 10. Escape closes the popover ────────────────────────────────────────
  it('closes the popover when Escape is pressed', async () => {
    render(<Controlled />)
    await userEvent.click(screen.getByRole('button', { name: 'Select…' }))
    expect(screen.getByPlaceholderText('Search…')).toBeInTheDocument()
    await userEvent.keyboard('{Escape}')
    expect(screen.queryByPlaceholderText('Search…')).not.toBeInTheDocument()
  })

  // ── Extra: custom placeholder ─────────────────────────────────────────────
  it('uses the provided placeholder text', () => {
    render(
      <MultiSelect
        options={OPTIONS}
        selected={[]}
        onChange={() => {}}
        placeholder="All companies"
      />
    )
    expect(screen.getByText('All companies')).toBeInTheDocument()
  })

  // ── Extra: className forwarding ───────────────────────────────────────────
  it('applies additional className to the trigger button', () => {
    render(
      <MultiSelect
        options={OPTIONS}
        selected={[]}
        onChange={() => {}}
        placeholder="Select…"
        className="custom-class"
      />
    )
    const trigger = screen.getByRole('button', { name: 'Select…' })
    expect(trigger).toHaveClass('custom-class')
  })

  // ── Extra: all three options rendered with checkboxes ────────────────────
  it('renders a checkbox indicator for each option reflecting selected state', async () => {
    render(<Controlled initialSelected={['react']} />)
    await userEvent.click(screen.getByRole('button', { name: 'react' }))
    // Radix Checkbox renders a button[role=checkbox] even with aria-hidden on the root
    // Use data-state to verify the checked state is reflected
    const checkedBoxes = document.querySelectorAll('[data-state="checked"]')
    const uncheckedBoxes = document.querySelectorAll('[data-state="unchecked"]')
    // 'react' is selected → one checked box
    expect(checkedBoxes.length).toBeGreaterThanOrEqual(1)
    // 'vue' and 'svelte' are not selected → two unchecked boxes
    expect(uncheckedBoxes.length).toBeGreaterThanOrEqual(2)
  })
})
