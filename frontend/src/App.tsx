import { BrowserRouter } from 'react-router-dom'
import { Providers } from './app/providers'
import { AppRouter } from './app/router'
import { ErrorBoundary } from '@/components/shared/ErrorBoundary'

export default function App() {
  return (
    <ErrorBoundary>
      <BrowserRouter>
        <Providers>
          <AppRouter />
        </Providers>
      </BrowserRouter>
    </ErrorBoundary>
  )
}
