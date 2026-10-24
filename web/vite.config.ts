import { defineConfig } from 'vitest/config'

export default defineConfig({
  root: '.',
  build: {
    outDir: '../internal/webassets/static',
    emptyOutDir: true,
    assetsDir: 'assets',
  },
  test: {
    environment: 'node',
    include: ['src/**/*.test.ts', 'src/**/*.test.tsx'],
  },
})
