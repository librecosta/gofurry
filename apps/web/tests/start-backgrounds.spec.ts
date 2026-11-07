import { test, expect } from '@playwright/test'

const api = process.env.START_API_URL ?? 'http://127.0.0.1:17321'
const region = process.env.WEB_REGION ?? 'CN'
type Asset = { key: string; url: string; sha256: string; bytes: number; content_type: string; width: number; height: number }
type Background = { id: string; sfw: boolean; preview: Asset; image: Asset }

test('live: continuous resize retains the decoded wallpaper, full viewport coverage and zero overflow', async ({ page }, testInfo) => {
  test.setTimeout(90000)
  const requests: string[] = []
  page.on('request', (request) => {
    if (['image', 'fetch', 'xhr'].includes(request.resourceType())) requests.push(request.url())
  })
  await page.goto('/start')
  const image = page.locator('[data-start-hd]')
  await expect(image).toHaveCSS('opacity', '1', { timeout: 20000 })
  const original = await image.elementHandle()
  const requestCount = requests.length
  await page.evaluate(() => {
    const samples: { viewport: number[]; client: number[]; scroll: number[]; image: number[]; opacity: string }[] = []
    const probe = { running: true, samples }
    ;(window as unknown as { viewportProbe: typeof probe }).viewportProbe = probe
    const sample = () => {
      const root = document.documentElement, image = document.querySelector('[data-start-hd]')!
      const r = image.getBoundingClientRect()
      samples.push({ viewport: [innerWidth, innerHeight], client: [root.clientWidth, root.clientHeight], scroll: [root.scrollWidth, root.scrollHeight], image: [r.x, r.y, r.right, r.bottom], opacity: getComputedStyle(image).opacity })
      if (probe.running) requestAnimationFrame(sample)
    }
    requestAnimationFrame(sample)
  })
  const growing = Array.from({ length: 21 }, (_, i) => ({ width: 960 + i * 48, height: 600 + i * 24 }))
  const sizes = [...growing, ...[...growing].reverse(), { width: 390, height: 844 }, { width: 375, height: 667 }, { width: 844, height: 390 }]
  const appearance = async (value: 'light' | 'dark') => {
    await page.locator('[data-start-settings]').click()
    await page.getByRole('radio', { name: value === 'dark' ? /^(Dark|深色)$/ : /^(Light|浅色)$/ }).click()
  }
  for (const [index, size] of sizes.entries()) {
    if (index === 12) await appearance('dark') // Resize during the native reveal too.
    await page.setViewportSize(size)
    expect(await image.evaluate((element, first) => element === first, original)).toBe(true)
    await expect(image).toHaveCSS('opacity', '1')
    if (index % 4 === 0 || index >= sizes.length - 3) {
      // Inspect painted pixels at the right edge, not just CSS dimensions.
      const edge = await page.screenshot({ clip: { x: size.width - 12, y: Math.floor(size.height * 0.2), width: 12, height: Math.floor(size.height * 0.5) } })
      const blank = await page.evaluate(async (base64) => {
        const bitmap = new Image(); bitmap.src = `data:image/png;base64,${base64}`; await bitmap.decode()
        const canvas = new OffscreenCanvas(bitmap.width, bitmap.height), ctx = canvas.getContext('2d')!
        ctx.drawImage(bitmap, 0, 0)
        const data = ctx.getImageData(0, 0, canvas.width, canvas.height).data
        let white = 0, black = 0
        for (let i = 0; i < data.length; i += 4) {
          if (data[i] > 245 && data[i + 1] > 245 && data[i + 2] > 245) white++
          if (data[i] < 10 && data[i + 1] < 10 && data[i + 2] < 10) black++
        }
        return { white: white / (data.length / 4), black: black / (data.length / 4) }
      }, edge.toString('base64'))
      expect(blank.white).toBeLessThan(0.95); expect(blank.black).toBeLessThan(0.95)
    }
  }
  await expect(page.locator('html')).not.toHaveAttribute('data-gf-theme-reveal')
  await page.keyboard.press('Escape')
  await expect(page.locator('[data-start-preferences]')).toHaveCount(0)
  await appearance('light')
  await expect(page.locator('html')).toHaveAttribute('data-theme', 'light')
  await expect(page.locator('html')).not.toHaveAttribute('data-gf-theme-reveal')
  await page.keyboard.press('Escape')
  await expect(page.locator('[data-start-preferences]')).toHaveCount(0)
  const samples = await page.evaluate(() => {
    const probe = (window as unknown as { viewportProbe: { running: boolean; samples: { viewport: number[]; client: number[]; scroll: number[]; image: number[]; opacity: string }[] } }).viewportProbe
    probe.running = false
    return probe.samples
  })
  expect(samples.length).toBeGreaterThan(40)
  for (const sample of samples) {
    expect(sample.client).toEqual(sample.viewport)
    expect(sample.scroll).toEqual(sample.client)
    expect(sample.image).toEqual([0, 0, ...sample.viewport])
    expect(sample.opacity).toBe('1')
  }
  expect(requests).toHaveLength(requestCount)
  await testInfo.attach('viewport-frames', { body: JSON.stringify(samples), contentType: 'application/json' })
})

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
    const state = window as unknown as { startFades: number[] }
    state.startFades = []
    const sample = () => {
      const image = document.querySelector('[data-start-hd]')
      if (image) {
        const opacity = Number(getComputedStyle(image).opacity)
        state.startFades.push(opacity)
        if (opacity === 1) return
      }
      requestAnimationFrame(sample)
    }
    requestAnimationFrame(sample)
  })
  await page.setViewportSize({ width: 1440, height: 900 })
  await page.goto('/start')
  await expect(page.locator('html')).toHaveAttribute('lang', region === 'CN' ? 'zh' : 'en')
  await expect(page.locator('input[name="q"]')).toBeVisible()
  await expect(page.locator('[data-start-background]')).toHaveAttribute('data-stage', 'preview', { timeout: 20000 })
  await expect(page.locator('[data-start-preview]')).toHaveCSS('opacity', '1')
  expect(await page.locator('[data-start-preview]').evaluate((img) => (img as HTMLImageElement).naturalWidth)).toBe(item.preview.width)
  await page.screenshot({ path: testInfo.outputPath(`start-${region}-preview-desktop.png`) })
  release()
  await expect(page.locator('[data-start-background]')).toHaveAttribute('data-stage', 'image', { timeout: 20000 })
  await expect(page.locator('[data-start-hd]')).toHaveCSS('opacity', '1')
  expect(await page.evaluate(() => (window as unknown as { startFades: number[] }).startFades.some(opacity => opacity > 0 && opacity < 1))).toBe(true)
  await expect(page.locator('[data-start-preview]')).toHaveCSS('opacity', '1')
  expect(await page.locator('[data-start-hd]').evaluate((img) => (img as HTMLImageElement).naturalWidth)).toBe(item.image.width)
  await expect(page.locator('[data-start-hd]')).toHaveCSS('object-fit', 'cover')
  await expect(page.locator('[data-start-actions] > *')).toHaveCount(4)
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
  expect(form.y + form.height / 2).toBeLessThan(844 * 0.35)
  await page.screenshot({ path: testInfo.outputPath(`start-${region}-dark-mobile.png`) })
  await page.locator('[data-mobile-platform-trigger]').click()
  await expect(page.locator('[data-search-panel="platforms"]')).toHaveCSS('opacity', '1')
  await page.screenshot({ path: testInfo.outputPath(`start-${region}-mobile-platforms.png`) })
  expect(errors).toEqual([])
})

test('live: AVIF failure keeps the real WebP and does not block search', async ({ page, request }) => {
  const item = (await (await request.get(`${api}/api/v1/start/backgrounds`)).json() as { backgrounds: Background[] }).backgrounds[0]
  await page.route(item.image.url, (route) => route.abort())
  await page.goto('/start')
  await expect(page.locator('[data-start-background]')).toHaveAttribute('data-stage', 'preview', { timeout: 20000 })
  await expect(page.locator('[data-start-preview]')).toHaveCSS('opacity', '1')
  const action = 'https://www.bing.com/search'
  await expect(page.getByRole('search')).toHaveAttribute('action', action)
  await page.context().route(`${new URL(action).origin}/**`, (route) => route.fulfill({ contentType: 'text/html', body: '<!doctype html><title>Search intercepted</title>' }))
  await page.locator('input[name="q"]').fill('GoFurry 狼 & SFW')
  const opened = page.waitForEvent('popup')
  await page.locator('input[name="q"]').press('Enter')
  const result = await opened
  await result.waitForURL((url) => url.origin === new URL(action).origin)
  expect(new URL(result.url()).searchParams.get('q')).toBe('GoFurry 狼 & SFW')
  await result.close()
})
