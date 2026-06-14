/**
 * Smoke-render tests for shadcn/ui primitives added in Phase 2.
 *
 * These tests are intentionally minimal: they verify the components mount
 * without errors, export the expected named exports, and accept standard
 * HTML props. Behavioural tests (interactions, keyboard, ARIA) live in the
 * feature-level tests that compose these primitives.
 */
import { describe, it, expect } from 'vitest'
import { render, screen } from '@testing-library/react'
import {
  Card,
  CardHeader,
  CardTitle,
  CardDescription,
  CardContent,
  CardFooter,
} from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Separator } from '@/components/ui/separator'
import { Skeleton } from '@/components/ui/skeleton'
import { Checkbox } from '@/components/ui/checkbox'

// Note: Popover and ScrollArea rely on Radix portals / observers that require
// a fuller DOM environment; their integration is covered by the
// multi-select-dropdown tests (Phase 3) which compose them.

describe('Card', () => {
  it('renders children inside CardContent', () => {
    render(
      <Card>
        <CardHeader>
          <CardTitle>Test Title</CardTitle>
          <CardDescription>Test description</CardDescription>
        </CardHeader>
        <CardContent>Card body</CardContent>
        <CardFooter>Footer</CardFooter>
      </Card>
    )
    expect(screen.getByText('Test Title')).toBeInTheDocument()
    expect(screen.getByText('Test description')).toBeInTheDocument()
    expect(screen.getByText('Card body')).toBeInTheDocument()
    expect(screen.getByText('Footer')).toBeInTheDocument()
  })

  it('passes className to the root element', () => {
    const { container } = render(<Card className="custom-card" />)
    expect(container.firstChild).toHaveClass('custom-card')
  })
})

describe('Input', () => {
  it('renders an input element', () => {
    render(<Input placeholder="Search…" />)
    expect(screen.getByPlaceholderText('Search…')).toBeInTheDocument()
  })

  it('passes type and disabled props', () => {
    render(<Input type="email" disabled data-testid="email-input" />)
    const input = screen.getByTestId('email-input')
    expect(input).toHaveAttribute('type', 'email')
    expect(input).toBeDisabled()
  })
})

describe('Label', () => {
  it('renders label text', () => {
    render(<Label htmlFor="name">Full name</Label>)
    expect(screen.getByText('Full name')).toBeInTheDocument()
  })

  it('associates with an input via htmlFor', () => {
    render(
      <>
        <Label htmlFor="email">Email</Label>
        <Input id="email" />
      </>
    )
    expect(screen.getByLabelText('Email')).toBeInTheDocument()
  })
})

describe('Separator', () => {
  it('renders with default horizontal orientation', () => {
    render(<Separator data-testid="sep" />)
    const el = screen.getByTestId('sep')
    expect(el).toBeInTheDocument()
    expect(el).toHaveAttribute('data-orientation', 'horizontal')
  })

  it('renders with vertical orientation', () => {
    render(<Separator orientation="vertical" data-testid="vsep" />)
    expect(screen.getByTestId('vsep')).toHaveAttribute('data-orientation', 'vertical')
  })
})

describe('Skeleton', () => {
  it('renders a pulsing placeholder element', () => {
    const { container } = render(<Skeleton className="h-4 w-24" />)
    expect(container.firstChild).toHaveClass('animate-pulse')
    expect(container.firstChild).toHaveClass('h-4')
    expect(container.firstChild).toHaveClass('w-24')
  })
})

describe('Checkbox', () => {
  it('renders an unchecked checkbox by default', () => {
    render(<Checkbox aria-label="Accept terms" />)
    const cb = screen.getByRole('checkbox', { name: 'Accept terms' })
    expect(cb).toBeInTheDocument()
    expect(cb).not.toBeChecked()
  })

  it('renders a checked checkbox when defaultChecked', () => {
    render(<Checkbox defaultChecked aria-label="Checked box" />)
    expect(screen.getByRole('checkbox', { name: 'Checked box' })).toBeChecked()
  })

  it('is disabled when the disabled prop is passed', () => {
    render(<Checkbox disabled aria-label="Disabled box" />)
    expect(screen.getByRole('checkbox', { name: 'Disabled box' })).toBeDisabled()
  })
})
