import { test, expect } from '@playwright/test'

test('SSR sends metadata, content and extracted CSS before hydration', async ({ request, browser }) => {
  const response = await request.get('/')
  expect(response.status()).toBe(200)
  const html = await response.text()
  expect(html).toContain('<title>GoFurry · Runtime skeleton</title>')
  expect(html).toContain('name="description"')
  expect(html).toContain('GoFurry runtime skeleton')
  const cssURLs = [...html.matchAll(/<link[^>]+href="([^"]+\.css)"/g)].map((m) => m[1])
  expect(cssURLs.length).toBeGreaterThan(0)
  let css = ''
  for (const url of cssURLs) {
    const asset = await request.get(url)
    expect(asset.status()).toBe(200)
    css += await asset.text()
  }
  expect(css).toMatch(/min-height:\s*100vh/)
  expect(css).toContain('#f2e3d0')
  expect(css).toContain('#1d1f22')
  const context = await browser.newContext({ javaScriptEnabled: false, colorScheme: 'light' })
  const page = await context.newPage()
  await page.goto('/')
  await expect(page.locator('main')).toHaveCSS('padding-top', '32px')
  await expect(page.locator('html')).toHaveCSS('background-color', 'rgb(242, 227, 208)')
  await context.close()
})

for (const colorScheme of ['light', 'dark'] as const) {
  test(`hydration, Popover keyboard/focus, navigation and ${colorScheme} theme`, async ({ page }) => {
    await page.context().addCookies([{ name: 'gf-theme', value: colorScheme, url: 'http://127.0.0.1:17320' }])
    const errors: string[] = []
    page.on('pageerror', (e) => errors.push(e.message))
    page.on('console', (e) => { if (e.type() === 'error') errors.push(e.text()) })
    await page.goto('/')
    const trigger = page.getByRole('button', { name: '验证交互' })
    await expect(page.getByRole('button', { name: '禁用状态' })).toBeDisabled()
    await expect(page.locator('html')).toHaveCSS('background-color', colorScheme === 'dark' ? 'rgb(29, 31, 34)' : 'rgb(242, 227, 208)')
    await trigger.focus()
    await page.keyboard.press('Enter')
    const popup = page.getByRole('dialog')
    await expect(popup).toBeVisible()
    await expect(page.getByRole('button', { name: '关闭' })).toBeFocused()
    await page.keyboard.press('Escape')
    await expect(popup).toBeHidden()
    await expect(trigger).toBeFocused()
    await trigger.click()
    await expect(popup).toBeVisible()
    await page.getByRole('button', { name: '关闭' }).click()
    await expect(popup).toBeHidden()
    await expect(trigger).toBeFocused()
    await page.evaluate(() => { (window as unknown as { navigationMarker: boolean }).navigationMarker = true })
    await page.getByRole('link', { name: '运行状态' }).click()
    await expect(page.getByRole('heading', { name: 'Web SSR ready' })).toBeVisible()
    expect(await page.evaluate(() => (window as unknown as { navigationMarker: boolean }).navigationMarker)).toBe(true)
    await page.getByRole('link', { name: '返回' }).click()
    await expect(trigger).toBeVisible()
    expect(errors).toEqual([])
  })
}

test('health probes and direct SSR navigation', async ({ request }) => {
  for (const path of ['/livez', '/readyz']) expect((await request.get(path)).status()).toBe(200)
  expect(await (await request.get('/status')).text()).toContain('Web SSR ready')
  expect((await request.get('/does-not-exist')).status()).toBe(404)
})
