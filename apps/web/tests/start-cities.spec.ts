import { test, expect, type Page } from '@playwright/test'
import { sampleMotion } from './motion'

const region = process.env.WEB_REGION ?? 'CN'
const path = region === 'CN' ? '/en/start' : '/start'
const headers = { 'X-GoFurry-Region': region, 'X-GoFurry-Content-Mode': 'SFW' }
const weatherURL = '**/api/v1/start/weather?*', citiesURL = '**/api/v1/start/cities?*'
const storageKey = `gf-weather-city:${region}`
const popup = (page: Page) => page.locator('[data-start-weather-details]')
const trigger = (page: Page) => page.locator('[data-start-weather]')
const chengdu = { city_id: '1815286', name: 'Chengdu', names: { zh: '成都', en: 'Chengdu' }, admin: 'Sichuan', country_code: 'CN' }
const springfields = [
  { city_id: '4409896', name: 'Springfield', names: { zh: '斯普林菲尔德', en: 'Springfield' }, admin: 'Missouri', country_code: 'US' },
  { city_id: '4250542', name: 'Springfield', names: { zh: '斯普林菲尔德', en: 'Springfield' }, admin: 'Illinois', country_code: 'US' },
]

async function fixtures(page: Page, calls: (string | null)[] = []) {
  await page.route(weatherURL, route => {
    const id = new URL(route.request().url()).searchParams.get('city_id'); calls.push(id)
    const city = [chengdu, ...springfields].find(city => city.city_id === id)
    return route.fulfill({ headers, json: { state: city ? 'ready' : 'unavailable', location_source: id ? 'manual' : 'ip', ...(id ? { city_id: id } : {}),
      location: city ? { city: city.names, country_code: city.country_code } : null,
      weather: city ? { temperature_c: 22.5, condition_code: '101', condition_text: 'Cloudy', fetched_at: new Date().toISOString() } : null,
      attribution: { weather_attributions: city ? ['QWeather'] : [] } } })
  })
  await page.route(citiesURL, route => {
    const q = new URL(route.request().url()).searchParams.get('q')!
    return route.fulfill({ headers, json: { state: 'ready', cities: q.toLowerCase().includes('spring') ? springfields : q === 'empty' ? [] : [chengdu] } })
  })
}
async function open(page: Page) { await trigger(page).click(); await expect(popup(page)).toHaveCSS('opacity', '1') }
async function picker(page: Page) {
  await popup(page).getByRole('button', { name: 'Choose city', exact: true }).click()
  await expect(popup(page).getByRole('combobox')).toBeFocused()
  return popup(page).getByRole('combobox')
}

test('cities: three transparent header triggers stay bright in all themes; unavailable has no temperature slot', async ({ page }) => {
  await fixtures(page); await page.goto(path)
  for (const scheme of ['light', 'dark'] as const) {
    await page.emulateMedia({ colorScheme: scheme }); await page.mouse.move(400, 400)
    for (const control of await page.locator('[data-start-actions] > button').all()) {
      await expect(control).toHaveCSS('color', 'rgb(215, 220, 226)')
      await expect(control).toHaveCSS('background-color', 'rgba(0, 0, 0, 0)')
      await expect(control).toHaveCSS('box-shadow', 'none'); await expect(control).toHaveCSS('border-top-width', '0px')
      await control.hover(); await expect(control).toHaveCSS('background-color', 'rgba(224, 230, 238, 0.14)')
      await expect(control).toHaveCSS('transition-duration', '0.3s'); await page.mouse.move(400, 400)
    }
    await expect(trigger(page)).toHaveText('Weather')
    await expect(trigger(page).locator('span')).toHaveCount(1)
    await open(page); await expect(popup(page)).toContainText('Weather unavailable')
    await expect(popup(page)).not.toHaveCSS('color', 'rgb(215, 220, 226)')
    await page.keyboard.press('Escape'); await expect(popup(page)).toHaveCount(0)
  }
  await page.locator('[data-start-settings]').click(); await page.getByRole('radio', { name: 'Light', exact: true }).click()
  await page.keyboard.press('Escape'); await expect(page.locator('[data-start-preferences]')).toHaveCount(0)
  await expect(trigger(page)).toHaveCSS('color', 'rgb(215, 220, 226)')
  await trigger(page).focus(); await page.keyboard.press('Tab'); await expect(page.locator('[data-start-settings]')).toBeFocused()
  expect(await page.locator('[data-start-settings]').evaluate(el => el.matches(':focus-visible'))).toBe(true)
})

