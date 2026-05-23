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
          <div className="rounded-xl border border-white/[0.07] bg-white/[0.04] p-5 text-sm text-slate-500">
            Something went wrong loading this panel.
          </div>
        )
      )
    }
    return this.props.children
  }
}
