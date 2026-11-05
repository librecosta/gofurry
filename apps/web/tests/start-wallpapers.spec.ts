import { test, expect } from '@playwright/test'
const api = process.env.START_API_URL ?? 'http://127.0.0.1:17321'
const region = process.env.WEB_REGION ?? 'CN'
const unavailable = process.env.START_EXPECT_UNAVAILABLE === '1'
test('live: database catalog, WebP high-resolution compatibility and outage fallback preserve Start', async ({ page, request }) => {
  const errors: string[] = []
  page.on('pageerror', error => errors.push(error.message))
  page.on('console', entry => { if (entry.type() === 'error') errors.push(entry.text()) })
  const response = await request.get(`${api}/api/v1/start/backgrounds`)
  expect(response.headers()['x-gofurry-region']).toBe(region)
  expect(response.headers()['x-gofurry-content-mode']).toBe('SFW')
  const imageRequests: string[] = []
  page.on('request', request => { if (request.resourceType() === 'image' && request.url().includes('/start/backgrounds/')) imageRequests.push(request.url()) })
  await page.goto('/start')
  await expect(page.getByRole('search')).toBeVisible()
  if (unavailable) {
    expect(response.status()).toBe(503)
    await expect(page.locator('[data-start-background]')).toHaveAttribute('data-stage', 'theme')
    await expect(page.locator('[data-start-preview], [data-start-hd]')).toHaveCount(0)
  } else {
    expect(response.status()).toBe(200)
    const { backgrounds } = await response.json() as { backgrounds: { id: string; sfw: boolean; image: { content_type: string } }[] }
    expect(backgrounds[0].id).toMatch(/^[0-9a-f]{8}-[0-9a-f]{4}-7[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/)
    expect(backgrounds.every(row => row.sfw)).toBe(true)
    if (process.env.START_EXPECT_IMAGE_TYPE) expect(backgrounds[0].image.content_type).toBe(process.env.START_EXPECT_IMAGE_TYPE)
    await expect(page.locator('[data-start-hd]')).toHaveCSS('opacity', '1', { timeout: 20000 })
  }
  const count = imageRequests.length
  await page.locator('[data-start-settings]').click()
  await page.getByRole('radio', { name: /^(Dark|深色)$/ }).click()
  await expect(page.locator('html')).toHaveAttribute('data-theme', 'dark')
  await page.keyboard.press('Escape')
  await page.setViewportSize({ width: 375, height: 812 })
  expect(await page.evaluate(() => document.documentElement.scrollWidth === innerWidth)).toBe(true)
  expect(imageRequests).toHaveLength(count)
  await page.getByRole('search').locator('input[name=q]').fill('GoFurry wallpaper compatibility')
  await expect(page.getByRole('search')).toHaveAttribute('action', 'https://www.bing.com/search')
  expect(errors).toEqual([])
})