test('cities: debounce, IME, keyboard selection, ID-only persistence, new tab and automatic reset', async ({ page, context }) => {
  const calls: (string | null)[] = [], queries: string[] = []
  await fixtures(page, calls)
  await page.route(citiesURL, route => { queries.push(new URL(route.request().url()).searchParams.get('q')!); return route.fulfill({ headers, json: { state: 'ready', cities: [chengdu] } }) })
  await page.goto(path); await open(page); const input = await picker(page)
  await input.dispatchEvent('compositionstart'); await input.fill('成都'); await page.waitForTimeout(350); expect(queries).toEqual([])
  await input.dispatchEvent('compositionend'); await expect(popup(page).getByRole('option')).toContainText('Chengdu')
  expect(queries).toEqual(['成都']); await expect(popup(page).getByRole('option')).toContainText('Sichuan · CN')
  await input.press('Enter'); await expect(trigger(page)).toContainText('22.5°C')
  await expect(popup(page).getByRole('button', { name: 'Choose city', exact: true })).toBeFocused()
  await expect(popup(page).getByRole('link', { name: 'GeoNames', exact: true })).toHaveAttribute('href', 'https://www.geonames.org')
  expect(await page.evaluate(key => localStorage.getItem(key), storageKey)).toBe('1815286')
  const before = calls.length
  await page.reload(); await expect(trigger(page)).toContainText('22.5°C'); expect(calls.slice(before)).toEqual(['1815286'])
  const another = await context.newPage(), tabCalls: (string | null)[] = []
  await fixtures(another, tabCalls); await another.goto(path); await expect(trigger(another)).toContainText('22.5°C'); expect(tabCalls).toEqual(['1815286']); await another.close()
  await open(page); await picker(page); await popup(page).getByRole('button', { name: 'Automatic location' }).click()
  await expect(trigger(page)).toHaveText('Weather'); await expect(popup(page)).toContainText('Weather unavailable')
  expect(await page.evaluate(key => localStorage.getItem(key), storageKey)).toBeNull(); expect(calls.at(-1)).toBeNull()
  await expect(popup(page).getByRole('button', { name: 'Choose city', exact: true })).toBeFocused()
  await page.keyboard.press('Escape'); await expect(popup(page)).toHaveCount(0); await expect(trigger(page)).toBeFocused()
})

test('cities: disambiguation, cancellation, Escape, empty and failed searches keep one popup', async ({ page }) => {
  await fixtures(page)
  const calls: string[] = []
  await page.route(citiesURL, async route => {
    const q = new URL(route.request().url()).searchParams.get('q')!; calls.push(q)
    if (q === 'old') await new Promise(resolve => setTimeout(resolve, 1000))
    await route.fulfill(q === 'failed' ? { status: 429, headers, json: {} } : { headers, json: { state: 'ready', cities: q === 'old' ? [chengdu] : q === 'empty' ? [] : springfields } }).catch(() => {})
  })
  await page.goto(path); await open(page); const input = await picker(page)
  await input.fill('old'); await expect.poll(() => calls.includes('old')).toBe(true)
  await input.fill('Springfield'); await expect(popup(page).getByRole('option')).toHaveCount(2)
  await expect(popup(page)).toContainText('Missouri · US'); await expect(popup(page)).toContainText('Illinois · US')
  await input.press('ArrowDown'); await input.press('Enter'); await expect(trigger(page)).toContainText('22.5°C')
  expect(await page.evaluate(key => localStorage.getItem(key), storageKey)).toBe('4250542')
  const again = await picker(page)
  for (const q of ['empty', 'failed']) { await again.fill(q); await expect(popup(page).getByRole('status')).toHaveText(q === 'empty' ? 'No matching cities' : 'City search unavailable'); await expect(popup(page).getByRole('option')).toHaveCount(0) }
  await again.press('Escape'); await expect(popup(page)).toBeVisible(); await expect(popup(page).getByRole('button', { name: 'Choose city', exact: true })).toBeFocused()
  await expect(page.locator('[data-weather-city-picker]')).toHaveCount(0); await expect(page.getByRole('alert')).toHaveCount(0)
})

test('cities: mobile popup, complete exit and screenshots', async ({ page }, testInfo) => {
  test.setTimeout(60000)
  await fixtures(page); await page.goto(path)
  for (const [width, height] of [[320,480], [375,667], [390,844], [768,360], [1280,720]]) {
    await page.setViewportSize({ width, height }); await open(page); await picker(page)
    await popup(page).getByRole('combobox').fill('Springfield'); await expect(popup(page).getByRole('option')).toHaveCount(2)
    const bounds = (await popup(page).boundingBox())!
    expect(bounds.x).toBeGreaterThanOrEqual(16); expect(bounds.x+bounds.width).toBeLessThanOrEqual(width-16)
    expect(bounds.y+bounds.height).toBeLessThanOrEqual(height)
    expect(await page.evaluate(() => [document.documentElement.scrollWidth,document.documentElement.scrollHeight])).toEqual([width,height])
    await popup(page).getByRole('combobox').press('Escape'); await expect(popup(page).getByRole('button', { name: 'Choose city', exact: true })).toBeFocused()
    await page.keyboard.press('Escape'); const exit = await sampleMotion(popup(page)); expect(exit.some(f=>f.opacity>0&&f.opacity<1)).toBe(true)
    await expect(popup(page)).toHaveCount(0); await expect(trigger(page)).toBeFocused()
  }
  await page.emulateMedia({ reducedMotion: 'reduce' }); await open(page); await picker(page); await page.keyboard.press('Escape')
  await expect(page.locator('[data-weather-city-picker]')).toHaveCount(0)
  await page.keyboard.press('Escape'); await expect(popup(page)).toHaveCount(0)
  await page.screenshot({ path: testInfo.outputPath('header.png') })
})
