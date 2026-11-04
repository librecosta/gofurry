import { test, expect, type Page } from '@playwright/test'
import { sampleMotion } from './motion'
import { writeFile } from 'node:fs/promises'

const region = process.env.WEB_REGION ?? 'CN'
const live = process.env.START_WEATHER_LIVE === '1'
const english = region === 'CN' ? '/en/start' : '/start'
const chinese = region === 'CN' ? '/start' : '/zh/start'
const endpoint = '**/api/v1/start/weather?*'
const headers = { 'X-GoFurry-Region': region, 'X-GoFurry-Content-Mode': 'SFW' }
const trigger = (page: Page) => page.locator('[data-start-weather]')
const popup = (page: Page) => page.locator('[data-start-weather-details]')
function weather(state = 'ready', lang = 'en', code = '101') {
  return {
    state, location_source: 'ip', location: state === 'unavailable' ? null : { city: { zh: '成都', en: 'Chengdu' }, country_code: 'CN', latitude: 30.67, longitude: 104.07 },
    weather: state === 'unavailable' ? null : { temperature_c: 22.5, condition_code: code, condition_text: lang === 'zh' ? '多云' : 'Cloudy', fetched_at: new Date().toISOString() },
    attribution: { weather: 'QWeather', weather_url: 'https://www.qweather.com', geolocation: 'DB-IP', geolocation_url: 'https://db-ip.com', weather_attributions: state === 'unavailable' ? [] : ['QWeather', 'https://www.qweather.com'] },
  }
}
async function mock(page: Page, state = 'ready', code = '101') {
  await page.route(endpoint, route => route.fulfill({ headers, json: weather(state, new URL(route.request().url()).searchParams.get('lang')!, code) }))
}
async function open(page: Page) { await trigger(page).click(); await expect(popup(page)).toHaveCSS('opacity', '1') }
async function noOverflow(page: Page) {
  expect(await page.evaluate(() => [document.documentElement, document.body].map(el => [el.scrollWidth <= innerWidth, el.scrollHeight <= innerHeight]))).toEqual([[true, true], [true, true]])
}

test('weather: hydration-only localized request, ready/stale/unavailable and safe visible sources', async ({ page, browser }) => {
  const ssrContext = await browser.newContext({ javaScriptEnabled: false })
  const ssr = await ssrContext.newPage(), ssrCalls: string[] = []
  ssr.on('request', req => { if (req.url().includes('/api/v1/start/weather')) ssrCalls.push(req.url()) })
  await ssr.goto(english); await expect(ssr.getByRole('search')).toBeVisible()
  await expect(trigger(ssr)).toHaveAttribute('data-weather-state', 'unavailable'); expect(ssrCalls).toEqual([])
  await ssrContext.close()

  for (const state of ['ready', 'stale', 'unavailable']) {
    const calls: string[] = []
    await page.route(endpoint, route => {
      calls.push(route.request().url())
      const requestHeaders = route.request().headers()
      for (const key of ['authorization', 'x-forwarded-for', 'x-real-ip', 'cf-connecting-ip']) expect(requestHeaders[key]).toBeUndefined()
      const body = weather(state, 'en', state === 'ready' ? '999' : '101')
      body.attribution.weather_attributions.push('javascript:alert(1)', '<img src=x onerror=alert(1)>', 'https://example.com/license', 'https://developer.qweather.com/attribution.html')
      return route.fulfill({ headers, json: body })
    })
    await page.goto(english)
    await expect(trigger(page)).toHaveAttribute('data-weather-state', state)
    if (state !== 'unavailable') await expect(trigger(page)).toContainText('22.5°C')
    else await expect(trigger(page)).not.toContainText('°C')
    if (state === 'ready') await expect(trigger(page).locator('svg')).toHaveAttribute('data-weather-icon', '999')
    await open(page)
    await expect(popup(page)).toContainText(state === 'unavailable' ? 'Weather unavailable' : 'Cloudy')
    await expect(popup(page).locator('time')).toHaveCount(state === 'unavailable' ? 0 : 1)
    await expect(popup(page).getByText('Cached weather;', { exact: false })).toHaveCount(state === 'stale' ? 1 : 0)
    await expect(popup(page).getByRole('link', { name: 'DB-IP', exact: true })).toHaveCount(0)
    await expect(popup(page).getByRole('link', { name: 'QWeather', exact: true })).toHaveAttribute('href', 'https://www.qweather.com')
    await expect(popup(page)).not.toContainText('Source details')
    await expect(popup(page)).not.toContainText('developer.qweather.com')
    await expect(popup(page).locator('a[href="https://developer.qweather.com/attribution.html"]')).toHaveCount(0)
    await expect(popup(page).locator('a[href="https://example.com/license"]')).toHaveText('example.com')
    await expect(popup(page).locator('a[href="https://example.com/license"]')).toHaveAttribute('rel', 'noopener noreferrer')
    await expect(popup(page).locator('a[href^="javascript:"]')).toHaveCount(0)
    await expect(popup(page).locator('img')).toHaveCount(0)
    await expect(popup(page)).toContainText('<img src=x onerror=alert(1)>')
    expect(calls).toHaveLength(1); expect(new URL(calls[0]).searchParams.toString()).toBe('lang=en')
    await popup(page).getByRole('button', { name: 'Choose city', exact: true }).click()
    await expect(popup(page).getByRole('link', { name: 'DB-IP', exact: true })).toHaveAttribute('href', 'https://db-ip.com')
    await popup(page).getByRole('combobox').press('Escape')
    await expect(popup(page).getByRole('button', { name: 'Choose city', exact: true })).toBeFocused()
    await page.keyboard.press('Escape'); await expect(popup(page)).toHaveCount(0)
    await page.unroute(endpoint)
  }
})

