import { test, expect } from '@playwright/test'

const region = process.env.WEB_REGION ?? 'CN'
const action = region === 'CN' ? 'https://www.bing.com/search' : 'https://www.google.com/search'
const defaultLocale = region === 'CN' ? 'zh' : 'en'
const placeholders = { zh: '搜索网络', en: 'Search the web' }

test.use({ viewport: { width: 1440, height: 900 } })

for (const theme of ['light', 'dark'] as const) {
  test(`start: ${theme} interaction states keep a single focus frame`, async ({ page }, testInfo) => {
    await page.context().addCookies([{ name: 'gf-theme', value: theme, url: 'http://127.0.0.1:17320' }])
    await page.goto('/start')
    await page.waitForLoadState('networkidle')
    const form = page.getByRole('search')
    const input = page.getByRole('searchbox')
    const bounds = await form.boundingBox()
    const background = await form.evaluate((el) => getComputedStyle(el).backgroundColor)
    const border = await form.evaluate((el) => getComputedStyle(el).borderTopColor)
    const plainFrame = async () => {
      await expect(form).toHaveCSS('outline-style', 'none')
      await expect(form).toHaveCSS('border-top-width', '1px')
      await expect(form).toHaveCSS('box-shadow', 'none')
      await expect(input).toHaveCSS('outline-style', 'none')
      await expect(input).toHaveCSS('box-shadow', 'none')
      expect(await form.boundingBox()).toEqual(bounds)
    }
    await plainFrame()
    await form.screenshot({ path: testInfo.outputPath(`${theme}-default.png`) })
    await input.hover()
    await plainFrame()
    expect(await form.evaluate((el) => getComputedStyle(el).backgroundColor)).not.toBe(background)
    await expect(form).toHaveCSS('border-top-color', border)
    await form.screenshot({ path: testInfo.outputPath(`${theme}-hover.png`) })
    await page.mouse.down()
    await plainFrame()
    expect(await form.evaluate((el) => getComputedStyle(el).borderTopColor)).not.toBe(border)
    await page.mouse.up()
    await expect(input).toBeFocused()
    expect(await form.evaluate((el) => el.matches(':focus-within'))).toBe(true)
    await plainFrame()
    await expect(form).toHaveCSS('border-top-color', border)
    await form.screenshot({ path: testInfo.outputPath(`${theme}-pointer.png`) })
    await page.getByRole('banner').click()
    await page.keyboard.press('Tab')
    await expect(input).toBeFocused()
    await expect(form).toHaveCSS('outline-style', 'solid')
    await expect(form).toHaveCSS('outline-width', '2px')
    await expect(form).toHaveCSS('outline-offset', '-2px')
    await expect(form).toHaveCSS('border-top-color', 'rgba(0, 0, 0, 0)')
    await expect(input).toHaveCSS('outline-style', 'none')
    expect(await form.evaluate((el) => getComputedStyle(el).outlineColor)).not.toBe('rgb(0, 0, 0)')
    expect(await form.boundingBox()).toEqual(bounds)
    await form.screenshot({ path: testInfo.outputPath(`${theme}-keyboard.png`) })
    // Hover must not override keyboard focus; clicking an already focused input
    // must still switch back to the pointer presentation.
    await input.hover()
    await expect(form).toHaveCSS('outline-style', 'solid')
    await expect(form).toHaveCSS('border-top-color', 'rgba(0, 0, 0, 0)')
    await input.click()
    await plainFrame()
    await input.evaluate((el) => { (el as HTMLInputElement).disabled = true })
    await expect(input).toBeDisabled()
    await input.hover()
    await plainFrame()
    await expect(form).toHaveCSS('opacity', '0.5')
    await expect(form).toHaveCSS('background-color', background)
    await expect(input).toHaveCSS('cursor', 'not-allowed')
    await page.keyboard.press('Tab')
    await expect(input).not.toBeFocused()
  })
}

test('start: default SSR has only the brand and a working search form', async ({ browser }) => {
  const context = await browser.newContext({ javaScriptEnabled: false })
  const page = await context.newPage()
  const response = await page.goto('/start')
  expect(response!.status()).toBe(200)
  expect(response!.headers()['cache-control']).toContain('no-store')
  await expect(page).toHaveTitle('GoFurry · Start')
  await expect(page.locator('html')).toHaveAttribute('lang', defaultLocale)
  await expect(page.locator('html')).toHaveAttribute('data-region', region)
  await expect(page.getByRole('banner')).toHaveText('GoFurry')
  await expect(page.getByRole('search')).toHaveAttribute('action', action)
  await expect(page.getByRole('searchbox')).toHaveAttribute('placeholder', placeholders[defaultLocale])
  await expect(page.getByRole('searchbox')).toHaveCount(1)
  await expect(page.locator('button, select, a')).toHaveCount(0)
  expect(await page.locator('header img').evaluate((img) => (img as HTMLImageElement).naturalWidth)).toBeGreaterThan(0)
  await expect(page.getByRole('search')).toHaveCSS('max-width', '640px')
  await expect(page.getByRole('search').locator('svg')).toHaveCount(1)
  await context.close()
})

