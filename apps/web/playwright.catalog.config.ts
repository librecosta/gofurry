import { defineConfig } from '@playwright/test'

export default defineConfig({
  testDir: './tests',
  testMatch: ['catalog.spec.ts', 'yearbook.spec.ts', 'navigation.spec.ts'],
  workers: 1,
  use: { browserName: 'chromium', channel: process.env.PLAYWRIGHT_CHANNEL },
})
