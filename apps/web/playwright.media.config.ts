import { defineConfig } from '@playwright/test'
import { webFixtureConfig } from './tests/web-fixture-config'

const api = 'http://127.0.0.1:17561'
const web = 'http://127.0.0.1:17560'

export default defineConfig({
  testDir: './tests',
  testMatch: 'public-media.spec.ts',
  outputDir: './playwright-report/public-media',
  use: { baseURL: web, browserName: 'chromium', channel: process.env.PLAYWRIGHT_CHANNEL },
  webServer: [
    { command: 'node tests/public-media-fixture.mjs', url: api + '/readyz' },
    {
      command: 'node .output/server/index.mjs', url: web + '/readyz',
      env: { PORT: '17560', HOST: '127.0.0.1', NODE_ENV: 'production', GOFURRY_WEB_CONFIG_FILE: webFixtureConfig(api) },
    },
  ],
})
