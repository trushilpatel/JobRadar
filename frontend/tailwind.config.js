export default {
  content: ['./index.html', './src/**/*.{ts,tsx}'],
  darkMode: 'class',
  theme: {
    extend: {
      colors: {
        page: 'var(--bg-page)',
        panel: 'var(--bg-panel)',
        subtle: 'var(--bg-subtle)',
        'input-bg': 'var(--bg-input)',
        hover: 'var(--bg-hover)',
        'theme-border': 'var(--border)',
        'border-strong': 'var(--border-strong)',
        primary: 'var(--text-primary)',
        secondary: 'var(--text-secondary)',
        muted: 'var(--text-muted)',
        placeholder: 'var(--text-placeholder)',
        accent: 'var(--accent)',
        'accent-dim': 'var(--accent-dim)',
        'accent-text': 'var(--accent-text)',
        sidebar: 'var(--sidebar-bg)',
        'sidebar-border': 'var(--sidebar-border)',
        'sidebar-fg': 'var(--sidebar-fg)',
      },
    },
  },
  plugins: [],
}
