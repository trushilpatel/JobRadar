import js from '@eslint/js'
import globals from 'globals'
import reactHooks from 'eslint-plugin-react-hooks'
import reactRefresh from 'eslint-plugin-react-refresh'
import tseslint from 'typescript-eslint'
import { defineConfig, globalIgnores } from 'eslint/config'

export default defineConfig([
  globalIgnores(['dist']),

  // ── Core rules ─────────────────────────────────────────────────────────────
  {
    files: ['**/*.{ts,tsx}'],
    extends: [
      js.configs.recommended,
      tseslint.configs.recommended,
      reactHooks.configs.flat.recommended,
      reactRefresh.configs.vite,
    ],
    languageOptions: {
      ecmaVersion: 2020,
      globals: globals.browser,
      // Type-aware linting. Required by the no-unsafe-* family below, which
      // catches implicit `any` flowing from untyped boundaries — the holes
      // `strict` / no-explicit-any leave open.
      parserOptions: {
        projectService: true,
        tsconfigRootDir: import.meta.dirname,
      },
    },
    rules: {
      // Stricter typing bar (Phase 0): no `any` leaking through untyped values.
      // We intentionally adopt only the no-unsafe-* family for now; the
      // promise-safety rules (no-floating-promises / no-misused-promises) are
      // deferred to a later pass.
      '@typescript-eslint/no-unsafe-assignment': 'error',
      '@typescript-eslint/no-unsafe-member-access': 'error',
      '@typescript-eslint/no-unsafe-call': 'error',
      '@typescript-eslint/no-unsafe-return': 'error',
      '@typescript-eslint/no-unsafe-argument': 'error',
    },
  },

  // ── Type-unsafe rules off in tests ─────────────────────────────────────────
  {
    files: ['**/__tests__/**', '**/*.test.{ts,tsx}', 'src/test/**'],
    rules: {
      '@typescript-eslint/no-unsafe-assignment': 'off',
      '@typescript-eslint/no-unsafe-member-access': 'off',
      '@typescript-eslint/no-unsafe-call': 'off',
      '@typescript-eslint/no-unsafe-return': 'off',
      '@typescript-eslint/no-unsafe-argument': 'off',
    },
  },

  // ── Layer boundary rules (WARN → error in Arch PR10) ─────────────────────
  // Enforces the 6-layer unidirectional architecture from ARCHITECTURE.md.
  // Uses no-restricted-imports (built-in, no resolver needed) so @/ path aliases
  // are matched as-written against the import specifier string.
  //
  // Rule 1: lib/ + components/ cannot import from feature / page / app layers.
  {
    files: ['src/lib/**/*.{ts,tsx}', 'src/components/**/*.{ts,tsx}'],
    rules: {
      'no-restricted-imports': [
        'warn',
        {
          patterns: [
            {
              group: ['@/features/**', '@/pages/**', '@/app/**'],
              message:
                'Shared layers (lib/, components/) cannot import from feature, page, or app layers. See ARCHITECTURE.md.',
            },
          ],
        },
      ],
    },
  },

  // Rule 2: Cross-feature imports forbidden.
  // Inside a feature slice, use relative imports for own-feature files.
  // @/features/** from within features/ always means a different feature.
  {
    files: ['src/features/**/*.{ts,tsx}'],
    ignores: ['src/features/**/__tests__/**', 'src/features/**/*.test.{ts,tsx}'],
    rules: {
      'no-restricted-imports': [
        'warn',
        {
          patterns: [
            {
              group: ['@/features/**'],
              message:
                'Cross-feature imports are forbidden. Use relative imports within a feature; compose at the page level instead. See ARCHITECTURE.md.',
            },
          ],
        },
      ],
    },
  },

  // Rule 3: Pages + app must use feature barrels (@/features/X), not internals.
  // Pattern @/features/*/* matches one segment past the domain (internals).
  {
    files: ['src/pages/**/*.{ts,tsx}', 'src/app/**/*.{ts,tsx}'],
    rules: {
      'no-restricted-imports': [
        'warn',
        {
          patterns: [
            {
              group: ['@/features/*/*', '@/features/*/**'],
              message:
                'Import from the feature barrel (@/features/X), not internals (@/features/X/Y). See ARCHITECTURE.md.',
            },
          ],
        },
      ],
    },
  },
])
