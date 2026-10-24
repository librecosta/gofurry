import { defineConfig } from '@playwright/test'
export default defineConfig({
  testDir: './tests',
  testMatch: 'hmr.spec.ts',
  workers: 1,
  use: { baseURL: 'http://127.0.0.1:3101', browserName: 'chromium', channel: process.env.PLAYWRIGHT_CHANNEL },
  webServer: {
    command: 'pnpm exec vite --host 127.0.0.1 --port 3101 --strictPort',
    url: 'http://127.0.0.1:3101/readyz',
    reuseExistingServer: false,
  },
})
