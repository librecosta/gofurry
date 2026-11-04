import { test, expect, type Page } from '@playwright/test'

const region = process.env.WEB_REGION ?? 'CN'
const path = region === 'CN' ? '/en/start' : '/start'
const chinese = region === 'CN' ? '/start' : '/zh/start'
const headers = { 'X-GoFurry-Region': region, 'X-GoFurry-Content-Mode': 'SFW' }
const key = `gf-weather-city:${region}`
const city = { city_id: '1815286', name: 'Chengdu', names: { zh: '成都', en: 'Chengdu' }, admin: 'Sichuan', country_code: 'CN' }
const trigger = (page: Page) => page.locator('[data-start-weather]')
const popup = (page: Page) => page.locator('[data-start-weather-details]')
const locate = (page: Page) => popup(page).getByRole('button', { name: 'Locate current position' })
async function fixture(page: Page, codes = ['150'], attributions = ['QWeather', 'https://example.com/license', 'Required attribution statement.']) {
  const calls: string[] = []
  await page.route('**/api/v1/start/weather?*', route => {
    calls.push(route.request().url())
    const params = new URL(route.request().url()).searchParams, id = params.get('city_id')
    return route.fulfill({ headers, json: { state: 'ready', location_source: id ? 'manual' : 'ip', ...(id ? { city_id: id } : {}),
      location: { city: city.names, country_code: 'CN' },
      weather: { temperature_c: 22.5, condition_code: codes.shift() ?? '100', condition_text: params.get('lang') === 'zh' ? '晴' : 'Clear', fetched_at: new Date().toISOString() },
      attribution: { weather_attributions: attributions } } })
  })
  await page.route('**/api/v1/start/cities?*', route => route.fulfill({ headers, json: { state: 'ready', cities: [city] } }))
  return calls
}
async function picker(page: Page) {
  await trigger(page).click(); await expect(popup(page)).toHaveCSS('opacity', '1')
  await popup(page).getByRole('button', { name: 'Choose city', exact: true }).click()
  await expect(popup(page).getByRole('combobox')).toBeFocused()
}

test('weather UX: device permission is gesture-only, rounded POST resolves ID, weather persists only ID', async ({ page, context }) => {
  await context.grantPermissions(['geolocation']); await context.setGeolocation({ latitude: 30.671234, longitude: 104.071234, accuracy: 100 })
  const calls = await fixture(page), positions: unknown[] = []
  await page.route('**/api/v1/start/cities/nearest', route => {
    expect(route.request().method()).toBe('POST'); expect(new URL(route.request().url()).search).toBe('')
    expect(route.request().headers()['x-forwarded-for']).toBeUndefined()
    positions.push(route.request().postDataJSON())
    return route.fulfill({ headers, json: { state: 'ready', city } })
  })
  await page.goto(path); await expect(trigger(page)).toContainText('22.5°C')
  await picker(page); expect(positions).toHaveLength(0)
  await expect(locate(page)).toHaveAttribute('aria-describedby', /.+/)
  await expect(popup(page)).toContainText('Precise location is not saved.')
  const backBox = await popup(page).getByRole('button', { name: 'Back to weather' }).boundingBox(), locateBox = await locate(page).boundingBox()
  expect(Math.abs(backBox!.y - locateBox!.y)).toBeLessThan(1); expect(backBox!.x + backBox!.width).toBeLessThan(locateBox!.x)
  await locate(page).click()
  await expect(popup(page).getByRole('button', { name: 'Choose city', exact: true })).toBeFocused()
  expect(positions).toEqual([{ latitude: 30.67, longitude: 104.07 }])
  expect(new URL(calls.at(-1)!).searchParams.toString()).toBe('lang=en&city_id=1815286')
  expect(await page.evaluate(k => localStorage.getItem(k), key)).toBe(city.city_id)
  await expect(popup(page)).toContainText('Clear')
  await expect(popup(page).getByRole('link', { name: 'example.com' })).toHaveAttribute('href', 'https://example.com/license')
  await expect(popup(page)).toContainText('Required attribution statement.')
  await expect(popup(page)).not.toContainText('https://')
  await popup(page).getByRole('button', { name: 'Choose city', exact: true }).click()
  await expect(popup(page).getByRole('link', { name: 'GeoNames', exact: true })).toBeVisible()
  await expect(popup(page).getByRole('link', { name: 'DB-IP', exact: true })).toHaveCount(0)
  await popup(page).getByRole('button', { name: 'Use network location' }).click()
  expect(await page.evaluate(k => localStorage.getItem(k), key)).toBeNull()
  expect(new URL(calls.at(-1)!).searchParams.toString()).toBe('lang=en')
})

