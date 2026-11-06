import { test, expect } from '@playwright/test'

// Opt-in development YAML/API integration. No suggestions or backgrounds are
// mocked; only outbound search navigation is intercepted.
test('live: identical regional suggestions, selected targets and compact weather sources', async ({ page, request }, testInfo) => {
  test.setTimeout(90000)
  const region = process.env.WEB_REGION ?? 'CN'
  const api = process.env.START_API_URL ?? 'http://127.0.0.1:17321'
  await page.setViewportSize({ width: 1440, height: 900 })
  await page.addInitScript(region => localStorage.setItem(`gf-weather-city:${region}`, '1815286'), region)
  await page.goto('/start')
  await expect(page.locator('[data-start-hd]')).toHaveCSS('opacity', '1', { timeout: 20000 })
  const image = await page.locator('[data-start-hd]').elementHandle()
  const field = page.locator('input[name="q"]')
  const panel = page.locator('[data-search-panel="suggestions"][data-open="true"]')
  const catalog = new Map<string, string[]>()
  for (const query of ['furry', '兽人', 'GoFurry']) {
    const response = await request.get(`${api}/api/v1/start/search/suggestions?q=${encodeURIComponent(query)}`)
    expect(response.status(), 'Check explicit YAML proxy and developer network; do not substitute mock results').toBe(200)
    expect(response.headers()['x-gofurry-region']).toBe(region)
    expect(response.headers()['x-gofurry-suggestions-state']).toBe('ready')
    const { suggestions } = await response.json() as { suggestions: string[] }
    expect(suggestions.length, 'Real upstream must produce usable suggestions').toBeGreaterThan(1)
    catalog.set(query, suggestions)
    await field.fill(query)
    await expect(panel).toHaveCSS('opacity', '1', { timeout: 8000 })
    await expect(panel.getByRole('option')).toHaveText(suggestions)
    await page.screenshot({ path: testInfo.outputPath(`${region}-desktop-${query}-suggestions.png`) })
    await field.press('Escape'); await expect(panel).toHaveCount(0)
  }
  await page.context().route('https://**/*', route => route.request().isNavigationRequest() ? route.fulfill({ contentType: 'text/html', body: '<title>Search target</title>' }) : route.continue())
  await field.fill('furry'); await expect(panel).toHaveCSS('opacity', '1')
  await field.press('ArrowDown')
  const opened = page.waitForEvent('popup'); await field.press('Enter'); const target = await opened
  await target.waitForLoadState('domcontentloaded')
  expect(new URL(target.url()).hostname).toBe('www.bing.com')
  expect(new URL(target.url()).searchParams.get('q')).toBe(catalog.get('furry')![1]); await target.close()
  await page.locator('[data-desktop-platforms]').getByRole('button', { name: region === 'CN' ? '百度' : 'Google', exact: true }).click()
  await field.fill('兽人'); await expect(panel).toHaveCSS('opacity', '1')
  const clicked = page.waitForEvent('popup'); await panel.getByRole('option').last().click(); const selected = await clicked
  await selected.waitForLoadState('domcontentloaded')
  expect(new URL(selected.url()).hostname).toBe(region === 'CN' ? 'www.baidu.com' : 'www.google.com')
  expect(new URL(selected.url()).searchParams.get(region === 'CN' ? 'wd' : 'q')).toBe(catalog.get('兽人')!.at(-1)); await selected.close()
  await page.locator('[data-search-clear]').click(); await page.mouse.click(4, 500)
  for (const theme of ['light', 'dark']) {
    await page.locator('[data-start-settings]').click()
    const dialog = page.locator('[data-start-preferences]'); await expect(dialog).toHaveCSS('opacity', '1')
    await page.getByRole('radio', { name: theme === 'dark' ? /^(Dark|深色)$/ : /^(Light|浅色)$/ }).click()
    await expect(page.locator('html')).toHaveAttribute('data-theme', theme)
    await expect(page.locator('html')).not.toHaveAttribute('data-gf-theme-reveal')
    await page.keyboard.press('Escape'); await expect(dialog).toHaveCount(0)
    await field.fill('furry'); await expect(panel).toHaveCSS('opacity', '1')
    await page.screenshot({ path: testInfo.outputPath(`${region}-desktop-${theme}-suggestions.png`) })
    await field.press('Escape'); await expect(panel).toHaveCount(0)
  }
  await expect(page.locator('[data-start-weather]')).toHaveAttribute('data-weather-state', /ready|stale/, { timeout: 10000 })
  await page.locator('[data-start-weather]').click()
  const weather = page.locator('[data-start-weather-details]')
  await expect(weather).toHaveCSS('opacity', '1')
  await expect(weather.getByRole('link', { name: region === 'CN' ? '和风天气 / QWeather' : 'QWeather', exact: true })).toHaveAttribute('href', 'https://www.qweather.com')
  await expect(weather).not.toContainText('developer.qweather.com')
  await expect(weather).not.toContainText('来源说明')
  await expect(weather).not.toContainText('Source details')
  await page.screenshot({ path: testInfo.outputPath(`${region}-desktop-weather-sources.png`) })
  await page.keyboard.press('Escape'); await expect(weather).toHaveCount(0)
  for (const width of [390, 375]) {
    await page.setViewportSize({ width, height: width === 390 ? 844 : 667 })
    await field.fill('GoFurry'); await expect(panel).toHaveCSS('opacity', '1')
    await page.screenshot({ path: testInfo.outputPath(`${region}-mobile-${width}-suggestions.png`) })
    await page.locator('[data-mobile-platform-trigger]').click()
    await expect(page.locator('[data-search-panel]')).toHaveCount(1)
    const picker = page.locator('[data-search-panel="platforms"]'); await expect(picker).toHaveCSS('opacity', '1')
    await page.screenshot({ path: testInfo.outputPath(`${region}-mobile-${width}-platforms.png`) })
    await page.keyboard.press('Escape'); await expect(picker).toHaveCount(0)
    expect(await page.evaluate(() => [document.documentElement, document.body].map(el => [el.scrollWidth, el.scrollHeight]))).toEqual([[width, width === 390 ? 844 : 667], [width, width === 390 ? 844 : 667]])
  }
  expect(await page.locator('[data-start-hd]').evaluate((el, first) => el === first, image)).toBe(true)
})
