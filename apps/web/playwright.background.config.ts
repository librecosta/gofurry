import { defineConfig } from '@playwright/test'

// Opt-in live check: requires the API running from private --config YAML.
const baseURL = 'http://127.0.0.1:17322'
export default defineConfig({
  testDir: './tests', testMatch: ['start-backgrounds.spec.ts', 'start-suggestions-live.spec.ts'], workers: 1,
  use: { baseURL, browserName: 'chromium', channel: process.env.PLAYWRIGHT_CHANNEL },
  webServer: {
    command: 'node .output/server/index.mjs', url: `${baseURL}/readyz`, reuseExistingServer: false,
    env: { HOST: '127.0.0.1', PORT: '17322', NODE_ENV: 'development', WEB_REGION: process.env.WEB_REGION ?? 'CN', WEB_API_BASE_URL: process.env.START_API_URL ?? 'http://127.0.0.1:17321' },
  },
})
