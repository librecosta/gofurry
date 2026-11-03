import { test, expect } from '@playwright/test'

// Opt-in alongside real backgrounds; requires enabled YAML suggestions and, for
// CN, reviewed golang phrases. No upstream results are mocked in this case.
test('live: regional Go DuckDuckGo suggestions and transparent desktop/mobile views', async ({ page, request }, testInfo) => {
  const api = process.env.START_API_URL ?? 'http://127.0.0.1:17321'
  const response = await request.get(`${api}/api/v1/start/search/suggestions?q=golang`)
  expect(response.status(), 'Check explicit YAML enabled/proxy_url/CN allowlist and developer network').toBe(200)
  const body = await response.json() as { suggestions: string[] }
  expect(body.suggestions.length, 'CN is disabled or has no reviewed upstream matches').toBeGreaterThan(0)
  await page.setViewportSize({ width: 1440, height: 900 })
  await page.goto('/start')
  await expect(page.locator('[data-start-hd]')).toHaveCSS('opacity', '1', { timeout: 20000 })
  const field = page.locator('input[name="q"]')
  await field.fill('golang')
  const panel = page.locator('[data-search-panel="suggestions"][data-open="true"]')
  await expect(panel).toHaveCSS('opacity', '1', { timeout: 8000 })
  await expect(panel.getByRole('option')).toHaveText(body.suggestions)
  await page.screenshot({ path: testInfo.outputPath('desktop-light-suggestions.png') })
  await field.press('Escape'); await field.fill(''); await page.mouse.click(4, 500); await page.mouse.move(0, 0)
  await page.screenshot({ path: testInfo.outputPath('desktop-light.png') })
  await page.locator('[data-start-actions] > button').nth(1).click()
  await expect(page.locator('html')).toHaveAttribute('data-theme', 'dark')
  await expect(page.locator('html')).not.toHaveAttribute('data-gf-theme-reveal')
  await page.mouse.move(0, 0)
  await page.screenshot({ path: testInfo.outputPath('desktop-dark.png') })
  for (const width of [390, 375]) {
    await page.setViewportSize({ width, height: width === 390 ? 844 : 667 })
    await page.locator('[data-mobile-platform-trigger]').click()
    const menu = page.locator('[data-search-panel="platforms"]')
    await expect(menu).toHaveCSS('opacity', '1')
    await page.screenshot({ path: testInfo.outputPath(`mobile-${width}-platforms.png`) })
    await page.keyboard.press('Escape')
  }
})
