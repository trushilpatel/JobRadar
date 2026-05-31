import { describe, it, expect, vi } from 'vitest'
import { render, screen, fireEvent } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MultiSelectDropdown } from '@/components/ui/multi-select-dropdown'

const options = ['react', 'vue', 'svelte']

describe('MultiSelectDropdown', () => {
  it('shows the placeholder when nothing is selected', () => {
    render(<MultiSelectDropdown options={options} selected={[]} onChange={() => {}} />)
    expect(screen.getByRole('button', { name: /All/i })).toBeInTheDocument()
  })

  it('shows the single selected value when one is selected', () => {
    render(<MultiSelectDropdown options={options} selected={['react']} onChange={() => {}} />)
    expect(screen.getByText('react')).toBeInTheDocument()
  })

  it('shows the singular-count label when multiple are selected', () => {
    render(
      <MultiSelectDropdown
        options={options}
        selected={['react', 'vue']}
        onChange={() => {}}
        singularLabel={(n) => `${n} techs`}
      />
    )
    expect(screen.getByText('2 techs')).toBeInTheDocument()
  })

  it('opens on click and lists options, toggling selection on click', async () => {
    const onChange = vi.fn()
    render(<MultiSelectDropdown options={options} selected={[]} onChange={onChange} />)
    await userEvent.click(screen.getByRole('button'))
    await userEvent.click(screen.getByRole('button', { name: 'vue' }))
    expect(onChange).toHaveBeenCalledWith(['vue'])
  })

  it('deselects an already-selected option', async () => {
    const onChange = vi.fn()
    render(<MultiSelectDropdown options={options} selected={['vue']} onChange={onChange} />)
    // The trigger label is 'vue' (single selection). Open via aria-expanded trigger.
    const trigger = screen.getByRole('button', { expanded: false })
    await userEvent.click(trigger)
    // The option rows live inside the search panel. Find the option button that
    // is NOT the trigger (the trigger is now expanded).
    const optionRow = screen
      .getAllByRole('button')
      .find((b) => b.textContent?.includes('vue') && b.getAttribute('aria-expanded') === null)
    await userEvent.click(optionRow!)
    expect(onChange).toHaveBeenCalledWith([])
  })

  it('filters options via the search input and shows "No matches"', async () => {
    render(<MultiSelectDropdown options={options} selected={[]} onChange={() => {}} />)
    await userEvent.click(screen.getByRole('button'))
    const search = screen.getByPlaceholderText('Search…')
    await userEvent.type(search, 'zzz')
    expect(screen.getByText('No matches')).toBeInTheDocument()
  })

  it('closes when clicking outside', async () => {
    render(<MultiSelectDropdown options={options} selected={[]} onChange={() => {}} />)
    await userEvent.click(screen.getByRole('button'))
    expect(screen.getByPlaceholderText('Search…')).toBeInTheDocument()
    fireEvent.mouseDown(document.body)
    expect(screen.queryByPlaceholderText('Search…')).not.toBeInTheDocument()
  })

  it('adds a new value via the add input and the Add button when addable', async () => {
    const onChange = vi.fn()
    const onAdd = vi.fn()
    render(
      <MultiSelectDropdown
        options={options}
        selected={[]}
        onChange={onChange}
        addable
        onAdd={onAdd}
      />
    )
    await userEvent.click(screen.getByRole('button'))
    const addInput = screen.getByPlaceholderText('Add new…')
    await userEvent.type(addInput, 'Angular')
    await userEvent.click(screen.getByRole('button', { name: '+ Add' }))
    expect(onAdd).toHaveBeenCalledWith('angular')
    expect(onChange).toHaveBeenCalledWith(['angular'])
  })

  it('adds a new value on Enter key', async () => {
    const onChange = vi.fn()
    render(<MultiSelectDropdown options={options} selected={[]} onChange={onChange} addable />)
    await userEvent.click(screen.getByRole('button'))
    const addInput = screen.getByPlaceholderText('Add new…')
    await userEvent.type(addInput, 'ember{Enter}')
    expect(onChange).toHaveBeenCalledWith(['ember'])
  })

  it('ignores an empty add value', async () => {
    const onChange = vi.fn()
    render(<MultiSelectDropdown options={options} selected={[]} onChange={onChange} addable />)
    await userEvent.click(screen.getByRole('button'))
    await userEvent.click(screen.getByRole('button', { name: '+ Add' }))
    expect(onChange).not.toHaveBeenCalled()
  })

  it('does not duplicate when adding an already-selected value', async () => {
    const onChange = vi.fn()
    const onAdd = vi.fn()
    render(
      <MultiSelectDropdown
        options={options}
        selected={['react']}
        onChange={onChange}
        addable
        onAdd={onAdd}
      />
    )
    await userEvent.click(screen.getByRole('button'))
    const addInput = screen.getByPlaceholderText('Add new…')
    await userEvent.type(addInput, 'react{Enter}')
    expect(onAdd).toHaveBeenCalledWith('react')
    expect(onChange).not.toHaveBeenCalled()
  })

  it('renders a leading icon and forwards aria-label', async () => {
    render(
      <MultiSelectDropdown
        options={options}
        selected={[]}
        onChange={() => {}}
        leadingIcon={<svg data-testid="icon" />}
        aria-label="Tech filter"
      />
    )
    expect(screen.getByTestId('icon')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Tech filter' })).toBeInTheDocument()
  })
})
