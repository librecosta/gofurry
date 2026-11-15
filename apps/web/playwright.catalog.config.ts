import { defineConfig } from '@playwright/test'

export default defineConfig({
  testDir: './tests',
  outputDir: './playwright-report/catalog',
  testMatch: ['catalog.spec.ts', 'yearbook.spec.ts', 'navigation.spec.ts', 'games-channel.spec.ts', 'game-profile.spec.ts'],
  workers: 1,
  use: { browserName: 'chromium', channel: process.env.PLAYWRIGHT_CHANNEL },
})