test('weather UX: denied, timeout, unsupported and failed matches preserve selected city and weather', async ({ page }) => {
  test.setTimeout(60000)
  await page.addInitScript(k => {
    localStorage.setItem(k, '1815286')
    Object.defineProperty(navigator, 'geolocation', { configurable: true, value: { getCurrentPosition(success: PositionCallback, error: PositionErrorCallback) {
      const mode = (window as unknown as { geoMode: string }).geoMode
      if (mode === 'denied' || mode === 'timeout') error({ code: mode === 'denied' ? 1 : 3 } as GeolocationPositionError)
      else success({ coords: { latitude: 30.67, longitude: 104.07 } } as GeolocationPosition)
    } } })
  }, key)
  const calls = await fixture(page)
  let mode = 'empty'
  await page.route('**/api/v1/start/cities/nearest', route => mode === 'failed' ? route.fulfill({ status: 429, headers, json: {} }) : mode === 'network' ? route.abort() :
    mode === 'region' ? route.fulfill({ headers: { ...headers, 'X-GoFurry-Region': 'OTHER' }, json: { state: 'ready', city } }) :
    route.fulfill({ headers, json: { state: 'unavailable', city: null } }))
  await page.goto(path); await picker(page)
  for (const [value, message] of [['denied', 'Location permission denied.'], ['timeout', 'Location timed out.'], ['empty', 'No suitable nearby city.'], ['failed', 'Location unavailable.'], ['network', 'Location unavailable.'], ['region', 'Location unavailable.'], ['unsupported', 'Device location is not supported']]) {
    mode = value
    await page.evaluate(value => {
      ;(window as unknown as { geoMode: string }).geoMode = value
      if (value === 'unsupported') Object.defineProperty(navigator, 'geolocation', { configurable: true, value: undefined })
    }, value)
    await locate(page).click(); await expect(popup(page).getByRole('status')).toContainText(message)
    await expect(locate(page)).toBeEnabled(); await expect(trigger(page)).toContainText('22.5°C')
    expect(await page.evaluate(k => localStorage.getItem(k), key)).toBe(city.city_id)
  }
  expect(calls).toHaveLength(1)
  await popup(page).getByRole('combobox').press('Escape')
  await expect(popup(page).getByRole('button', { name: 'Choose city', exact: true })).toBeFocused()
})

test('weather UX: late permission callbacks, unanswered prompts and closing cannot replace a manual choice', async ({ page }) => {
  await page.addInitScript(() => {
    Object.defineProperty(navigator, 'geolocation', { value: { getCurrentPosition(success: PositionCallback) {
      ;(window as unknown as { lateLocation: () => void }).lateLocation = () => success({ coords: { latitude: 30.67, longitude: 104.07 } } as GeolocationPosition)
    } } })
  })
  const calls = await fixture(page); let positions = 0
  await page.route('**/api/v1/start/cities/nearest', route => { positions++; return route.fulfill({ headers, json: { state: 'ready', city } }) })
  await page.goto(path); await picker(page); await locate(page).click(); await expect(locate(page)).toBeDisabled()
  await popup(page).getByRole('combobox').fill('Chengdu')
  await expect(popup(page).getByRole('option')).toBeVisible(); await popup(page).getByRole('combobox').press('Enter')
  await expect(popup(page).getByRole('button', { name: 'Choose city', exact: true })).toBeFocused()
  await page.evaluate(() => (window as unknown as { lateLocation: () => void }).lateLocation()); expect(positions).toBe(0)
  await popup(page).getByRole('button', { name: 'Choose city', exact: true }).click()
  await locate(page).click(); await page.mouse.click(4, 4)
  await expect(popup(page)).toHaveCount(0)
  await page.evaluate(() => (window as unknown as { lateLocation: () => void }).lateLocation())
  expect(positions).toBe(0); expect(calls).toHaveLength(2)
  expect(await page.evaluate(k => localStorage.getItem(k), key)).toBe(city.city_id)
  await picker(page)
  // Advance only the permission deadline; Motion exits are checked on real time.
  await page.clock.install(); await locate(page).click(); await page.clock.fastForward(12001)
  await expect(popup(page).getByRole('status')).toContainText('Location timed out.')
  await page.evaluate(() => (window as unknown as { lateLocation: () => void }).lateLocation()); expect(positions).toBe(0)
})