test('weather: first/repeated popup frames are stable with full exit and keyboard restoration', async ({ page }, testInfo) => {
  await mock(page); await page.goto(english); await expect(trigger(page)).toContainText('22.5°C')
  await page.evaluate(() => {
    const frames: { x: number; y: number; w: number; h: number; opacity: number; background: string }[] = []
    ;(window as unknown as { weatherFrames: typeof frames }).weatherFrames = frames
    const sample = () => {
      const el = document.querySelector('[data-start-weather-details]')
      if (el) { const r = el.getBoundingClientRect(), css = getComputedStyle(el); frames.push({ x: r.x, y: r.y, w: r.width, h: r.height, opacity: Number(css.opacity), background: css.backgroundColor }) }
      if (frames.length < 40) requestAnimationFrame(sample)
    }
    requestAnimationFrame(sample)
  })
  await trigger(page).focus(); await page.keyboard.press('Enter'); await expect(popup(page)).toHaveCSS('opacity', '1')
  const frames = await page.evaluate(() => (window as unknown as { weatherFrames: { x: number; y: number; w: number; h: number; opacity: number; background: string }[] }).weatherFrames)
  await testInfo.attach('weather-first-frames', { body: JSON.stringify(frames), contentType: 'application/json' })
  await writeFile(testInfo.outputPath('weather-first-frames.json'), JSON.stringify(frames))
  expect(frames.length).toBeGreaterThan(5)
  expect(Math.max(...frames.map(f => f.x)) - Math.min(...frames.map(f => f.x))).toBeLessThan(0.5)
  expect(Math.max(...frames.map(f => f.y)) - Math.min(...frames.map(f => f.y))).toBeLessThanOrEqual(4.5)
  expect(Math.max(...frames.map(f => f.w)) - Math.min(...frames.map(f => f.w))).toBeLessThan(0.5)
  expect(Math.max(...frames.map(f => f.h)) - Math.min(...frames.map(f => f.h))).toBeLessThan(0.5)
  expect(frames.every(f => f.background !== 'rgba(0, 0, 0, 0)')).toBe(true)
  await expect(popup(page).getByRole('link').first()).toBeFocused()
  await page.keyboard.press('Tab'); await expect(popup(page).getByRole('link').nth(1)).toBeFocused()
  await page.keyboard.press('Escape')
  const exit = await sampleMotion(popup(page))
  expect(exit.some(f => f.opacity > 0 && f.opacity < 1)).toBe(true)
  expect(exit.at(-1)!.t).toBeGreaterThan(250)
  await expect(popup(page)).toHaveCount(0); await expect(trigger(page)).toBeFocused()
  expect(await trigger(page).evaluate(el => el.matches(':focus-visible'))).toBe(true)
  await open(page); await page.mouse.click(4, 4); await expect(popup(page)).toHaveCount(0)
  await open(page); await trigger(page).click(); await expect(popup(page)).toHaveCount(0)
  await page.emulateMedia({ reducedMotion: 'reduce' }); await open(page)
  await expect(trigger(page)).toHaveCSS('transition-duration', '0s')
  await page.keyboard.press('Escape'); await expect(popup(page)).toHaveCount(0)
})

