import { Component, type ErrorInfo, type ReactNode } from 'react'

interface Props {
  fallback?: ReactNode
  children: ReactNode
}
interface State {
  hasError: boolean
}

export class ErrorBoundary extends Component<Props, State> {
  state: State = { hasError: false }

  static getDerivedStateFromError(): State {
    return { hasError: true }
  }

  componentDidCatch(error: Error, info: ErrorInfo) {
    console.error('ErrorBoundary caught:', error, info.componentStack)
  }

  render() {
    if (this.state.hasError) {
      return (
        this.props.fallback ?? (
          <div className="rounded-xl border border-[var(--border)] bg-subtle p-5 text-sm text-muted">
            Something went wrong loading this panel.
          </div>
        )
      )
    }
    return this.props.children
  }
}
