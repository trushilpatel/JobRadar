import { describe, it, expect, vi } from 'vitest'
import { useState } from 'react'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { FilterPopover, type FilterOption } from '../FilterPopover'

const OPTIONS: FilterOption[] = [
  { value: 'remote', label: 'Remote' },
  { value: 'onsite', label: 'On-site' },
  { value: 'hybrid', label: 'Hybrid' },
]

// Controlled wrapper so assertions after onChange reflect new state.
function Controlled({
  initialSelected = [] as string[],
  onChange,
  searchable,
  emptyLabel,
  showCount,
}: {
  initialSelected?: string[]
  onChange?: (v: string[]) => void
  searchable?: boolean
  emptyLabel?: string
  showCount?: boolean
}) {
  const [selected, setSelected] = useState(initialSelected)
  return (
    <FilterPopover
      label="Location"
      options={OPTIONS}
      selected={selected}
      onChange={(v) => {
        setSelected(v)
        onChange?.(v)
      }}
      searchable={searchable}
      emptyLabel={emptyLabel}
      showCount={showCount}
    />
  )
}

describe('FilterPopover', () => {
  it('renders the label on the trigger when closed', () => {
    render(<FilterPopover label="Location" options={OPTIONS} selected={[]} onChange={() => {}} />)
    expect(screen.getByRole('button', { name: 'Location' })).toBeInTheDocument()
  })

  it('renders all option labels once opened', async () => {
    render(<Controlled />)
    await userEvent.click(screen.getByRole('button', { name: 'Location' }))
    for (const opt of OPTIONS) {
      expect(screen.getByText(opt.label)).toBeInTheDocument()
    }
  })

  it('calls onChange with the option appended when clicking an unchecked option', async () => {
    const onChange = vi.fn()
    render(<Controlled onChange={onChange} />)
    await userEvent.click(screen.getByRole('button', { name: 'Location' }))
    await userEvent.click(screen.getByText('Remote'))
    expect(onChange).toHaveBeenCalledWith(['remote'])
  })

  it('calls onChange with the option removed when clicking a checked option', async () => {
    const onChange = vi.fn()
    render(<Controlled initialSelected={['remote', 'hybrid']} onChange={onChange} />)
    await userEvent.click(screen.getByRole('button', { name: /Location/ }))
    await userEvent.click(screen.getByText('Remote'))
    expect(onChange).toHaveBeenCalledWith(['hybrid'])
  })

  it('does not render a search input by default', async () => {
    render(<Controlled />)
    await userEvent.click(screen.getByRole('button', { name: 'Location' }))
    expect(screen.queryByPlaceholderText('Search…')).not.toBeInTheDocument()
  })

  it('renders a search input when searchable is true', async () => {
    render(<Controlled searchable />)
    await userEvent.click(screen.getByRole('button', { name: 'Location' }))
    expect(screen.getByPlaceholderText('Search…')).toBeInTheDocument()
  })

  it('filters visible options when searchable and typing a query', async () => {
    render(<Controlled searchable />)
    await userEvent.click(screen.getByRole('button', { name: 'Location' }))
    await userEvent.type(screen.getByPlaceholderText('Search…'), 'rem')
    expect(screen.getByText('Remote')).toBeInTheDocument()
    expect(screen.queryByText('On-site')).not.toBeInTheDocument()
    expect(screen.queryByText('Hybrid')).not.toBeInTheDocument()
  })

  it('shows the default empty label when a search matches nothing', async () => {
    render(<Controlled searchable />)
    await userEvent.click(screen.getByRole('button', { name: 'Location' }))
    await userEvent.type(screen.getByPlaceholderText('Search…'), 'zzz')
    expect(screen.getByText('No results')).toBeInTheDocument()
  })

  it('shows a custom empty label when provided and a search matches nothing', async () => {
    render(<Controlled searchable emptyLabel="No companies" />)
    await userEvent.click(screen.getByRole('button', { name: 'Location' }))
    await userEvent.type(screen.getByPlaceholderText('Search…'), 'zzz')
    expect(screen.getByText('No companies')).toBeInTheDocument()
  })

  it('appends the selection count to the trigger label when showCount is true and something is selected', () => {
    render(
      <FilterPopover
        label="Location"
        options={OPTIONS}
        selected={['remote', 'hybrid']}
        onChange={() => {}}
        showCount
      />
    )
    expect(screen.getByRole('button', { name: 'Location (2)' })).toBeInTheDocument()
  })

  it('omits the count suffix when showCount is true but nothing is selected', () => {
    render(
      <FilterPopover
        label="Location"
        options={OPTIONS}
        selected={[]}
        onChange={() => {}}
        showCount
      />
    )
    expect(screen.getByRole('button', { name: 'Location' })).toBeInTheDocument()
  })

  it('omits the count suffix when showCount is false even if selections exist', () => {
    render(
      <FilterPopover label="Location" options={OPTIONS} selected={['remote']} onChange={() => {}} />
    )
    expect(screen.getByRole('button', { name: 'Location' })).toBeInTheDocument()
  })

  it('applies the active styling class when a selection exists', () => {
    render(
      <FilterPopover label="Location" options={OPTIONS} selected={['remote']} onChange={() => {}} />
    )
    expect(screen.getByRole('button', { name: 'Location' })).toHaveClass('text-violet')
  })

  it('selects a focused item via keyboard (ArrowDown + Enter)', async () => {
    // cmdk auto-highlights the first item ("Remote") on open; ArrowDown moves
    // the highlight to the second item ("On-site"), and Enter selects it.
    const onChange = vi.fn()
    render(<Controlled onChange={onChange} />)
    await userEvent.click(screen.getByRole('button', { name: 'Location' }))
    await userEvent.keyboard('{ArrowDown}')
    await userEvent.keyboard('{Enter}')
    expect(onChange).toHaveBeenCalledWith(['onsite'])
  })

  it('forwards an additional className to the trigger', () => {
    render(
      <FilterPopover
        label="Location"
        options={OPTIONS}
        selected={[]}
        onChange={() => {}}
        className="custom-class"
      />
    )
    expect(screen.getByRole('button', { name: 'Location' })).toHaveClass('custom-class')
  })
})
