import { test, expect } from '@playwright/test'

const region = process.env.WEB_REGION ?? 'CN'
const initialLocale = region === 'CN' ? 'zh' : 'en'

test('governance: deployment owns region; both locales are available without changing it', async ({ page, request }) => {
  const html = await (await request.get('/')).text()
  expect(html).toContain(`lang="${initialLocale}"`)
  expect(html).toContain(`data-region="${region}"`)
  expect(html).toContain('data-content-mode="SFW"')
  expect(html).toContain('data-theme="light"')
  await page.goto('/')
  await page.waitForLoadState('networkidle')
  for (const locale of ['en', 'zh']) {
    await page.getByLabel('Language', { exact: true }).selectOption(locale)
    await expect(page.locator('html')).toHaveAttribute('lang', locale)
    await expect(page.locator('html')).toHaveAttribute('data-region', region)
    await expect(page.locator('html')).toHaveAttribute('data-content-mode', 'SFW')
    await expect(page.getByRole('button', { name: locale === 'en' ? 'Check interaction' : '验证交互' })).toBeVisible()
    await page.reload()
    await expect(page.locator('html')).toHaveAttribute('lang', locale)
  }
})

test('governance: manual theme persists in initial SSR and hydrates without errors', async ({ page, browser }) => {
  const errors: string[] = []
  page.on('pageerror', (e) => errors.push(e.message))
  page.on('console', (e) => { if (e.type() === 'error') errors.push(e.text()) })
  await page.goto('/')
  await page.waitForLoadState('networkidle')
  await page.getByLabel('Theme', { exact: true }).selectOption('dark')
  await expect(page.locator('html')).toHaveCSS('background-color', 'rgb(29, 31, 34)')
  await page.reload()
  await expect(page.locator('html')).toHaveAttribute('data-theme', 'dark')
  const context = await browser.newContext({ javaScriptEnabled: false })
  await context.addCookies(await page.context().cookies())
  const ssr = await context.newPage()
  const response = await ssr.goto('/')
  expect(await response!.text()).toContain('data-theme="dark"')
  expect(response!.headers()['cache-control']).toContain('no-store')
  await expect(ssr.locator('html')).toHaveCSS('background-color', 'rgb(29, 31, 34)')
  await context.close()
  await page.getByLabel('Theme', { exact: true }).selectOption('light')
  await expect(page.locator('html')).toHaveCSS('background-color', 'rgb(242, 227, 208)')
  expect(errors).toEqual([])
})

test('governance: invalid preferences fall back without accepting a region cookie', async ({ browser }) => {
  const context = await browser.newContext()
  await context.addCookies(['gf-region', 'gf-locale', 'gf-theme'].map((name) => ({ name, value: 'invalid', url: 'http://127.0.0.1:17320' })))
  const page = await context.newPage()
  await page.goto('/')
  await expect(page.locator('html')).toHaveAttribute('data-region', region)
  await expect(page.locator('html')).toHaveAttribute('lang', initialLocale)
  await expect(page.locator('html')).toHaveAttribute('data-theme', 'light')
  await context.close()
})
