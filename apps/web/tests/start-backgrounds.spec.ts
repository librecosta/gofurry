import { test, expect } from '@playwright/test'

const api = process.env.START_API_URL ?? 'http://127.0.0.1:17321'
const region = process.env.WEB_REGION ?? 'CN'
type Asset = { key: string; url: string; sha256: string; bytes: number; content_type: string; width: number; height: number }
type Background = { id: string; sfw: boolean; preview: Asset; image: Asset }

test('live: real Go/R2 metadata and CDN WebP decode precede the AVIF handoff', async ({ page, request }, testInfo) => {
  test.setTimeout(90000)
  const response = await request.get(`${api}/api/v1/start/backgrounds`)
  expect(response.ok()).toBe(true)
  expect(response.headers()['x-gofurry-region']).toBe(region)
  const item = (await response.json() as { backgrounds: Background[] }).backgrounds[0]
  expect(item.sfw).toBe(true)
  for (const asset of [item.preview, item.image]) {
    expect(asset.key).toMatch(/^start\/backgrounds\/[a-f0-9]{64}\.(webp|avif)$/)
    const result = await request.get(asset.url)
    expect(result.ok()).toBe(true)
    expect(result.headers()['content-type']).toBe(asset.content_type)
    const bytes = await result.body()
    expect(bytes.length).toBe(asset.bytes)
    const { createHash } = await import('node:crypto')
    expect(createHash('sha256').update(bytes).digest('hex')).toBe(asset.sha256)
  }
  const errors: string[] = []
  page.on('pageerror', (error) => errors.push(error.message))
  page.on('console', (entry) => { if (entry.type() === 'error') errors.push(entry.text()) })
  let release!: () => void
  const highResolution = new Promise<void>((resolve) => { release = resolve })
  await page.route(item.image.url, async (route) => { await highResolution; await route.continue() })
  await page.addInitScript(() => {
    const state = window as unknown as { startFades: string[] }
    state.startFades = []
    document.addEventListener('transitionstart', (event) => {
      if ((event.target as HTMLElement).matches('[data-start-hd]')) state.startFades.push(event.propertyName)
    })
  })
  await page.setViewportSize({ width: 1440, height: 900 })
  await page.goto('/start')
  await expect(page.locator('html')).toHaveAttribute('lang', region === 'CN' ? 'zh' : 'en')
  await expect(page.getByRole('searchbox')).toBeVisible()
  await expect(page.locator('[data-start-background]')).toHaveAttribute('data-stage', 'preview', { timeout: 20000 })
  await expect(page.locator('[data-start-preview]')).toHaveCSS('opacity', '1')
  expect(await page.locator('[data-start-preview]').evaluate((img) => (img as HTMLImageElement).naturalWidth)).toBe(item.preview.width)
  await page.screenshot({ path: testInfo.outputPath(`start-${region}-preview-desktop.png`) })
  release()
  await expect(page.locator('[data-start-background]')).toHaveAttribute('data-stage', 'image', { timeout: 20000 })
  await expect(page.locator('[data-start-hd]')).toHaveCSS('opacity', '1')
  expect(await page.evaluate(() => (window as unknown as { startFades: string[] }).startFades)).toContain('opacity')
  await expect(page.locator('[data-start-preview]')).toHaveCSS('opacity', '1')
  expect(await page.locator('[data-start-hd]').evaluate((img) => (img as HTMLImageElement).naturalWidth)).toBe(item.image.width)
  await expect(page.locator('[data-start-hd]')).toHaveCSS('object-fit', 'cover')
  await expect(page.locator('button, select, a')).toHaveCount(0)
  for (const theme of ['light', 'dark']) {
    await page.context().addCookies([{ name: 'gf-theme', value: theme, url: 'http://127.0.0.1:17322' }])
    await page.reload()
    await expect(page.locator('[data-start-hd]')).toHaveCSS('opacity', '1', { timeout: 20000 })
    await expect(page.locator('html')).toHaveAttribute('data-theme', theme)
    await page.screenshot({ path: testInfo.outputPath(`start-${region}-${theme}-desktop.png`) })
  }
  await page.setViewportSize({ width: 390, height: 844 })
  const form = (await page.getByRole('search').boundingBox())!
  expect(Math.abs(form.x + form.width / 2 - 195)).toBeLessThanOrEqual(1)
  expect(Math.abs(form.y + form.height / 2 - 422)).toBeLessThanOrEqual(1)
  await page.screenshot({ path: testInfo.outputPath(`start-${region}-dark-mobile.png`) })
  expect(errors).toEqual([])
})

test('live: AVIF failure keeps the real WebP and does not block search', async ({ page, request }) => {
  const item = (await (await request.get(`${api}/api/v1/start/backgrounds`)).json() as { backgrounds: Background[] }).backgrounds[0]
  await page.route(item.image.url, (route) => route.abort())
  await page.goto('/start')
  await expect(page.locator('[data-start-background]')).toHaveAttribute('data-stage', 'preview', { timeout: 20000 })
  await expect(page.locator('[data-start-preview]')).toHaveCSS('opacity', '1')
  const action = region === 'CN' ? 'https://www.bing.com/search' : 'https://www.google.com/search'
  await expect(page.getByRole('search')).toHaveAttribute('action', action)
  await page.route(`${new URL(action).origin}/**`, (route) => route.fulfill({ contentType: 'text/html', body: '<!doctype html><title>Search intercepted</title>' }))
  await page.getByRole('searchbox').fill('GoFurry 狼 & SFW')
  await page.getByRole('searchbox').press('Enter')
  await page.waitForURL((url) => url.origin === new URL(action).origin)
  expect(new URL(page.url()).searchParams.get('q')).toBe('GoFurry 狼 & SFW')
})
