/// <reference types="vitest" />
import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react-swc'
import path from 'path'

export default defineConfig({
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
      '/api': {
        target: 'http://localhost:8080',
        rewrite: (p) => p.replace(/^\/api/, ''),
      },
    },
  },
  test: {
    environment: 'jsdom',
    setupFiles: ['./src/test/setup.ts'],
    globals: true,
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
        'src/app/auth-bootstrap.tsx',
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
})
