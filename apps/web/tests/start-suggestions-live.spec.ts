import { test, expect } from '@playwright/test'

// Opt-in regional integration: CN requires reviewed golang phrases; INTL tests
// real furry suggestions without CN's exact review list. No results are mocked.
test('live: regional real suggestions, search submission and preferences screenshots', async ({ page, request }, testInfo) => {
  const region = process.env.WEB_REGION ?? 'CN'
  const api = process.env.START_API_URL ?? 'http://127.0.0.1:17321'
  const query = region === 'CN' ? 'golang' : 'furry'
  const response = await request.get(`${api}/api/v1/start/search/suggestions?q=${query}`)
  expect(response.status(), 'Check explicit YAML enabled/proxy_url/CN review list and developer network').toBe(200)
  const body = await response.json() as { suggestions: string[] }
  expect(body.suggestions.length, 'Disabled or no reviewed upstream matches').toBeGreaterThan(1)
  await page.setViewportSize({ width: 1440, height: 900 }); await page.goto('/start')
  await expect(page.locator('[data-start-hd]')).toHaveCSS('opacity', '1', { timeout: 20000 })
  const field = page.locator('input[name="q"]')
  await field.fill(query)
  const panel = page.locator('[data-search-panel="suggestions"][data-open="true"]')
  await expect(panel).toHaveCSS('opacity', '1', { timeout: 8000 }); await expect(panel.getByRole('option')).toHaveText(body.suggestions)
  await page.screenshot({ path: testInfo.outputPath('desktop-real-suggestions.png') })
  await page.keyboard.press('ArrowDown')
  await page.context().route('https://**/*', route => route.request().isNavigationRequest() ? route.fulfill({ contentType: 'text/html', body: '<title>Search target</title>' }) : route.continue())
  const opened = page.waitForEvent('popup'); await page.keyboard.press('Enter'); const target = await opened
  await target.waitForLoadState('domcontentloaded')
  const url = new URL(target.url())
  expect(url.hostname).toBe(region === 'CN' ? 'www.bing.com' : 'www.google.com'); expect(url.searchParams.get('q')).toBe(body.suggestions[1]); await target.close()
  await page.locator('[data-search-clear]').click(); await page.mouse.click(4, 500)
  for (const theme of ['light', 'dark']) {
    await page.locator('[data-start-settings]').click()
    const dialog = page.locator('[data-start-preferences]'); await expect(dialog).toHaveCSS('opacity', '1')
    await page.getByRole('radio', { name: theme === 'dark' ? /^(Dark|深色)$/ : /^(Light|浅色)$/ }).click()
    await expect(page.locator('html')).toHaveAttribute('data-theme', theme); await expect(page.locator('html')).not.toHaveAttribute('data-gf-theme-reveal')
    await page.screenshot({ path: testInfo.outputPath(`desktop-${theme}-settings.png`) })
    await page.keyboard.press('Escape'); await expect(dialog).toHaveCount(0); await page.mouse.click(4, 500)
    await page.screenshot({ path: testInfo.outputPath(`desktop-${theme}.png`) })
  }
  for (const width of [390, 320]) {
    await page.setViewportSize({ width, height: width === 390 ? 844 : 667 })
    await page.locator('[data-start-settings]').click(); await expect(page.locator('[data-start-preferences]')).toHaveCSS('opacity', '1')
    await page.screenshot({ path: testInfo.outputPath(`mobile-${width}-settings.png`) })
    await page.keyboard.press('Escape'); await expect(page.locator('[data-start-preferences]')).toHaveCount(0)
    await page.locator('[data-start-logo]').click(); await expect(page.locator('[data-start-brand]')).toHaveCSS('opacity', '1')
    await page.screenshot({ path: testInfo.outputPath(`mobile-${width}-navigation.png`) }); await page.keyboard.press('Escape')
    await expect(page.locator('[data-start-brand]')).toHaveCount(0)
    expect(await page.evaluate(() => [document.documentElement.scrollWidth, document.documentElement.scrollHeight])).toEqual([width, width === 390 ? 844 : 667])
  }
})
