import { test, expect, type Page } from '@playwright/test'
import { sampleMotion } from './motion'

const region = process.env.WEB_REGION ?? 'CN'
const origin = 'http://127.0.0.1:17320'
const englishPath = region === 'CN' ? '/en/start' : '/start'
const chinesePath = region === 'CN' ? '/start' : '/zh/start'
const input = (page: Page) => page.locator('input[name="q"]')
async function english(page: Page) { await page.goto(englishPath); await page.waitForLoadState('networkidle') }
async function settings(page: Page) { await page.locator('[data-start-settings]').click(); await expect(page.locator('[data-start-preferences]')).toHaveCSS('opacity', '1') }

test.use({ viewport: { width: 1440, height: 900 } })

test('start material: default is lazy; visible surfaces use SVG refraction and release it', async ({ page, context, baseURL }) => {
  const cookie = `gf-start-material-${region}`
  const other = region === 'CN' ? 'INTL' : 'CN'
  await context.addCookies([
    { name: cookie, value: 'unsupported', url: baseURL! },
    { name: `gf-start-material-${other}`, value: 'liquid', url: baseURL! },
    { name: `gf-quick-access-${region}`, value: '1', url: baseURL! },
  ])
  await page.addInitScript(region => localStorage.setItem(`gf-quick-access:${region}`, JSON.stringify({
    version: 1, pinned: [{ id: 'example', name: 'Example', url: 'https://example.com/', order: 0 }], recent: [],
  })), region)
  await context.route(/https:\/\/(a\.favicon\.im\/example\.com|example\.com\/favicon\.ico)/, route => route.fulfill({ status: 404 }))
  const liquidRequests: string[] = []
  page.on('request', request => { if (/\/assets\/start-liquid-[^/]+\.(css|js)$/.test(request.url())) liquidRequests.push(request.url()) })
  const errors: string[] = []
  page.on('pageerror', error => errors.push(error.message))
  const start = page.locator('[data-start-material]')
  const surfaces = page.locator('[data-start-liquid]')
  const filters = page.locator('filter[id^="liquid-glass-"]')
  const ssr = await page.goto(englishPath)
  expect(await ssr?.text()).toContain('data-start-material="frosted"')
  await expect(page.locator('[data-quick-site]')).toHaveCount(1)
  await expect(start).toHaveAttribute('data-start-material', 'frosted')
  await expect(filters).toHaveCount(0)
  expect(liquidRequests).toEqual([])
  const categoryTint = await page.locator('[data-categories]').getByRole('button', { name: 'Search', exact: true })
    .evaluate(node => getComputedStyle(node).backgroundColor)

  await settings(page)
  await page.getByRole('tab', { name: 'Advanced', exact: true }).click()
  await page.getByRole('radio', { name: 'Liquid Glass', exact: true }).click()
  await expect(surfaces).toHaveCount(12)
  const active = await surfaces.evaluateAll(nodes => nodes.map(node => {
    const css = getComputedStyle(node).backdropFilter
    const filter = document.getElementById(css.match(/liquid-glass-\d+/)?.[0] ?? '')
    return {
      surface: node.getAttribute('data-start-liquid'), material: node.getAttribute('data-lg-material'),
      refracts: Number(filter?.querySelector('feDisplacementMap')?.getAttribute('scale')) > 0,
    }
  }))
  expect(active.sort((a, b) => a.surface!.localeCompare(b.surface!))).toEqual(
    ['category', 'category', 'category', 'category', 'category', 'quick-icon', 'quick-icon',
      'search', 'tool', 'tool', 'tool', 'widget-manager'].map(surface => ({ surface, material: 'clear', refracts: true })),
  )
  await expect(page.locator('[data-start-liquid="category"][aria-pressed="true"]')).toHaveCSS('background-color', categoryTint)
  expect(liquidRequests.some(url => url.endsWith('.js'))).toBe(true)
  expect((await context.cookies()).find(item => item.name === cookie)?.value).toBe('liquid')
  await page.keyboard.press('Escape')
  await expect(page.getByRole('dialog')).toHaveCount(0)

  await input(page).fill('狐狸 art')
  await expect(input(page)).toHaveValue('狐狸 art')
  const calculator = page.getByRole('button', { name: 'Calculator', exact: true })
  await calculator.click()
  const panel = page.getByRole('dialog', { name: 'Calculator', exact: true })
  await expect(panel).toBeVisible()
  await expect(panel.locator('[data-start-liquid]')).toHaveCount(0)
  await page.keyboard.press('Escape')
  await expect(calculator).toBeFocused()

  const reloaded = await page.reload()
  expect(await reloaded?.text()).toContain('data-start-material="liquid"')
  await expect(surfaces).toHaveCount(12)
  await settings(page)
  await page.getByRole('tab', { name: 'Advanced', exact: true }).click()
  await page.getByRole('radio', { name: 'Frosted Glass', exact: true }).click()
  await expect(start).toHaveAttribute('data-start-material', 'frosted')
  await expect(surfaces).toHaveCount(0)
  await expect(filters).toHaveCount(0)
  await expect(page.locator('[style*="--hyalite:"]')).toHaveCount(0)
  expect(errors).toEqual([])
})

