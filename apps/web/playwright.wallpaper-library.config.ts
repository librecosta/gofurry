import { defineConfig } from '@playwright/test'
import live from './playwright.background.config'
// Explicit opt-in: native regional API, PostgreSQL and development CDN required.
export default defineConfig(live, { testMatch: 'wallpaper-library.spec.ts' })