test('weather UX: local official day/night icons and neutral fallback follow currentColor', async ({ page }) => {
  const codes = ['100', '150', '350', '456', '998']
  await fixture(page, [...codes])
  for (const code of codes) {
    await page.goto(path); await expect(trigger(page).locator('svg')).toHaveAttribute('data-weather-icon', code)
    const resolved = code === '998' ? '999' : code
    await expect(trigger(page).locator('use')).toHaveAttribute('href', `/weather/qweather-icons.svg#weather-${resolved}`)
    await expect(trigger(page).locator('svg')).toHaveCSS('fill', 'rgb(215, 220, 226)')
    const size = await trigger(page).locator('use').evaluate(el => { const r = (el as SVGGraphicsElement).getBBox(); return [r.width, r.height] })
    expect(size[0]).toBeGreaterThan(0); expect(size[1]).toBeGreaterThan(0)
  }
  const response = await page.request.get('/weather/qweather-icons.svg'), source = await response.text()
  expect(response.ok()).toBe(true); expect(source.match(/<symbol /g)).toHaveLength(62)
  expect(source).not.toMatch(/<script|<image|(?:href|src)=["']https?:/)
})

test('weather UX: focus, fine native scrollbar, themes, bilingual small layouts and no wallpaper reload', async ({ page }, testInfo) => {
  test.setTimeout(90000)
  await fixture(page)
  await page.route('**/api/v1/start/cities?*', route => route.fulfill({ headers, json: { state: 'ready', cities: Array.from({ length: 10 }, (_, i) => ({ ...city, city_id: String(200000 + i), admin: `Region ${i}` })) } }))
  const images: string[] = []
  page.on('request', req => { if (/background|\.(webp|avif)/.test(req.url())) images.push(req.url()) })
  await page.goto(path); await page.waitForLoadState('networkidle')
  const imageCount = images.length
  for (const scheme of ['light', 'dark'] as const) {
    await page.emulateMedia({ colorScheme: scheme }); await picker(page)
    const input = popup(page).getByRole('combobox')
    await expect(input).toHaveCSS('outline-style', 'none'); await expect(input).toHaveCSS('box-shadow', 'none')
    await expect(input).toHaveCSS('border-top-width', '0px'); await expect(input).toHaveCSS('transition-duration', '0.42s')
    await page.waitForTimeout(450); const focused = await input.evaluate(el => getComputedStyle(el).backgroundColor)
    await popup(page).getByRole('button', { name: 'Back to weather' }).focus(); await page.mouse.move(4, 400)
    await expect(input).not.toHaveCSS('background-color', focused)
    await input.focus(); await input.fill('Chengdu'); await expect(popup(page).getByRole('option')).toHaveCount(10)
    const list = popup(page).getByRole('listbox')
    expect(await list.evaluate(el => getComputedStyle(el, '::-webkit-scrollbar').width)).toBe('4px')
    expect(await list.evaluate(el => getComputedStyle(el, '::-webkit-scrollbar-track').backgroundColor)).toBe('rgba(0, 0, 0, 0)')
    await page.mouse.move(4, 400); await page.waitForTimeout(350)
    const [steps] = await Promise.all([
      list.evaluate(el => new Promise<number>(resolve => {
        const values = new Set<string>(), end = performance.now() + 420
        const sample = () => {
          values.add(getComputedStyle(el).getPropertyValue('--weather-scrollbar-alpha'))
          if (performance.now() < end) requestAnimationFrame(sample); else resolve(values.size)
        }
        requestAnimationFrame(sample)
      })), list.hover(),
    ])
    expect(steps).toBeGreaterThan(2)
    await list.hover(); await page.mouse.wheel(0, 120); await expect.poll(() => list.evaluate(el => el.scrollTop)).toBeGreaterThan(0)
    await page.mouse.move(4, 400)
    const index = await list.getByRole('option').evaluateAll(options => options.findIndex(el => el.getAttribute('aria-selected') === 'true'))
    await input.press('ArrowUp'); await expect(list.getByRole('option').nth((index + 9) % 10)).toHaveAttribute('aria-selected', 'true')
    for (const [width, height] of [[320,480], [375,667], [390,844], [768,360], [1280,720]]) {
      await page.setViewportSize({ width, height })
      // Base UI applies ResizeObserver/positioner measurements on the next frame.
      await expect.poll(async () => {
        const r = (await popup(page).boundingBox())!
        return r.x >= 16 && r.x+r.width <= width-16 && r.y+r.height <= height
      }, { timeout: 1000 }).toBe(true)
      expect(await page.evaluate(() => [document.documentElement.scrollWidth, document.documentElement.scrollHeight])).toEqual([width,height])
    }
    await page.screenshot({ path: testInfo.outputPath(`city-picker-${scheme}.png`) })
    await input.press('Escape'); await expect(popup(page).getByRole('button', { name: 'Choose city', exact: true })).toBeFocused()
    await page.keyboard.press('Escape'); await expect(popup(page)).toHaveCount(0)
  }
  expect(images).toHaveLength(imageCount)
  await page.goto(chinese); await pickerChinese(page)
  await expect(popup(page).getByRole('button', { name: '定位当前位置' })).toBeVisible()
  await expect(popup(page).getByRole('button', { name: '使用网络位置' })).toBeVisible()
  await page.emulateMedia({ reducedMotion: 'reduce' }); await expect(popup(page).getByRole('combobox')).toHaveCSS('transition-duration', '0s')
})
async function pickerChinese(page: Page) {
  await trigger(page).click(); await expect(popup(page)).toHaveCSS('opacity', '1')
  await popup(page).getByRole('button', { name: '选择城市', exact: true }).click(); await expect(popup(page).getByRole('combobox')).toBeFocused()
}

test('weather UX: returning from city selection resizes continuously without an exit-frame snap', async ({ page }, testInfo) => {
  test.setTimeout(60000)
  await fixture(page, ['150'], ['QWeather'])
  await page.route('**/api/v1/start/cities?*', route => route.fulfill({ headers, json: { state: 'ready', cities: Array.from({ length: 10 }, (_, i) => ({ ...city, city_id: String(200000 + i) })) } }))
  for (const [url, width, height, scheme] of [[path, 1280, 720, 'light'], [chinese, 375, 667, 'dark']] as const) {
    const zh = url === chinese
    await page.setViewportSize({ width, height }); await page.emulateMedia({ colorScheme: scheme })
    await page.goto(url); await expect(trigger(page)).toContainText('22.5°C')
    await trigger(page).click(); await expect(popup(page)).toHaveCSS('opacity', '1')
    const choose = popup(page).getByRole('button', { name: zh ? '选择城市' : 'Choose city', exact: true })
    const detailHeight = (await popup(page).boundingBox())!.height
    for (const withResults of [false, true]) {
      await choose.click()
      const input = popup(page).getByRole('combobox'); await expect(input).toBeFocused()
      if (withResults) { await input.fill('Chengdu'); await expect(popup(page).getByRole('option')).toHaveCount(10) }
      // Wait for the current height animation to settle before measuring the return.
      await expect(popup(page).locator('[data-weather-view]')).toHaveCount(1)
      await expect.poll(() => popup(page).evaluate(el => Math.abs(el.querySelector('[data-weather-views]')!.getBoundingClientRect().height - el.querySelector('[data-weather-view]')!.getBoundingClientRect().height))).toBeLessThan(0.1)
      const back = popup(page).getByRole('button', { name: zh ? '返回天气' : 'Back to weather', exact: true })
      const before = (await popup(page).boundingBox())!
      const [frames] = await Promise.all([
        popup(page).evaluate(el => new Promise<{ t: number; h: number; x: number; y: number; w: number; layers: number; opacity: number }[]>(resolve => {
          const frames: { t: number; h: number; x: number; y: number; w: number; layers: number; opacity: number }[] = []
          let start: number | undefined
          el.addEventListener('click', () => { start = performance.now() }, { once: true })
          const sample = () => {
            const r = el.getBoundingClientRect()
            if (start !== undefined) frames.push({ t: performance.now() - start, h: r.height, x: r.x, y: r.y, w: r.width, layers: el.querySelectorAll('[data-weather-view]').length, opacity: Number(getComputedStyle(el).opacity) })
            if (start === undefined || performance.now() - start < 700) requestAnimationFrame(sample)
            else resolve(frames)
          }
          requestAnimationFrame(sample)
        })), back.click(),
      ])
      await testInfo.attach(`return-${width}-${withResults ? 'results' : 'empty'}-frames`, { body: JSON.stringify(frames), contentType: 'application/json' })
      const distance = before.height - detailHeight
      expect(distance).toBeGreaterThan(10)
      const middle = frames.find(f => f.t >= 150)!
      expect(middle.h).toBeLessThan(before.height - distance * 0.4)
      expect(frames.filter(f => f.h < before.height - 1 && f.h > detailHeight + 1).length).toBeGreaterThan(3)
      for (let i = 1; i < frames.length; i++) {
        expect(frames[i].h).toBeLessThanOrEqual(frames[i - 1].h + 0.5)
        expect(frames[i].h).toBeGreaterThanOrEqual(detailHeight - 0.5)
        // The old grid snapped when its taller exiting child unmounted at 420ms.
        if (frames[i].t > 350 && frames[i].t - frames[i - 1].t < 80) expect(Math.abs(frames[i].h - frames[i - 1].h)).toBeLessThan(1)
        expect(Math.abs(frames[i].x - before.x)).toBeLessThan(0.5)
        expect(Math.abs(frames[i].y - before.y)).toBeLessThan(0.5)
        expect(Math.abs(frames[i].w - before.width)).toBeLessThan(0.5)
        expect(frames[i].opacity).toBe(1)
      }
      expect(Math.abs(frames.at(-1)!.h - detailHeight)).toBeLessThan(0.1)
      await expect(choose).toBeFocused()
      await expect(popup(page).locator('[data-weather-view]')).toHaveCount(1)
      expect(await popup(page).evaluate(el => el.scrollTop)).toBe(0)
      expect(await page.evaluate(() => [document.documentElement.scrollWidth, document.documentElement.scrollHeight])).toEqual([width, height])
    }
    await page.screenshot({ path: testInfo.outputPath(`weather-return-${width}-${scheme}.png`) })
    // Keyboard return and reduced-motion must not leave a frozen explicit height.
    await page.emulateMedia({ reducedMotion: 'reduce' }); await page.reload()
    await expect(trigger(page)).toContainText('22.5°C')
    await trigger(page).click(); await expect(popup(page)).toHaveCSS('opacity', '1'); await choose.click()
    await expect(popup(page).getByRole('combobox')).toBeFocused()
    await popup(page).getByRole('combobox').press('Escape'); await expect(choose).toBeFocused()
    await expect(popup(page).locator('[data-weather-city-picker]')).toHaveCount(0)
    await expect.poll(async () => Math.abs((await popup(page).boundingBox())!.height - detailHeight), { timeout: 200, intervals: [16, 32] }).toBeLessThan(0.1)
    await page.keyboard.press('Escape'); await expect(popup(page)).toHaveCount(0); await expect(trigger(page)).toBeFocused()
    await page.emulateMedia({ reducedMotion: 'no-preference' })
  }
})

test('weather UX: real same-origin nearest API is bounded and regional', async ({ request }) => {
  test.skip(process.env.START_WEATHER_LIVE !== '1', 'Optional real local API check; no weather provider quota is used.')
  const response = await request.post('/api/v1/start/cities/nearest', { data: { latitude: 30.67, longitude: 104.07 } })
  if (region === 'INTL') { expect(response.status()).toBe(503); return }
  expect(response.status()).toBe(200); expect(response.headers()['x-gofurry-region']).toBe(region)
  expect(response.headers()['cache-control']).toBe('private, no-store')
  expect(await response.json()).toMatchObject({ state: 'ready', city: { city_id: '1815286' } })
  const ocean = await request.post('/api/v1/start/cities/nearest', { data: { latitude: 0, longitude: 0 } })
  expect(await ocean.json()).toEqual({ state: 'unavailable', city: null })
  const invalid = await request.post('/api/v1/start/cities/nearest', { data: { latitude: 91, longitude: 0 } })
  expect(invalid.status()).toBe(400)
})

test('weather UX: real catalog/device-match/weather flow and screenshots', async ({ page, context }, testInfo) => {
  test.skip(process.env.START_WEATHER_LIVE !== '1' || region !== 'CN', 'Uses the private local CN development API; no cross-region requests.')
  test.setTimeout(60000)
  // Only the browser's device fix is simulated. Catalog, weather and wallpaper
  // are real local development services, with no route fixtures.
  await context.grantPermissions(['geolocation']); await context.setGeolocation({ latitude: 30.67, longitude: 104.07, accuracy: 500 })
  const errors: string[] = [], images: string[] = []
  page.on('pageerror', error => errors.push(error.message))
  page.on('request', req => { if (/background|\.(webp|avif)/.test(req.url())) images.push(req.url()) })
  await page.goto(chinese)
  await expect(page.locator('[data-start-hd]')).toHaveCSS('opacity', '1', { timeout: 20000 })
  const count = images.length
  await pickerChinese(page); await popup(page).getByRole('button', { name: '定位当前位置' }).click()
  await expect(trigger(page)).toHaveAttribute('data-weather-state', 'ready', { timeout: 10000 })
  await expect(popup(page).getByRole('button', { name: '选择城市', exact: true })).toBeFocused()
  await expect(page.locator('[data-weather-city-picker]')).toHaveCount(0)
  await expect(popup(page)).toContainText('成都')
  expect(await page.evaluate(k => localStorage.getItem(k), key)).toBe(city.city_id)
  const first = await page.request.get('/api/v1/start/weather?lang=zh&city_id=1815286')
  const second = await page.request.get('/api/v1/start/weather?lang=zh&city_id=1815286')
  const a = await first.json(), b = await second.json()
  expect(first.status()).toBe(200); expect(second.status()).toBe(200)
  expect(a.state).toBe('ready'); expect(a.location_source).toBe('manual'); expect(b.weather.fetched_at).toBe(a.weather.fetched_at)
  for (const scheme of ['light', 'dark'] as const) {
    await page.emulateMedia({ colorScheme: scheme }); await page.setViewportSize({ width: 1280, height: 720 })
    await expect(page.locator('html')).toHaveAttribute('data-theme', scheme)
    await expect(popup(page).getByRole('link').first()).toHaveCSS('color', await popup(page).evaluate(el => getComputedStyle(el).color))
    await page.screenshot({ path: testInfo.outputPath(`real-weather-${scheme}.png`), animations: 'disabled' })
  }
  await popup(page).getByRole('button', { name: '选择城市', exact: true }).click()
  await page.setViewportSize({ width: 390, height: 844 })
  await popup(page).getByRole('combobox').fill('Springfield')
  await expect(popup(page).getByRole('option')).toHaveCount(10)
  await expect(popup(page).locator('[inert]')).toHaveCount(0)
  await page.screenshot({ path: testInfo.outputPath('real-city-mobile.png'), animations: 'disabled' })
  expect(await page.evaluate(() => [document.documentElement.scrollWidth, document.documentElement.scrollHeight])).toEqual([390,844])
  expect(images).toHaveLength(count); expect(errors).toEqual([])
})