test('start: both themes and locales render consistently through SSR and hydration', async ({ page }, testInfo) => {
  const errors: string[] = []
  const remote: string[] = []
  page.on('pageerror', (error) => errors.push(error.message))
  page.on('console', (entry) => { if (entry.type() === 'error') errors.push(entry.text()) })
  page.on('request', (request) => { if (new URL(request.url()).origin !== 'http://127.0.0.1:17320' && (new URL(request.url()).protocol !== 'https:' || request.resourceType() !== 'image')) remote.push(request.url()) })
  for (const theme of ['light', 'dark'] as const) {
    for (const locale of ['zh', 'en'] as const) {
      await page.context().addCookies([
        { name: 'gf-theme', value: theme, url: 'http://127.0.0.1:17320' },
        { name: 'gf-locale', value: locale, url: 'http://127.0.0.1:17320' },
      ])
      const response = await page.goto('/start')
      const html = await response!.text()
      expect(html).toContain(`data-theme="${theme}"`)
      expect(html).toContain(`lang="${locale}"`)
      expect(html).toContain(`placeholder="${placeholders[locale]}"`)
      await page.waitForLoadState('networkidle')
      await expect(page.getByRole('searchbox')).toHaveAccessibleName(placeholders[locale])
      await expect(page.getByRole('search')).toHaveAttribute('action', action)
      await expect(page.locator('html')).toHaveAttribute('data-region', region)
      await expect(page.locator('html')).toHaveCSS('background-color', theme === 'light' ? 'rgb(242, 227, 208)' : 'rgb(29, 31, 34)')
      await page.screenshot({ path: testInfo.outputPath(`start-${region}-${theme}-${locale}-desktop.png`) })
    }
  }
  expect(errors).toEqual([])
  expect(remote).toEqual([])
})

test('start: responsive centering, keyboard focus, validation and encoded Enter submission', async ({ page }, testInfo) => {
  await page.goto('/start')
  const form = page.getByRole('search')
  const input = page.getByRole('searchbox')
  for (const viewport of [{ width: 1440, height: 900 }, { width: 390, height: 844 }, { width: 320, height: 568 }, { width: 844, height: 390 }]) {
    await page.setViewportSize(viewport)
    const bounds = (await form.boundingBox())!
    expect(Math.abs(bounds.x + bounds.width / 2 - viewport.width / 2)).toBeLessThanOrEqual(1)
    expect(Math.abs(bounds.y + bounds.height / 2 - viewport.height / 2)).toBeLessThanOrEqual(1)
    expect(bounds.x).toBeGreaterThanOrEqual(20)
    expect(bounds.width).toBeLessThanOrEqual(640)
    expect(await page.evaluate(() => document.documentElement.scrollWidth)).toBe(viewport.width)
    if (viewport.width === 390) {
      await page.evaluate(() => document.fonts.ready)
      await page.screenshot({ path: testInfo.outputPath(`start-${region}-mobile.png`) })
    }
  }
  await page.keyboard.press('Tab')
  await expect(input).toBeFocused()
  expect(await form.evaluate((el) => getComputedStyle(el).outlineColor)).not.toBe('rgba(0, 0, 0, 0)')
  await input.press('Enter')
  await expect(page).toHaveURL('/start')
  await input.fill('   ')
  await input.press('Enter')
  await expect(page).toHaveURL('/start')
  const query = 'GoFurry 狐狸 & 雪+?/#'
  await page.route('https://**/*', (route) => route.fulfill({ contentType: 'text/html', body: '<!doctype html><title>Intercepted search</title>' }))
  await input.fill(query)
  await input.press('Enter')
  await page.waitForURL((url) => url.origin === new URL(action).origin)
  expect(new URL(page.url()).pathname).toBe('/search')
  expect(new URL(page.url()).searchParams.get('q')).toBe(query)
})

test('start: Enter submission works without JavaScript and locale never changes the engine', async ({ browser }) => {
  const context = await browser.newContext({ javaScriptEnabled: false })
  await context.addCookies([{ name: 'gf-locale', value: defaultLocale === 'zh' ? 'en' : 'zh', url: 'http://127.0.0.1:17320' }])
  const page = await context.newPage()
  await page.goto('/start')
  await expect(page.getByRole('search')).toHaveAttribute('action', action)
  await page.keyboard.press('Tab')
  await expect(page.getByRole('searchbox')).toBeFocused()
  await expect(page.getByRole('search')).toHaveCSS('outline-style', 'solid')
  await page.route('https://**/*', (route) => route.fulfill({ contentType: 'text/html', body: '<!doctype html><title>Intercepted search</title>' }))
  await page.getByRole('searchbox').fill('GoFurry')
  await page.getByRole('searchbox').press('Enter')
  await page.waitForURL((url) => url.origin === new URL(action).origin)
  expect(new URL(page.url()).searchParams.get('q')).toBe('GoFurry')
  await context.close()
})