test('weather: URL language replaces/aborts old work and ordinary renders never refetch', async ({ page }) => {
  let release!: () => void
  const held = new Promise<void>(resolve => { release = resolve })
  const calls: string[] = [], aborted: string[] = []
  page.on('requestfailed', req => { if (req.url().includes('/api/v1/start/weather')) aborted.push(req.url()) })
  await page.route(endpoint, async route => {
    const lang = new URL(route.request().url()).searchParams.get('lang')!
    calls.push(lang)
    if (lang === 'zh') await held
    await route.fulfill({ headers, json: weather('ready', lang) }).catch(() => {})
  })
  await page.goto(chinese); await expect.poll(() => calls).toEqual(['zh'])
  await page.locator('[data-start-settings]').click()
  await page.getByRole('radio', { name: 'English', exact: true }).click()
  await expect(page).toHaveURL(english); await expect(trigger(page)).toContainText('Chengdu')
  release(); await expect.poll(() => aborted.length).toBe(1)
  await page.keyboard.press('Escape'); await open(page); await expect(popup(page)).toContainText('Cloudy')
  await page.keyboard.press('Escape'); await page.locator('[data-start-settings]').click()
  await page.getByRole('radio', { name: 'Dark', exact: true }).click()
  await expect(page.locator('html')).not.toHaveAttribute('data-gf-theme-reveal')
  await page.keyboard.press('Escape'); await open(page)
  expect(calls).toEqual(['zh', 'en']); await expect(popup(page)).not.toContainText('多云')
  expect(await page.evaluate(() => Object.keys(localStorage).concat(Object.keys(sessionStorage)).some(key => /weather|location|geoip/i.test(key)))).toBe(false)
})

test('weather: visibility refresh after 20 minutes retains content, bounds stale fallback, never polls', async ({ page }) => {
  let now = Date.now(), calls = 0, failed = false
  await page.addInitScript(value => {
    const realNow = Date.now
    ;(window as unknown as { weatherNow: number }).weatherNow = value
    Date.now = () => (window as unknown as { weatherNow: number }).weatherNow ?? realNow()
  }, now)
  await page.route(endpoint, route => {
    calls++
    const body = weather(); body.weather!.fetched_at = new Date(now).toISOString()
    return route.fulfill(failed ? { status: 429, headers, json: { error: { code: 429 } } } : { headers, json: body })
  })
  const visible = async (minutes: number) => {
    now += minutes * 60000
    await page.evaluate(value => { (window as unknown as { weatherNow: number }).weatherNow = value; document.dispatchEvent(new Event('visibilitychange')) }, now)
  }
  await page.goto(english); await expect(trigger(page)).toContainText('22.5°C'); expect(calls).toBe(1)
  await visible(19); await open(page); await page.keyboard.press('Escape'); expect(calls).toBe(1)
  await visible(2); await expect.poll(() => calls).toBe(2)
  await expect(trigger(page)).toHaveAttribute('data-weather-state', 'ready')
  failed = true; await visible(21); await expect(trigger(page)).toHaveAttribute('data-weather-state', 'stale')
  await expect(trigger(page)).toContainText('22.5°C')
  await visible(40); await expect(trigger(page)).toHaveAttribute('data-weather-state', 'unavailable')
  await expect(trigger(page)).not.toContainText('°C'); expect(calls).toBe(4)
})

test('weather: failures, bounded timeout and wrong region/content never fabricate weather', async ({ page }) => {
  for (const mode of ['timeout', 'network', 'invalid', 'region', 'content']) {
    await page.route(endpoint, async route => {
      if (mode === 'timeout') { await new Promise(resolve => setTimeout(resolve, 6200)); return route.fulfill({ headers, json: weather() }).catch(() => {}) }
      if (mode === 'network') return route.abort('failed')
      return route.fulfill({ headers: { ...headers, ...(mode === 'region' ? { 'X-GoFurry-Region': region === 'CN' ? 'INTL' : 'CN' } : {}), ...(mode === 'content' ? { 'X-GoFurry-Content-Mode': 'NSFW' } : {}) }, json: mode === 'invalid' ? { ...weather(), weather: { temperature_c: 500 } } : weather() })
    })
    await page.goto(english)
    if (mode === 'timeout') await page.waitForTimeout(6100)
    await open(page); await expect(popup(page)).toContainText('Weather unavailable')
    await expect(trigger(page)).not.toContainText('°C'); await expect(page.getByRole('alert')).toHaveCount(0)
    await expect(page.getByRole('search')).toBeVisible()
    await page.keyboard.press('Escape'); await expect(popup(page)).toHaveCount(0)
    await page.unroute(endpoint)
  }
})

