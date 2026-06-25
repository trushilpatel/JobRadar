/// <reference types="vitest" />
import { defineConfig, loadEnv } from 'vite'
import react from '@vitejs/plugin-react-swc'
import path from 'path'

export default defineConfig(({ mode }) => {
  // Load env vars from repo root (where doppler writes them / .env lives).
  // API_PROXY_TARGET is not VITE_-prefixed — it's a build-time Node var used
  // only by the dev-server proxy, never shipped to the browser.
  const env = loadEnv(mode, path.resolve(__dirname, '../'), '')
  const apiProxyTarget = env.API_PROXY_TARGET || 'http://localhost:30080'

  return {
    plugins: [react()],
    envDir: path.resolve(__dirname, '../'),
    resolve: {
      alias: {
        '@': path.resolve(__dirname, './src'),
      },
    },
    build: {
      sourcemap: false,
      rollupOptions: {
        output: {
          manualChunks: {
            vendor: ['react', 'react-dom', 'react-router-dom'],
            charts: ['recharts'],
            ui: [
              '@radix-ui/react-dialog',
              '@radix-ui/react-select',
              '@radix-ui/react-switch',
              '@radix-ui/react-tooltip',
              'lucide-react',
            ],
            data: ['@tanstack/react-query', '@tanstack/react-table'],
          },
        },
      },
    },
    server: {
      host: true,
      proxy: {
        // Admin routes: strip /api prefix — Go router registers /admin/* at root
        '/api/admin': {
          target: apiProxyTarget,
          rewrite: (p) => p.replace(/^\/api/, ''),
        },
        // User API routes: preserve /api prefix — Go router registers under /api/
        '/api': {
          target: apiProxyTarget,
        },
      },
    },
    test: {
      environment: 'jsdom',
      // Dummy values so `config/env.ts`'s eager validation passes at import time.
      // CI has no repo-root `.env` (gitignored), and every test mocks the API /
      // auth layer, so Clerk is never actually initialised from these.
      env: {
        VITE_CLERK_PUBLISHABLE_KEY: 'pk_test_fake_key_for_testing',
      },
      setupFiles: ['./src/test/setup.ts'],
      globals: true,
      // Emit a JUnit report in CI (GitHub sets CI=true) for Codecov Test
      // Analytics; locally we keep the default reporter only — no stray files.
      reporters: process.env.CI ? ['default', 'junit'] : ['default'],
      outputFile: { junit: './coverage/junit.xml' },
      coverage: {
        provider: 'v8',
        reporter: ['text', 'lcov', 'json-summary'],
        thresholds: {
          statements: 90, // enforced — every measured source file is at >=90% (overall ~98%); see Designs/frontend-test-coverage-spec.md
        },
        exclude: [
          // Test scaffolding, generated, and build output — never source under test
          'src/test/**',
          '**/*.d.ts',
          '**/*.config.*',
          'dist/**',
          'plopfile.js',
          '**/__tests__/**',
          '**/*.test.{ts,tsx}',
          // Type-only modules (no executable statements to cover)
          'src/types/**',
          'src/lib/commands/types.ts',
          // App wiring / entry points — frontend analogue of backend main.go / wire.go
          'src/main.tsx',
          'src/App.tsx',
          'src/app/router.tsx',
          'src/app/providers.tsx',
          'src/features/auth/components/SignInPage.tsx',
          'src/lib/core/queryClient.ts',
          // resumes feature is mock-data WIP (no real api/ layer) — excluded until wired to the backend
          'src/features/resumes/**',
          'src/pages/admin/ResumeLibraryPage.tsx',
          'src/pages/admin/ResumeDetailPage.tsx',
          'src/pages/admin/ResumeEditorPage.tsx',
          'src/pages/admin/ApplyQueuePage.tsx',
          'src/pages/jobs/JobResumeSection.tsx',
        ],
      },
    },
  }
})
