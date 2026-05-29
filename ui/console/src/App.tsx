import { BrowserRouter } from 'react-router-dom'
import { Providers } from './app/providers'
import { AuthBootstrap } from './app/auth-bootstrap'
import { AppRouter } from './app/router'

export default function App() {
  return (
    <BrowserRouter>
      <Providers>
        <AuthBootstrap>
          <AppRouter />
        </AuthBootstrap>
      </Providers>
    </BrowserRouter>
  )
}
