import { defineConfig } from '@playwright/test'
export default defineConfig({
  testDir: './tests',
  testMatch: 'runtime.spec.ts',
  use: { baseURL: 'http://127.0.0.1:3100', browserName: 'chromium', channel: process.env.PLAYWRIGHT_CHANNEL },
  webServer: {
    command: 'node .output/server/index.mjs',
    url: 'http://127.0.0.1:3100/readyz',
    env: { PORT: '3100', HOST: '127.0.0.1', NODE_ENV: 'production' },
    reuseExistingServer: false,
  },
})