test('start material: responsive shortcuts and keyed rail changes only bind visible surfaces', async ({ page, context, baseURL }) => {
  await context.addCookies([
    { name: `gf-start-material-${region}`, value: 'liquid', url: baseURL! },
    { name: `gf-quick-access-${region}`, value: '1', url: baseURL! },
  ])
  await page.addInitScript(region => {
    localStorage.setItem(`gf-quick-access:${region}`, JSON.stringify({
      version: 1,
      pinned: Array.from({ length: 9 }, (_, order) => ({ id: `pin-${order}`, name: `Pin ${order}`, url: `https://pin-${order}.example/`, order })),
      recent: Array.from({ length: 3 }, (_, index) => ({ name: `Recent ${index}`, url: `https://recent-${index}.example/`, lastVisited: Date.now() - index, count: 1 })),
    }))
    localStorage.setItem(`gf-widget-rails:${region}`, JSON.stringify({
      version: 1, leftEnabled: true, rightEnabled: true, widgets: ['ai-hub', 'asset-hub', 'game-data-hub'],
    }))
  }, region)
  await context.route(/favicon\.im|\.example\/favicon\.ico/, route => route.fulfill({ status: 404 }))
  await english(page)
  const surfaces = page.locator('[data-start-liquid]')
  const quick = page.locator('[data-start-liquid="quick-icon"]')
  const hiddenQuick = page.locator('[data-quick-site]:not(:visible) [style*="--hyalite:"]')
  await expect(surfaces).toHaveCount(24)
  await expect(quick).toHaveCount(11)
  await expect(hiddenQuick).toHaveCount(0)

  await page.setViewportSize({ width: 375, height: 667 })
  await expect(quick).toHaveCount(5)
  await expect(surfaces).toHaveCount(18)
  await expect(hiddenQuick).toHaveCount(0)
  await page.setViewportSize({ width: 1440, height: 900 })
  await expect(surfaces).toHaveCount(24)

  const recent = page.locator('[data-site-key="recent-0.example"] [data-start-liquid]')
  await expect(recent).toHaveCount(1)
  await page.locator('[data-quick-manage]').click()
  const quickManager = page.locator('[data-quick-manager]')
  await quickManager.getByLabel('Name', { exact: true }).fill('Added')
  await quickManager.getByLabel('URL', { exact: true }).fill('https://added.example/')
  await quickManager.getByRole('button', { name: 'Add site', exact: true }).click()
  const addedSite = page.locator('[data-site-key="added.example"]')
  await expect(addedSite.locator('[data-start-liquid]')).toHaveCount(1)
  await expect(recent).toHaveCount(0)
  await quickManager.getByRole('button', { name: 'Delete: Added', exact: true }).click()
  await expect(addedSite.locator('[style*="--hyalite:"]')).toHaveCount(0)
  await expect(recent).toHaveCount(1)
  await page.keyboard.press('Escape')
  await expect(quickManager).toHaveCount(0)

  const asset = page.locator('[data-rail-trigger="asset-hub"]')
  const original = await asset.elementHandle()
  await page.locator('[data-widget-manager-trigger]').click()
  const widgetManager = page.locator('[data-widget-manager]')
  await widgetManager.getByRole('tab', { name: 'Added', exact: true }).click()
  await widgetManager.getByRole('button', { name: 'Move up Asset Hub', exact: true }).click()
  await expect(page.locator('[data-start-widgets] [data-rail-trigger]').first()).toHaveAttribute('data-rail-trigger', 'asset-hub')
  expect(await asset.evaluate((element, previous) => element === previous, original)).toBe(true)
  await widgetManager.getByRole('button', { name: 'Remove Asset Hub', exact: true }).click()
  await expect(asset).toHaveCount(0)
  await expect(surfaces).toHaveCount(23)
  await widgetManager.getByRole('tab', { name: 'Library', exact: true }).click()
  await widgetManager.getByRole('button', { name: 'Add Asset Hub', exact: true }).click()
  await expect(asset).toHaveAttribute('data-start-liquid', 'widget')
  await expect(widgetManager.locator('[data-start-liquid]')).toHaveCount(0)
  await page.keyboard.press('Escape')
  await expect(widgetManager).toHaveCount(0)

  await settings(page)
  await page.getByRole('tab', { name: 'Advanced', exact: true }).click()
  const officialTools = page.getByRole('switch', { name: 'Show official tools' })
  const widgetRail = page.getByRole('switch', { name: 'Show widget rail' })
  await officialTools.click()
  await expect(surfaces).toHaveCount(21)
  await widgetRail.click()
  await expect(surfaces).toHaveCount(17)
  await officialTools.click()
  await widgetRail.click()
  await expect(surfaces).toHaveCount(24)
  await page.getByRole('tab', { name: 'Basic', exact: true }).click()
  const furrySearch = page.getByRole('switch', { name: 'Show furry search options' })
  await furrySearch.click()
  await expect(page.locator('[data-start-liquid="category"]')).toHaveCount(4)
  await expect(surfaces).toHaveCount(23)
  await furrySearch.click()
  await expect(surfaces).toHaveCount(24)
})

