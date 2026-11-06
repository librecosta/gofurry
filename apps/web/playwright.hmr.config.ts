import { defineConfig } from '@playwright/test'
const port = Number(process.env.PLAYWRIGHT_HMR_PORT ?? 17320)
const baseURL = `http://127.0.0.1:${port}`
export default defineConfig({
  testDir: './tests',
  testMatch: 'hmr.spec.ts',
  workers: 1,
  use: { baseURL, browserName: 'chromium', channel: process.env.PLAYWRIGHT_CHANNEL },
  webServer: {
    command: `pnpm exec vite --port ${port} --strictPort`,
    url: `${baseURL}/readyz`,
    reuseExistingServer: false,
  },
})
