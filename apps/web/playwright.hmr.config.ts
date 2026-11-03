import { defineConfig } from '@playwright/test'
export default defineConfig({
  testDir: './tests',
  testMatch: 'hmr.spec.ts',
  workers: 1,
  use: { baseURL: 'http://127.0.0.1:17320', browserName: 'chromium', channel: process.env.PLAYWRIGHT_CHANNEL },
  webServer: {
    command: 'pnpm exec vite',
    url: 'http://127.0.0.1:17320/readyz',
    reuseExistingServer: false,
  },
})