test('weather: compact layouts, themes and real wallpaper remain stable', async ({ page }, testInfo) => {
  test.setTimeout(60000)
  const requests: string[] = []
  page.on('request', req => { if (/background|\.(avif|webp)/.test(req.url())) requests.push(req.url()) })
  await mock(page); await page.goto(english)
  await expect(trigger(page)).toContainText('22.5°C')
  await expect(trigger(page).locator('svg')).toHaveAttribute('data-weather-icon', '101')
  await expect(trigger(page)).toHaveCSS('background-color', 'rgba(0, 0, 0, 0)')
  await trigger(page).hover(); await expect(trigger(page)).not.toHaveCSS('background-color', 'rgba(0, 0, 0, 0)')
  await expect(trigger(page)).toHaveCSS('box-shadow', 'none'); await expect(trigger(page)).toHaveCSS('border-top-width', '0px')
  // The live local Go deployment is CN. INTL must reject its metadata instead
  // of bypassing the region boundary to obtain a test wallpaper.
  if (live && region === 'CN') await expect(page.locator('[data-start-hd]')).toHaveCSS('opacity', '1', { timeout: 20000 })
  else await page.waitForLoadState('networkidle')
  if (live && region === 'INTL') await expect(page.locator('[data-start-background]')).toHaveAttribute('data-stage', 'theme')
  const image = await page.locator('[data-start-hd]').count() ? await page.locator('[data-start-hd]').elementHandle() : null, count = requests.length
  for (const [width, height] of [[1280, 720], [960, 600], [390, 844], [375, 667], [320, 480], [768, 360]]) {
    await page.setViewportSize({ width, height }); await open(page)
    const bounds = (await popup(page).boundingBox())!
    expect(bounds.x).toBeGreaterThanOrEqual(16); expect(bounds.x + bounds.width).toBeLessThanOrEqual(width - 16)
    expect(bounds.y).toBeGreaterThanOrEqual(0); expect(bounds.y + bounds.height).toBeLessThanOrEqual(height)
    await noOverflow(page)
    await expect(trigger(page).locator('span').first()).toBeVisible({ visible: width > 480 })
    await page.keyboard.press('Escape'); await expect(popup(page)).toHaveCount(0)
    if (image) expect(await page.locator('[data-start-hd]').evaluate((el, first) => el === first, image)).toBe(true)
  }
  expect(requests).toHaveLength(count)
  for (const colorScheme of ['light', 'dark'] as const) {
    await page.emulateMedia({ colorScheme }); await page.setViewportSize({ width: 1280, height: 720 }); await open(page)
    await expect(page.locator('html')).toHaveAttribute('data-theme', colorScheme)
    await page.screenshot({ path: testInfo.outputPath(`desktop-${colorScheme}.png`) })
    await page.keyboard.press('Escape'); await expect(popup(page)).toHaveCount(0)
  }
  await page.locator('[data-start-settings]').click(); await page.getByRole('radio', { name: '简体中文', exact: true }).click()
  await page.keyboard.press('Escape'); await page.setViewportSize({ width: 390, height: 844 }); await open(page)
  await expect(popup(page)).toContainText('成都'); await expect(popup(page)).toContainText('多云')
  await page.screenshot({ path: testInfo.outputPath('mobile-weather.png') })
})

test('weather: local same-origin API keeps loopback unavailable and regional headers intact', async ({ page, request }) => {
  test.skip(!live, 'Opt-in local Go API check; ordinary UI tests need no private YAML or live API.')
  const response = await request.get('/api/v1/start/weather?lang=en&ip=8.8.8.8&latitude=30&longitude=104')
  // INTL verification uses a separate Web deployment but no cross-region Go API.
  if (region === 'INTL') { expect(response.status()).toBe(503); return }
  expect(response.status()).toBe(200)
  expect(response.headers()['x-gofurry-region']).toBe(region); expect(response.headers()['x-gofurry-content-mode']).toBe('SFW')
  expect(await response.json()).toMatchObject({ state: 'unavailable', weather: null })
  await page.goto(english); await open(page); await expect(popup(page)).toContainText('Weather unavailable')
})
