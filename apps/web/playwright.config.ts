import { defineConfig } from '@playwright/test'
export default defineConfig({
  testDir: './tests',
  testMatch: ['runtime.spec.ts', 'governance.spec.ts', 'start.spec.ts', 'search-platforms.spec.ts', 'start-search-catalog.spec.ts', 'start-suggestions.spec.ts', 'start-weather.spec.ts', 'start-cities.spec.ts', 'start-weather-ux.spec.ts', 'start-quick-access.spec.ts'],
  use: { baseURL: 'http://127.0.0.1:17320', browserName: 'chromium', channel: process.env.PLAYWRIGHT_CHANNEL },
  webServer: {
    command: 'node .output/server/index.mjs',
    url: 'http://127.0.0.1:17320/readyz',
    env: { PORT: '17320', HOST: '127.0.0.1', NODE_ENV: 'production', WEB_REGION: process.env.WEB_REGION ?? 'CN' },
    reuseExistingServer: false,
  },
})