test('start material: the original blur remains when SVG backdrop is unavailable', async ({ page, context, baseURL }) => {
  await context.addCookies([{ name: `gf-start-material-${region}`, value: 'liquid', url: baseURL! }])
  await page.route(/\/assets\/start-liquid-[^/]+\.js$/, async route => {
    await page.evaluate(() => document.documentElement.setAttribute('data-hyalite', 'off'))
    await route.continue()
  })
  await english(page)
  await expect(page.locator('[data-start-material]')).toHaveAttribute('data-start-material', 'liquid')
  await expect(page.locator('[data-start-liquid], filter[id^="liquid-glass-"]')).toHaveCount(0)
  await expect(page.locator('form').filter({ has: input(page) })).toHaveCSS('backdrop-filter', /^blur\(/)
})

test('start material: Transparent paints on SSR, keeps its scope and releases Liquid', async ({ page, context, baseURL }) => {
  const cookie = `gf-start-material-${region}`
  const other = region === 'CN' ? 'INTL' : 'CN'
  await context.addCookies([
    { name: cookie, value: 'transparent', url: baseURL! },
    { name: `gf-start-material-${other}`, value: 'liquid', url: baseURL! },
    { name: `gf-quick-access-${region}`, value: '1', url: baseURL! },
  ])
  const requests: string[] = []
  const errors: string[] = []
  page.on('request', request => { if (/\/assets\/start-liquid-[^/]+\.(css|js)$/.test(request.url())) requests.push(request.url()) })
  page.on('pageerror', error => errors.push(error.message))
  const transparent = page.locator('[data-start-transparent]:visible')
  const residual = page.locator('[data-start-liquid], filter[id^="liquid-glass-"], [style*="--hyalite:"], [style*="--hyalite-edge:"]')
  const search = page.getByRole('search')
  const ssr = await page.goto(englishPath)
  expect(await ssr?.text()).toContain('data-start-material="transparent"')
  await expect(transparent).toHaveCount(11)
  const applied = await transparent.evaluateAll(nodes => nodes.map(node => {
    const css = getComputedStyle(node)
    const paint = document.createElement('canvas').getContext('2d')!
    paint.fillStyle = css.backgroundColor
    paint.fillRect(0, 0, 1, 1)
    return { target: node.getAttribute('data-start-transparent'), filter: css.backdropFilter, alpha: paint.getImageData(0, 0, 1, 1).data[3] }
  }))
  expect(applied.map(item => item.target).sort()).toEqual([
    'category', 'category', 'category', 'category', 'category', 'quick-icon', 'search', 'tool', 'tool', 'tool', 'widget-manager',
  ])
  expect(applied.filter(item => item.filter !== 'none' || item.alpha === 0 || item.alpha === 255)).toEqual([])
  await expect(residual).toHaveCount(0)
  expect(requests).toEqual([])
  const box = await search.boundingBox()
  await input(page).fill('狐狸 art')
  await settings(page)
  await page.getByRole('tab', { name: 'Advanced', exact: true }).click()
  const choice = page.getByRole('radio', { name: 'Transparent Glass', exact: true })
  await expect(choice).toBeChecked()
  await choice.focus()
  await page.keyboard.press('ArrowLeft')
  await expect(page.getByRole('radio', { name: 'Liquid Glass', exact: true })).toBeChecked()
  await expect(page.locator('[data-start-liquid]')).toHaveCount(11)
  await page.keyboard.press('ArrowRight')
  await expect(choice).toBeChecked()
  await expect(residual).toHaveCount(0)
  await expect(page.getByRole('dialog').locator('[data-start-transparent]')).toHaveCount(0)
  await page.keyboard.press('Escape')
  await expect(page.getByRole('dialog')).toHaveCount(0)
  await expect(page.locator('[data-start-settings]')).toBeFocused()
  expect(await search.boundingBox()).toEqual(box)
  await expect(input(page)).toHaveValue('狐狸 art')
  expect((await context.cookies()).find(item => item.name === cookie)?.value).toBe('transparent')
  const reloaded = await page.reload()
  expect(await reloaded?.text()).toContain('data-start-material="transparent"')
  await expect(residual).toHaveCount(0)
  expect(errors).toEqual([])
})

for (const { locale, path, width, colorScheme, message, query } of [
  { locale: 'zh', path: chinesePath, width: 1280, colorScheme: 'light', message: '请输入搜索内容', query: '狐狸 绘画' },
  { locale: 'en', path: englishPath, width: 375, colorScheme: 'dark', message: 'Enter a search term', query: 'fox art' },
] as const) {
  test(`start: empty search feedback in ${locale} at ${width}px`, async ({ page, context }) => {
    await page.setViewportSize({ width, height: 720 })
    await page.emulateMedia({ colorScheme, reducedMotion: width === 375 ? 'reduce' : 'no-preference' })
    await context.route('https://www.bing.com/search?**', route => route.fulfill({ body: 'Search destination' }))
    await page.route('**/api/v1/start/search/suggestions?*', route => route.fulfill({
      headers: { 'X-GoFurry-Region': region, 'X-GoFurry-Content-Mode': 'SFW' },
      json: { suggestions: [] },
    }))
    const errors: string[] = []
    page.on('pageerror', error => errors.push(error.message))
    page.on('console', message => {
      if (message.type() === 'error' && /hydrat|react error #(418|423)/i.test(message.text())) errors.push(message.text())
    })
    await page.goto(path, { waitUntil: 'networkidle' })
    await page.evaluate(() => document.fonts.ready.then(() => true))
    await expect(page.locator('html')).toHaveAttribute('data-theme', colorScheme)

    const form = page.getByRole('search'), field = input(page)
    const status = page.locator('[data-start-search]').getByRole('status')
    const button = form.locator('button[type="submit"]')
    const categories = page.locator('[data-categories]')
    const platforms = width === 375 ? page.locator('[data-mobile-platform-trigger]') : page.locator('[data-desktop-platforms]')
    const initialForm = await form.boundingBox()
    const initialPlatforms = await platforms.boundingBox()
    await field.evaluate(element => {
      element.addEventListener('invalid', event => {
        // Check the final event state after React has handled it.
        setTimeout(() => element.dataset.nativeValidationCanceled = String(event.defaultPrevented), 0)
      })
    })
    const expectHint = async () => {
      await expect(status).toHaveText(message)
      await expect(field).toBeFocused()
      await expect(field).toHaveAttribute('aria-invalid', 'true')
      await expect(field).toHaveAccessibleDescription(message)
      await expect(field).toHaveAttribute('data-native-validation-canceled', 'true')
      await field.evaluate(element => { delete element.dataset.nativeValidationCanceled })
    }

    await expect(status).toBeEmpty()
    await field.press('Enter')
    await expectHint()
    const hintBox = (await status.boundingBox())!
    const categoryBox = (await categories.boundingBox())!
    expect(hintBox.y).toBeGreaterThanOrEqual(categoryBox.y + categoryBox.height)
    expect(hintBox.y + hintBox.height).toBeLessThanOrEqual(initialPlatforms!.y)
    expect(await form.boundingBox()).toEqual(initialForm)
    expect(await platforms.boundingBox()).toEqual(initialPlatforms)
    await field.press('Escape')
    await expect(status).toBeEmpty()
    await expect(field).not.toHaveAttribute('aria-invalid')

    await button.click()
    await expectHint()
    await categories.getByRole('button').first().focus()
    await expect(status).toBeEmpty()
    await field.fill(' \u3000 ')
    await field.press('Enter')
    await expectHint()
    expect(context.pages()).toHaveLength(1)

    await field.fill(`  ${query}  `)
    await expect(status).toBeEmpty()
    const [destination] = await Promise.all([context.waitForEvent('page'), field.press('Enter')])
    await expect(destination).toHaveURL(/^https:\/\/www\.bing\.com\/search\?/)
    expect(new URL(destination.url()).searchParams.get('q')).toBe(query)
    expect(errors).toEqual([])
  })
}

test('start: search without JavaScript rejects blanks and submits the native GET form', async ({ browser, baseURL }) => {
  const context = await browser.newContext({ javaScriptEnabled: false, baseURL })
  const requests: string[] = []
  try {
    await context.route('https://www.bing.com/search?**', route => {
      requests.push(route.request().method())
      return route.fulfill({ body: 'Search destination' })
    })
    const page = await context.newPage()
    await page.goto(englishPath)
    const form = page.getByRole('search'), field = input(page)
    await expect(form).not.toHaveAttribute('novalidate')
    await expect(field).toHaveAttribute('required')
    await field.press('Enter')
    await expect(field).toBeFocused()
    expect(await field.evaluate((element: HTMLInputElement) => element.validity.valueMissing)).toBe(true)
    await field.fill(' \u3000 ')
    await form.locator('button[type="submit"]').click()
    expect(await field.evaluate((element: HTMLInputElement) => element.validity.patternMismatch)).toBe(true)
    expect(context.pages()).toHaveLength(1)
    expect(requests).toEqual([])
    await expect(page.locator('[data-start-search]').getByRole('status')).toBeEmpty()

    await field.fill('fox art')
    const [destination] = await Promise.all([context.waitForEvent('page'), field.press('Enter')])
    await expect(destination).toHaveURL(/^https:\/\/www\.bing\.com\/search\?/)
    expect(new URL(destination.url()).searchParams.get('q')).toBe('fox art')
    expect(requests).toEqual(['GET'])
  } finally {
    await context.close()
  }
})

test('start: explicit language URLs own SSR metadata and ignore conflicting cookies', async ({ browser }) => {
  for (const [path, locale] of [[englishPath, 'en'], [chinesePath, 'zh']]) {
    const context = await browser.newContext({ javaScriptEnabled: false, locale: locale === 'en' ? 'zh-CN' : 'en-US' })
    await context.addCookies([{ name: 'gf-locale', value: locale === 'en' ? 'zh' : 'en', url: origin }, { name: 'gf-region', value: region === 'CN' ? 'INTL' : 'CN', url: origin }])
    const page = await context.newPage(), response = await page.goto(path)
    expect(response!.status()).toBe(200)
    expect(new URL(page.url()).pathname).toBe(path)
    await expect(page.locator('html')).toHaveAttribute('lang', locale)
    await expect(page.locator('html')).toHaveAttribute('data-region', region)
    await expect(page.locator('html')).toHaveAttribute('data-content-mode', 'SFW')
    expect(response!.headers()['x-robots-tag']).toBe('noindex')
    await expect(page.locator('meta[name="robots"]')).toHaveAttribute('content', 'noindex')
    await expect(page).toHaveTitle(locale === 'en' ? 'GoFurry · Start' : 'GoFurry · 起始页')
    await expect(page.locator('[data-start-actions] > *')).toHaveCount(4)
    await expect(page.locator('[data-start-logo]')).toHaveCSS('background-color', 'rgb(215, 220, 226)')
    await expect(page.locator('[data-start-filing]')).toHaveCount(region === 'CN' ? 1 : 0)
    await expect(page.getByRole('search')).toHaveAttribute('action', 'https://www.bing.com/search')
    await context.close()
  }
})

test('start: system theme is painted by SSR CSS before JS and stays synchronized', async ({ browser, page }) => {
  for (const colorScheme of ['light', 'dark'] as const) {
    const context = await browser.newContext({ javaScriptEnabled: false, colorScheme })
    const ssr = await context.newPage(); await ssr.goto('/start')
    await expect(ssr.locator('html')).toHaveAttribute('data-theme-preference', 'system')
    await expect(ssr.locator('html')).toHaveCSS('background-color', colorScheme === 'dark' ? 'rgb(29, 31, 34)' : 'rgb(242, 227, 208)')
    await expect(ssr.getByRole('search')).toHaveCSS('background-color', colorScheme === 'dark' ? 'rgba(25, 30, 38, 0.36)' : 'rgba(250, 251, 253, 0.4)')
    await context.close()
  }
  // This suite starts the production Web without a Go API. Provide the neutral
  // weather response and use the real local no-wallpaper preference.
  await page.route('**/api/v1/start/weather?*', route => route.fulfill({
    headers: { 'X-GoFurry-Region': region, 'X-GoFurry-Content-Mode': 'SFW' },
    json: { state: 'unavailable', location: null, weather: null },
  }))
  await page.addInitScript(region => {
    localStorage.setItem(`gf-wallpaper:${region}`, JSON.stringify({ version: 1, source: 'official', mode: 'off' }))
  }, region)
  const errors: string[] = []
  page.on('pageerror', e => errors.push(e.message)); page.on('console', e => { if (e.type() === 'error') errors.push(e.text()) })
  await page.emulateMedia({ colorScheme: 'dark' }); await english(page)
  await expect(page.locator('html')).toHaveAttribute('data-theme', 'dark')
  await page.emulateMedia({ colorScheme: 'light' }); await expect(page.locator('html')).toHaveAttribute('data-theme', 'light')
  await settings(page); await page.getByRole('radio', { name: 'Dark', exact: true }).click()
  await expect(page.locator('html')).not.toHaveAttribute('data-gf-theme-reveal')
  await page.emulateMedia({ colorScheme: 'light' }); await expect(page.locator('html')).toHaveAttribute('data-theme', 'dark')
  await page.reload(); await expect(page.locator('html')).toHaveAttribute('data-theme-preference', 'dark')
  await settings(page); await page.getByRole('radio', { name: 'System', exact: true }).click()
  await expect(page.locator('html')).toHaveAttribute('data-theme-preference', 'system')
  await expect(page.locator('html')).not.toHaveAttribute('data-gf-theme-reveal')
  await page.emulateMedia({ colorScheme: 'dark' }); await expect(page.locator('html')).toHaveAttribute('data-theme', 'dark')
  expect(errors).toEqual([])
})

test('start: settings trap focus, retain their shell across language URLs and fully animate out', async ({ page }) => {
  await english(page); const shell = await page.locator('[data-start-page]').elementHandle()
  await settings(page)
  const dialog = page.locator('[data-start-preferences]')
  await expect(dialog).toHaveCSS('border-top-width', '0px'); await expect(dialog).toHaveCSS('box-shadow', 'none')
  await expect(dialog).toHaveCSS('transition-duration', '0s') // Motion owns opacity/transform.
  await expect(dialog.getByRole('button', { name: 'Close settings' })).toBeFocused()
  for (let i = 0; i < 12; i++) {
    await page.keyboard.press('Tab')
    // Base UI's boundary guards return focus to the dialog on the next frame.
    const inside = await dialog.evaluate(async el => {
      await new Promise<void>(resolve => requestAnimationFrame(() => resolve()))
      return el.contains(document.activeElement)
    })
    expect(inside).toBe(true)
  }
  await expect(dialog.getByRole('link', { name: /GitHub/ })).toHaveAttribute('href', 'https://github.com/gofurry')
  await expect(dialog.getByRole('radio', { name: 'NSFW', exact: true })).toHaveCount(region === 'CN' ? 0 : 1)
  if (region === 'INTL') {
    await dialog.getByRole('radio', { name: 'NSFW', exact: true }).click()
    expect((await page.context().cookies()).find(c => c.name === 'gf-content-preference')?.value).toBe('NSFW')
    await expect(dialog).not.toContainText('Controls adult search links')
    await expect(page.locator('html')).toHaveAttribute('data-content-mode', 'SFW')
  }
  await dialog.getByRole('radio', { name: '简体中文', exact: true }).click()
  await expect(page).toHaveURL(chinesePath)
  await expect(page.locator('html')).toHaveAttribute('lang', 'zh')
  await expect(dialog).toBeVisible()
  if (region === 'INTL') await expect(dialog).not.toContainText('控制成人搜索入口的显示')
  expect(await page.locator('[data-start-page]').evaluate((el, first) => el === first, shell)).toBe(true)
  await dialog.getByRole('radio', { name: 'English', exact: true }).click(); await expect(page).toHaveURL(englishPath)
  await page.keyboard.press('Escape'); await expect(dialog).toHaveAttribute('data-ending-style')
  const exit = await sampleMotion(dialog)
  expect(exit.some(frame => frame.opacity > 0 && frame.opacity < 1)).toBe(true)
  expect(exit.at(-1)!.t).toBeGreaterThan(250); await expect(dialog).toHaveCount(0)
  await expect(page.locator('[data-start-settings]')).toBeFocused()
  await settings(page); await page.mouse.click(4, 4); await expect(dialog).toHaveCount(0)
  await page.reload(); await settings(page)
  if (region === 'INTL') await expect(dialog.getByRole('radio', { name: 'NSFW', exact: true })).toBeChecked()
  else await expect(dialog.getByRole('radio', { name: 'NSFW', exact: true })).toHaveCount(0)
  await page.emulateMedia({ reducedMotion: 'reduce' }); await expect(dialog).toHaveCSS('transition-duration', '0s')
  await dialog.getByRole('button', { name: 'Close settings' }).click(); await expect(dialog).toHaveCount(0)
})

test('start: appearance retains the native circular reveal centered on the selected option', async ({ page }) => {
  await page.addInitScript(() => {
    (window as unknown as { reveal: unknown[] }).reveal = []
    const animate = Element.prototype.animate
    Element.prototype.animate = function(frames, options) { if (typeof options === 'object' && options.pseudoElement) (window as unknown as { reveal: unknown[] }).reveal.push({ frames, options }); return animate.call(this, frames, options) }
  })
  await page.emulateMedia({ colorScheme: 'light' }); await english(page); await settings(page)
  const dark = page.getByRole('radio', { name: 'Dark', exact: true }), bounds = (await dark.boundingBox())!
  await dark.click(); await expect(page.locator('html')).toHaveAttribute('data-theme', 'dark'); await expect(page.locator('html')).not.toHaveAttribute('data-gf-theme-reveal')
  const reveal = await page.evaluate(() => (window as unknown as { reveal: { frames: { clipPath: string[] }; options: { duration: number; easing: string } }[] }).reveal)
  expect(reveal).toHaveLength(1)
  expect(reveal[0].options).toMatchObject({ duration: 420, easing: 'cubic-bezier(0.22, 1, 0.36, 1)' })
  expect(reveal[0].frames.clipPath[0]).toBe(`circle(0px at ${bounds.x + bounds.width / 2}px ${bounds.y + bounds.height / 2}px)`)
  await page.emulateMedia({ reducedMotion: 'reduce' }); await page.getByRole('radio', { name: 'Light', exact: true }).click()
  await expect(page.locator('html')).toHaveAttribute('data-theme', 'light')
  expect(await page.evaluate(() => (window as unknown as { reveal: unknown[] }).reveal.length)).toBe(1)
})

test('start: settings text tabs move their underline and resize around the viewport center', async ({ page }) => {
  await english(page); await settings(page)
  const dialog = page.locator('[data-start-preferences]'), initial = (await dialog.boundingBox())!
  const basic = dialog.getByRole('tab', { name: 'Basic', exact: true }), advanced = dialog.getByRole('tab', { name: 'Advanced', exact: true })
  const underlineStart = (await dialog.locator('[data-settings-tab-indicator]').boundingBox())!.x
  await basic.focus()
  const resizing = sampleMotion(dialog)
  await basic.press('ArrowRight'); await expect(advanced).toBeFocused(); await expect(advanced).toHaveAttribute('aria-selected', 'true')
  await expect(advanced).toHaveCSS('background-color', 'rgba(0, 0, 0, 0)'); await expect(advanced).toHaveCSS('outline-style', 'solid')
  const underline = dialog.locator('[data-settings-tab-indicator]')
  const progress = await sampleMotion(underline)
  expect(progress.some(frame => frame.x > underlineStart + 0.1 && frame.x < progress.at(-1)!.x - 0.1)).toBe(true)
  await expect(dialog.getByRole('tabpanel').getByRole('switch', { name: 'Show Set as homepage shortcut' })).toBeChecked()
  const advancedBounds = (await dialog.boundingBox())!
  const activePanel = dialog.getByRole('tabpanel')
  const advancedHeight = (await activePanel.boundingBox())!.height
  expect(advancedBounds.height).toBeLessThan(initial.height)
  const frames = await resizing
  expect(frames.filter(frame => frame.h < initial.height - 1 && frame.h > advancedBounds.height + 1).length).toBeGreaterThan(1)
  expect(frames.every(frame => Math.abs(frame.y + frame.h / 2 - (initial.y + initial.height / 2)) < 1)).toBe(true)
  const hiddenPanel = dialog.locator('[role=tabpanel][inert]')
  await expect(hiddenPanel).toHaveCSS('display', 'none')
  expect(await hiddenPanel.evaluate(panel => {
    const focused = document.activeElement
    panel.querySelector('button')?.focus()
    return document.activeElement === focused
  })).toBe(true)

  await advanced.press('ArrowRight')
  await expect(dialog.getByRole('tab', { name: 'Feedback' })).toBeFocused()
  await expect(activePanel).toContainText('1074661551')
  const feedbackFrames = await sampleMotion(dialog)
  const feedbackHeight = (await activePanel.boundingBox())!.height
  expect(feedbackFrames.at(-1)!.h).toBeCloseTo(advancedBounds.height - advancedHeight + feedbackHeight, 0)
  await basic.click(); await expect(basic).toHaveAttribute('aria-selected', 'true')
  const language = dialog.getByRole('radiogroup', { name: 'Language', exact: true }), englishRadio = language.getByRole('radio', { name: 'English', exact: true })
  await expect(englishRadio).toBeChecked(); await englishRadio.hover()
  await expect(englishRadio.locator('[aria-hidden="true"] > span')).toHaveCSS('opacity', '1')
  await expect(englishRadio).toHaveCSS('background-color', 'rgba(0, 0, 0, 0)'); await expect(englishRadio).toHaveCSS('border-top-width', '0px')
  await language.getByRole('radio', { name: '简体中文', exact: true }).click(); await expect(page).toHaveURL(chinesePath)
  await expect(dialog.getByRole('radio', { name: '简体中文', exact: true }).locator('[aria-hidden="true"] > span')).toHaveCSS('opacity', '1')
  const footer = dialog.locator('footer')
  await expect(footer).toHaveText('用心打造，开源于 GitHub。'); await expect(footer).toHaveCSS('border-top-style', 'dashed'); await expect(footer).toHaveCSS('border-top-width', '2px')
  for (const width of [320, 375, 390]) {
    await page.setViewportSize({ width, height: 667 })
    for (const radio of await dialog.getByRole('radio').all()) { const r = (await radio.boundingBox())!; expect(r.x).toBeGreaterThanOrEqual(16); expect(r.x + r.width).toBeLessThanOrEqual(width - 16) }
    expect(await page.evaluate(() => document.documentElement.scrollWidth)).toBe(width)
  }
  await dialog.getByRole('radio', { name: 'English', exact: true }).click(); await expect(footer).toHaveText('Made with care. Open source on GitHub.')
  await expect(footer.getByRole('link')).toHaveAttribute('href', 'https://github.com/gofurry'); await expect(footer.getByRole('link')).toHaveCSS('background-color', 'rgba(0, 0, 0, 0)')
  await page.emulateMedia({ reducedMotion: 'reduce' }); await expect(underline).toHaveCSS('transition-duration', '0s')
})

test('start: settings feedback follows deployment in both languages and copies the QQ number', async ({ page, context }) => {
  await context.grantPermissions(['clipboard-read', 'clipboard-write'])
  await context.route('https://t.me/GoFurryOfficial', route => route.fulfill({ body: 'Telegram destination' }))
  await page.emulateMedia({ colorScheme: 'light' })
  await page.goto(englishPath)
  await settings(page)
  const dialog = page.locator('[data-start-preferences]')
  await dialog.getByRole('tab', { name: 'Feedback' }).click()
  const panel = dialog.getByRole('tabpanel')
  const qq = panel.getByRole('button', { name: /Copy QQ Group number/ })
  await expect(qq).toContainText('QQ Group')
  await qq.press('Enter')
  await expect(panel.getByRole('status')).toHaveText('Copied')
  expect(await page.evaluate(() => navigator.clipboard.readText())).toBe('1074661551')
  await page.evaluate(() => {
    navigator.clipboard.writeText = async () => { throw new Error('Permission denied') }
  })
  await qq.click()
  await expect(panel.getByRole('status')).toHaveText('Copy failed')
  const telegram = panel.getByRole('link', { name: /Telegram Channel/ })
  if (region === 'INTL') {
    await expect(telegram).toHaveAttribute('href', 'https://t.me/GoFurryOfficial')
    await expect(telegram).toHaveAttribute('rel', 'noopener noreferrer')
    const [opened] = await Promise.all([page.waitForEvent('popup'), telegram.click()])
    await expect(opened).toHaveURL('https://t.me/GoFurryOfficial')
    await opened.close()
  } else await expect(telegram).toHaveCount(0)
  await expect(panel.getByRole('link', { name: /^Email/ })).toHaveAttribute('href', 'mailto:contact@librecosta.com')

  await dialog.getByRole('tab', { name: 'Basic' }).click()
  await dialog.getByRole('radio', { name: '简体中文', exact: true }).click()
  await expect(page).toHaveURL(chinesePath)
  await page.emulateMedia({ colorScheme: 'dark', reducedMotion: 'reduce' })
  await page.setViewportSize({ width: 375, height: 260 })
  await expect(page.locator('html')).toHaveAttribute('data-theme', 'dark')
  const basicBounds = (await dialog.boundingBox())!
  expect(basicBounds.y).toBeGreaterThanOrEqual(15)
  expect(basicBounds.y + basicBounds.height).toBeLessThanOrEqual(245)
  const footer = dialog.locator('footer')
  await footer.getByRole('link').focus()
  await expect.poll(() => dialog.evaluate(el => el.scrollTop)).toBeGreaterThan(0)
  await dialog.getByRole('tab', { name: '反馈' }).click()
  await expect(panel.getByRole('button', { name: /复制 QQ 交流群群号/ })).toContainText('QQ 交流群')
  await expect(panel.getByRole('link', { name: /Telegram 频道/ })).toHaveCount(region === 'INTL' ? 1 : 0)
  await expect(panel.getByRole('link', { name: /^官方邮箱/ })).toHaveAttribute('href', 'mailto:contact@librecosta.com')
  const mobileBounds = (await dialog.boundingBox())!
  expect(mobileBounds.y).toBeGreaterThanOrEqual(15)
  expect(mobileBounds.y + mobileBounds.height).toBeLessThanOrEqual(245)
  expect(await page.evaluate(() => document.documentElement.scrollWidth)).toBe(375)
  await page.keyboard.press('Escape')
  await expect(page.locator('[data-start-settings]')).toBeFocused()
})

test('start: settings segments slide, retain flat theme-aware surfaces and preserve preferences', async ({ page }) => {
  await page.emulateMedia({ colorScheme: 'light' }); await english(page); await settings(page)
  const dialog = page.locator('[data-start-preferences]'), track = dialog.getByRole('radiogroup', { name: 'Appearance', exact: true }), selection = dialog.locator('[data-settings-selection="appearance"]')
  await expect(track.getByRole('radio', { name: 'System', exact: true })).toBeChecked()
  const lightSurface = await track.evaluate(el => getComputedStyle(el).backgroundColor)
  await expect(track).toHaveCSS('border-radius', '10px'); await expect(selection).toHaveCSS('border-radius', '8px'); await expect(selection).toHaveCSS('box-shadow', 'none')
  const selectionStart = (await selection.boundingBox())!.x
  await track.getByRole('radio', { name: 'Dark', exact: true }).click()
  const progress = await sampleMotion(selection)
  expect(progress.some(frame => frame.x > selectionStart + 0.1 && frame.x < progress.at(-1)!.x - 0.1)).toBe(true)
  await expect(page.locator('html')).not.toHaveAttribute('data-gf-theme-reveal'); expect(await track.evaluate(el => getComputedStyle(el).backgroundColor)).not.toBe(lightSurface)
  if (region === 'INTL') {
    const content = dialog.getByRole('radiogroup', { name: 'Content preference', exact: true })
    await expect(content.getByRole('radio', { name: 'SFW', exact: true })).toBeChecked()
    await content.getByRole('radio', { name: 'NSFW', exact: true }).click()
    await expect(page.locator('html')).toHaveAttribute('data-content-mode', 'SFW')
  } else await expect(dialog.getByRole('radio', { name: 'NSFW', exact: true })).toHaveCount(0)
  await page.reload(); await settings(page); await expect(track.getByRole('radio', { name: 'Dark', exact: true })).toBeChecked()
  if (region === 'INTL') await expect(dialog.getByRole('radio', { name: 'NSFW', exact: true })).toBeChecked()
  await page.emulateMedia({ reducedMotion: 'reduce' }); await expect(selection).toHaveCSS('transition-duration', '0s')
  await track.getByRole('radio', { name: 'Light', exact: true }).hover(); await expect(track.getByRole('radio', { name: 'Light', exact: true })).not.toHaveCSS('background-color', 'rgba(0, 0, 0, 0)')
  await track.getByRole('radio', { name: 'Dark', exact: true }).focus(); await page.keyboard.press('ArrowLeft')
  await expect(track.getByRole('radio', { name: 'Light', exact: true })).toBeFocused(); await expect(track.getByRole('radio', { name: 'Light', exact: true })).toHaveCSS('outline-style', 'solid')
})

test('start: logo navigation has stable first-frame placement and unavailable destinations never navigate', async ({ page }) => {
  await page.setViewportSize({ width: 375, height: 667 }); await english(page)
  await page.evaluate(() => {
    const samples: { x: number; y: number; w: number; h: number; opacity: number }[] = []
    ;(window as unknown as { navigationFrames: typeof samples }).navigationFrames = samples
    document.addEventListener('pointerdown', () => { let count = 0; const sample = () => { const el = document.querySelector('[data-start-brand]'); if (el) { const r = el.getBoundingClientRect(), parent = getComputedStyle(el.parentElement!); samples.push({ x: r.x, y: r.y, w: r.width, h: r.height, opacity: Number(getComputedStyle(el).opacity) * Number(parent.opacity) }) } if (count++ < 40) requestAnimationFrame(sample) }; requestAnimationFrame(sample) }, { once: true })
  })
  const trigger = page.getByRole('button', { name: 'GoFurry brand', exact: true }), popup = page.locator('[data-start-brand]')
  await trigger.click(); await expect(popup).toHaveCSS('opacity', '1')
  const sites = popup.getByRole('link', { name: 'Sites', exact: true })
  const sitesPath = region === 'CN' ? '/en/sites' : '/sites'
  await expect(sites).toHaveAttribute('href', sitesPath)
  await expect(sites).not.toHaveAttribute('aria-disabled')
  const buttons = popup.locator('[aria-disabled="true"]')
  for (const button of await buttons.all()) {
    const bounds = (await button.boundingBox())!
    await page.mouse.click(bounds.x + bounds.width / 2, bounds.y + bounds.height / 2)
    await expect(page).toHaveURL(englishPath); await expect(button).toHaveAccessibleDescription('Not available yet')
    await expect(button).not.toHaveAttribute('title')
  }
  const final = (await popup.boundingBox())!, samples = await page.evaluate(() => (window as unknown as { navigationFrames: { x: number; y: number; w: number; h: number; opacity: number }[] }).navigationFrames)
  const visible = samples.filter(s => s.opacity > 0.01)
  expect(visible.length).toBeGreaterThan(5)
  for (const frame of visible) { expect(frame.x).toBeCloseTo(final.x, 0); expect(frame.w).toBe(final.width); expect(frame.h).toBeCloseTo(final.height, 0); expect(Math.abs(frame.y - final.y)).toBeLessThanOrEqual(4.1) }
  await page.keyboard.press('Escape'); await expect(popup).toHaveCount(0); await expect(trigger).toBeFocused()
  await trigger.press('Enter'); await expect(popup).toHaveCSS('opacity', '1')
  await page.mouse.click(370, 600); await expect(popup).toHaveCount(0); await expect(trigger).toBeFocused()
  await trigger.press('Enter'); await sites.focus(); await sites.press('Enter')
  await expect(page).toHaveURL(sitesPath)
  await expect(popup).toHaveCount(0)
})

test('start: custom clear cancels suggestions, preserves focus and never submits', async ({ page }) => {
  let calls = 0
  await page.route('**/api/v1/start/search/suggestions?*', route => { calls++; return route.fulfill({ headers: { 'X-GoFurry-Region': region, 'X-GoFurry-Content-Mode': 'SFW' }, json: { suggestions: ['golang tutorial'] } }) })
  await english(page); await input(page).fill('golang')
  await expect(page.locator('[data-search-panel][data-open="true"]')).toHaveCSS('opacity', '1')
  const clear = page.getByRole('button', { name: 'Clear search', exact: true }), submit = page.getByRole('button', { name: 'Search (new tab)', exact: true })
  const a = (await clear.boundingBox())!, b = (await submit.boundingBox())!; expect(a.x + a.width).toBeLessThan(b.x)
  await input(page).press('Tab'); await expect(clear).toBeFocused(); await clear.press('Enter')
  await expect(input(page)).toHaveValue(''); await expect(input(page)).toBeFocused(); await expect(clear).toHaveCount(0)
  await expect(page.getByRole('listbox')).toHaveCount(0); expect(page.context().pages()).toHaveLength(1)
  const before = calls; await page.waitForTimeout(700); expect(calls).toBe(before)
  await input(page).fill('next'); await clear.click(); await expect(input(page)).toBeFocused(); await expect(input(page)).toHaveValue('')
})

test('start: all categories remain reachable at 320/375/390/768px with no document overflow', async ({ page }) => {
  await english(page)
  for (const path of [englishPath, chinesePath]) {
    await page.goto(path); await page.waitForLoadState('networkidle')
    for (const width of [320, 375, 390, 768]) {
      await page.setViewportSize({ width, height: 667 })
      const track = page.locator('[data-category-track]'), group = page.locator('[data-categories]')
      const first = group.getByRole('button').first(), last = group.getByRole('button').last()
      await first.focus(); await first.press('End'); await expect(last).toBeFocused()
      const r = (await last.boundingBox())!, t = (await track.boundingBox())!
      expect(r.x).toBeGreaterThanOrEqual(t.x - 1); expect(r.x + r.width).toBeLessThanOrEqual(t.x + t.width + 1)
      await last.press('Home'); await expect(first).toBeFocused()
      const dimensions = await page.evaluate(() => ({ viewport: [innerWidth, innerHeight], root: [document.documentElement.scrollWidth, document.documentElement.scrollHeight], body: [document.body.scrollWidth, document.body.scrollHeight] }))
      expect(dimensions.root).toEqual(dimensions.viewport); expect(dimensions.body).toEqual(dimensions.viewport)
      for (const element of await page.locator('[data-start-actions] > :not([data-start-homepage])').all()) await expect(element).toBeVisible()
    }
  }
  // Stress the documented scroll fallback, rather than only fitting default text.
  await page.setViewportSize({ width: 320, height: 667 })
  await page.locator('[data-categories] button').evaluateAll(elements => elements.forEach(el => { (el as HTMLElement).style.fontSize = '24px' }))
  await page.locator('[data-categories] button').first().focus()
  await expect(page.locator('[data-category-edge="right"]')).toBeVisible()
  await page.keyboard.press('End'); await expect(page.locator('[data-category-edge="left"]')).toBeVisible()
  expect(await page.evaluate(() => document.documentElement.scrollWidth)).toBe(320)
})

test('start: the user menu contains only an inert sign-in placeholder with full exit', async ({ page }) => {
  await english(page)
  const trigger = page.getByRole('button', { name: 'User menu', exact: true }), menu = page.getByRole('menu')
  await trigger.click(); await expect(menu).toHaveCSS('opacity', '1')
  await expect(menu.getByRole('menuitem')).toHaveCount(1); await expect(menu.getByRole('menuitem')).toBeDisabled(); await expect(menu).toContainText('Sign in')
  await page.keyboard.press('Escape'); await expect(menu).toHaveAttribute('data-ending-style'); await expect(menu).toHaveCount(0); await expect(trigger).toBeFocused()
})

test('start: first and repeated popup frames have CSS, stable geometry and complete Motion exits', async ({ page }) => {
  test.setTimeout(45000)
  await page.emulateMedia({ colorScheme: 'light' }); await english(page)
  for (const viewport of [{ width: 960, height: 600 }, { width: 375, height: 667 }]) {
    await page.setViewportSize(viewport)
    for (const [trigger, selector, travel] of [
      ['[data-start-logo]', '[data-start-brand]', 4], ['[data-start-actions] > button:last-child', '[data-start-user-menu]', 4], ['[data-start-settings]', '[data-start-preferences]', 6],
    ] as const) {
      for (let repeat = 0; repeat < 2; repeat++) {
        await page.evaluate(selector => {
          type Frame = { opacity: number; x: number; y: number; w: number; h: number; background: string }
          const frames: Frame[] = []; (window as unknown as { popupFrames: Frame[] }).popupFrames = frames
          let count = 0
          const sample = () => {
            const el = document.querySelector(selector)
            if (el) {
              const r = el.getBoundingClientRect(), s = getComputedStyle(el), parent = getComputedStyle(el.parentElement!)
              frames.push({ opacity: Number(s.opacity) * Number(parent.opacity), x: r.x, y: r.y, w: r.width, h: r.height, background: s.backgroundColor })
            }
            if (count++ < 80) requestAnimationFrame(sample)
          }
          requestAnimationFrame(sample)
        }, selector)
        await page.locator(trigger).click()
        const popup = page.locator(selector); await expect(popup).toHaveCSS('opacity', '1')
        const final = (await popup.boundingBox())!
        const frames = await page.evaluate(() => (window as unknown as { popupFrames: { opacity: number; x: number; y: number; w: number; h: number; background: string }[] }).popupFrames)
        const visible = frames.filter(frame => frame.opacity > 0.01)
        expect(visible.length).toBeGreaterThan(3)
        for (const frame of visible) {
          expect(frame.x).toBeCloseTo(final.x, 0); expect(frame.w).toBeCloseTo(final.width, 0); expect(frame.h).toBeCloseTo(final.height, 0)
          expect(Math.abs(frame.y - final.y)).toBeLessThanOrEqual(travel + 0.1)
          expect(frame.background).toBe('rgba(245, 247, 250, 0.33)')
        }
        await page.keyboard.press('Escape')
        const exit = await sampleMotion(popup)
        expect(exit.some(frame => frame.opacity > 0 && frame.opacity < 1)).toBe(true); expect(exit.at(-1)!.t).toBeGreaterThan(250)
        await expect(popup).toHaveCount(0)
        expect(await page.evaluate(() => document.documentElement.scrollWidth)).toBe(viewport.width)
      }
    }
  }
})

test('start: cold menu fonts are fetched with SSR, not on first open', async ({ page, context, baseURL }, info) => {
  test.setTimeout(45000)
  await context.addCookies([{name:`gf-quick-access-${region}`,value:'1',url:baseURL!}])
  const cdp=await context.newCDPSession(page)
  await cdp.send('DOM.enable');await cdp.send('CSS.enable');await cdp.send('Network.enable')
  await cdp.send('Network.setCacheDisabled',{cacheDisabled:true})
  const errors:string[]=[];page.on('pageerror',e=>errors.push(e.message))
  const fonts=async(selector:string)=>{
    const {root}=await cdp.send('DOM.getDocument'),{nodeId}=await cdp.send('DOM.querySelector',{nodeId:root.nodeId,selector})
    return (await cdp.send('CSS.getPlatformFontsForNode',{nodeId})).fonts
  }
  const evidence=[]
  for(const [path,locale] of [[chinesePath,'zh'],[englishPath,'en']] as const){
    await page.goto(path);await page.waitForLoadState('networkidle');await page.evaluate(()=>document.fonts.ready)
    await expect(page.locator('link[rel=preload][as=font]')).toHaveCount(locale==='zh'?2:0)
    const lateFonts:string[]=[];const record=(r:import('@playwright/test').Request)=>{if(r.resourceType()==='font')lateFonts.push(r.url())}
    page.on('request',record)
    for(const [trigger,selector,text] of [
      ['[data-start-logo]','[data-start-brand]','[data-start-brand] nav > :nth-child(2)'],
      ['[data-start-actions] > button:last-child','[data-start-user-menu]','[data-start-user-menu] [role=menuitem]'],
      ['[data-mobile-platform-trigger]','[data-search-panel=platforms]','[data-search-panel=platforms] button:nth-child(4)'],
    ] as const){
      for(let repeat=0;repeat<2;repeat++){
        await page.locator(trigger).click()
        const label = page.locator(text)
        await label.waitFor({ state: 'visible' })
        const first=await fonts(text),initial=await label.evaluate(el=>({font:getComputedStyle(el).font,width:el.getBoundingClientRect().width}))
        expect(first.length).toBeGreaterThan(0);expect(first.every(f=>f.isCustomFont)).toBe(true)
        await expect(page.locator(selector)).toHaveCSS('opacity','1')
        expect(await fonts(text)).toEqual(first)
        expect(await page.locator(text).evaluate(el=>({font:getComputedStyle(el).font,width:el.getBoundingClientRect().width}))).toEqual(initial)
        evidence.push({locale,selector,repeat,fonts:first,style:initial})
        if(!repeat)await page.screenshot({path:info.outputPath(`${locale}-${selector.includes('brand')?'brand':selector.includes('user')?'user':'platform'}.png`)})
        await page.keyboard.press('Escape');await expect(page.locator(selector)).toHaveCount(0)
      }
    }
    page.off('request',record);expect(lateFonts).toEqual([])
  }
  await info.attach('font-evidence',{body:JSON.stringify(evidence,null,2),contentType:'application/json'})
  expect(errors).toEqual([])
})

test.describe('start: popup focus and scaled-display text', () => {
  test.use({ deviceScaleFactor: 1.25 })

  test('pointer opening never focuses Home; keyboard entry retains a visible focus indicator', async ({ page }) => {
    await page.goto(chinesePath); await page.waitForLoadState('networkidle')
    const trigger=page.locator('[data-start-logo]').locator('..'),popup=page.locator('[data-start-brand]'),home=popup.locator('nav > :first-child')
    for(const colorScheme of ['light','dark'] as const){
      await page.emulateMedia({colorScheme})
      for(const fromInput of [false,true]){
        if(fromInput)await input(page).click()
        await trigger.click();await expect(popup).toBeFocused();await expect(home).not.toBeFocused()
        const frames=await home.evaluate(el=>new Promise<string[]>(resolve=>{
          const frames:string[]=[],start=performance.now()
          const sample=()=>{frames.push(getComputedStyle(el).outlineStyle);if(performance.now()-start>=480)resolve(frames);else requestAnimationFrame(sample)};sample()
        }))
        expect(new Set(frames)).toEqual(new Set(['none']))
        await page.keyboard.press('Tab');await expect(home).toBeFocused();await expect(home).toHaveCSS('outline-style','solid')
        await page.keyboard.press('Escape');await expect(popup).toHaveCount(0);await expect(trigger).toBeFocused()
      }
      await trigger.press('Enter');await expect(home).toBeFocused();await expect(home).toHaveCSS('outline-style','solid')
      await page.keyboard.press('Escape');await expect(popup).toHaveCount(0);await expect(trigger).toBeFocused()
      await trigger.click();await expect(popup).toBeFocused()
      await page.mouse.click(900,800);await expect(popup).toHaveCount(0)
    }
  })

  test('Logo, User and both search lists fade on a fixed text grid, including full exits', async ({ page, context, baseURL }, info) => {
    await context.addCookies([{name:`gf-quick-access-${region}`,value:'1',url:baseURL!}])
    await page.route('**/api/v1/start/search/suggestions?*',r=>r.fulfill({headers:{'X-GoFurry-Region':region,'X-GoFurry-Content-Mode':'SFW'},json:{suggestions:['furry Google','furry DuckDuckGo']}}))
    await english(page)
    const evidence=[]
    for(const kind of ['brand','user','platforms','suggestions']){
      const selector=kind==='brand'?'[data-start-brand]':kind==='user'?'[data-start-user-menu]':'[data-search-panel]'
      await page.evaluate(selector=>{
        type Frame={x:number;y:number;width:number;font:string;opacity:number;transform:string}
        const frames:Frame[]=[];(window as unknown as {stableTextFrames:Frame[]}).stableTextFrames=frames
        const start=performance.now()
        const sample=()=>{
          const popup=document.querySelector(selector),row=popup?.querySelector('nav > :first-child,[role=menuitem],[role=option]')
          if(popup && row){
            const walker=document.createTreeWalker(row,NodeFilter.SHOW_TEXT);let node:Node|null
            while((node=walker.nextNode()) && !node.textContent?.trim()){}
            if(node){const range=document.createRange();range.selectNodeContents(node);const r=range.getBoundingClientRect(),s=getComputedStyle(popup)
              frames.push({x:r.x,y:r.y,width:r.width,font:getComputedStyle(row).font,opacity:Number(s.opacity),transform:s.transform})}
          }
          if(performance.now()-start<1600)requestAnimationFrame(sample)
        };requestAnimationFrame(sample)
      },selector)
      if(kind==='suggestions')await input(page).fill('furry')
      else await page.locator(kind==='brand'?'[data-start-logo]':kind==='user'?'[data-start-actions] > button:last-child':'[data-mobile-platform-trigger]').click()
      const popup=page.locator(selector);await expect(popup).toHaveCSS('opacity','1')
      const frames=await page.evaluate(()=>(window as unknown as {stableTextFrames:{x:number;y:number;width:number;font:string;opacity:number;transform:string}[]}).stableTextFrames.filter(f=>f.opacity>.01))
      expect(frames.length).toBeGreaterThan(3);expect(frames.some(f=>f.opacity<1)).toBe(true)
      for(const key of ['x','y','width'] as const)expect(Math.max(...frames.map(f=>f[key]))-Math.min(...frames.map(f=>f[key]))).toBeLessThan(.01)
      expect(new Set(frames.map(f=>f.font)).size).toBe(1);expect(new Set(frames.map(f=>f.transform))).toEqual(new Set(['none']))
      evidence.push({kind,frames});await page.screenshot({path:info.outputPath(kind+'.png')})
      await page.keyboard.press('Escape');const exit=await sampleMotion(popup)
      expect(exit.some(f=>f.opacity>0 && f.opacity<1)).toBe(true);expect(exit.at(-1)!.t).toBeGreaterThan(250)
      const painted=exit.filter(f=>f.opacity>.01 && f.w>0 && f.h>0)
      expect(Math.max(...painted.map(f=>f.y))-Math.min(...painted.map(f=>f.y))).toBeLessThan(.01)
      await expect(popup).toHaveCount(0)
    }
    await info.attach('stable-text-frames',{body:JSON.stringify(evidence),contentType:'application/json'})
  })
})
