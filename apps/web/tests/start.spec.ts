import { test, expect, type Page } from '@playwright/test'

const region = process.env.WEB_REGION ?? 'CN'
const defaultLocale = region === 'CN' ? 'zh' : 'en'
const action = region === 'CN' ? 'https://www.bing.com/search' : 'https://www.google.com/search'
const origin = 'http://127.0.0.1:17320'
const placeholder = { zh: '搜索站点或内容…', en: 'Search sites or content…' }
test.use({ viewport: { width: 1440, height: 900 } })

async function english(page: Page) {
  await page.context().addCookies([{ name: 'gf-locale', value: 'en', url: origin }])
  await page.goto('/start')
}
async function interceptNavigation(page: Page) {
  await page.context().route('https://**/*', (route) => route.request().isNavigationRequest()
    ? route.fulfill({ contentType: 'text/html', body: '<!doctype html><title>Search target</title>' }) : route.continue())
}
async function submitted(page: Page, submit: () => Promise<unknown>) {
  const pending = page.waitForEvent('popup')
  await submit()
  const popup = await pending
  await popup.waitForLoadState('domcontentloaded')
  const url = new URL(popup.url())
  await popup.close()
  return url
}

test('start: SSR uses the themed legacy SVG, regional defaults and plain CN-only filing text', async ({ browser }) => {
  const context = await browser.newContext({ javaScriptEnabled: false })
  const page = await context.newPage()
  const response = await page.goto('/start')
  expect(response!.status()).toBe(200)
  expect(response!.headers()['cache-control']).toContain('no-store')
  await expect(page.locator('html')).toHaveAttribute('lang', defaultLocale)
  await expect(page.locator('html')).toHaveAttribute('data-region', region)
  await expect(page.getByRole('search')).toHaveAttribute('action', action)
  await expect(page.getByRole('searchbox')).toHaveAttribute('placeholder', placeholder[defaultLocale])
  const logo = page.getByRole('img', { name: 'GoFurry', exact: true })
  await expect(logo).toHaveCSS('background-color', 'rgb(215, 220, 226)')
  expect(await logo.evaluate((el) => getComputedStyle(el).maskImage)).toMatch(/url\(.*(?:\.svg|image\/svg)/)
  const bounds = (await logo.boundingBox())!
  expect(bounds.width / bounds.height).toBeCloseTo(1287 / 1024, 2)
  expect(await page.locator('header').innerText()).not.toContain('GoFurry')
  await expect(page.locator('[data-start-actions] > *')).toHaveCount(4)
  await expect(page.locator('[data-start-actions] > button:disabled')).toHaveCount(0)
  await expect(page.locator('[data-start-filing]')).toHaveCount(region === 'CN' ? 1 : 0)
  await expect(page.locator('[data-start-filing] a')).toHaveCount(0)
  if (region === 'CN') {
    await expect(page.locator('footer')).toContainText('待配置')
    await expect(page.locator('footer')).toContainText('·')
    for (const text of await page.locator('footer, footer > span, footer > div').all()) {
      await expect(text).toHaveCSS('background-color', 'rgba(0, 0, 0, 0)')
      await expect(text).toHaveCSS('box-shadow', 'none')
      await expect(text).toHaveCSS('backdrop-filter', 'none')
      await expect(text).toHaveCSS('padding', '0px')
      await expect(text).toHaveCSS('border-radius', '0px')
      await expect(text).toHaveCSS('text-shadow', 'none')
      await expect(text).toHaveCSS('filter', 'none')
      await expect(text).toHaveCSS('-webkit-text-stroke-width', '0px')
    }
  }
  // The default native GET search still works before hydration/without JS.
  await interceptNavigation(page)
  await page.getByRole('searchbox').fill('GoFurry 兽人 & 雪')
  const url = await submitted(page, () => page.getByRole('searchbox').press('Enter'))
  expect(url.origin).toBe(new URL(action).origin)
  expect(url.searchParams.get('q')).toBe('GoFurry 兽人 & 雪')
  await context.close()
})

test('start: top actions persist theme/locale, keep region and open the actual GitHub target', async ({ page }) => {
  const errors: string[] = []
  page.on('pageerror', (error) => errors.push(error.message))
  page.on('console', (entry) => { if (entry.type() === 'error') errors.push(entry.text()) })
  await english(page)
  const actions = page.locator('[data-start-actions]')
  const language = actions.locator(':scope > *').nth(0)
  const geometry = await language.boundingBox()
  const typography = await language.evaluate((el) => { const s = getComputedStyle(el); return [s.fontSize, s.fontWeight, s.padding, s.textAlign, s.lineHeight] })
  await expect(page.locator('[data-theme-icon]')).toHaveAttribute('data-theme-icon', 'sun')
  await expect(actions.locator(':scope > *').nth(0)).toHaveText('EN')
  await actions.getByRole('button', { name: 'Switch to 中文' }).click()
  await expect(page.locator('html')).toHaveAttribute('lang', 'zh')
  expect(await language.boundingBox()).toEqual(geometry)
  expect(await language.evaluate((el) => { const s = getComputedStyle(el); return [s.fontSize, s.fontWeight, s.padding, s.textAlign, s.lineHeight] })).toEqual(typography)
  await expect(page.getByRole('searchbox')).toHaveAttribute('placeholder', placeholder.zh)
  await actions.getByRole('button', { name: '切换为深色主题' }).click()
  await expect(page.locator('html')).toHaveAttribute('data-theme', 'dark')
  await expect(page.locator('[data-theme-icon]')).toHaveAttribute('data-theme-icon', 'moon')
  await expect(page.locator('[data-start-logo]')).toHaveCSS('background-color', 'rgb(215, 220, 226)')
  const response = await page.reload()
  expect(await response!.text()).toContain('data-theme="dark"')
  await expect(page.locator('html')).toHaveAttribute('data-theme', 'dark')
  await expect(page.locator('html')).toHaveAttribute('lang', 'zh')
  await expect(page.locator('html')).toHaveAttribute('data-region', region)
  await actions.getByRole('button', { name: '切换为 English' }).click()
  await actions.getByRole('button', { name: 'Switch to light theme' }).click()
  await expect(page.locator('html')).toHaveAttribute('data-theme', 'light')
  if (region === 'CN') await expect(page.locator('footer')).toContainText('Chengdu Yuanan Kaiwu Technology Co., Ltd.')
  await expect(actions.getByRole('button', { name: 'User menu', exact: true })).toBeEnabled()
  await expect(actions.getByRole('button', { name: /Settings/ })).toHaveCount(0)
  await expect(page.getByRole('dialog')).toHaveCount(0)
  const github = actions.getByRole('link')
  await expect(github).toHaveAttribute('href', 'https://github.com/gofurry')
  await expect(github).toHaveAttribute('rel', 'noopener noreferrer')
  await interceptNavigation(page)
  const url = await submitted(page, () => github.click())
  expect(url.href).toBe('https://github.com/gofurry')
  expect(errors).toEqual([])
})

test('start: Light/Dark pointer, active and keyboard states use one focus frame', async ({ page }) => {
  await english(page)
  for (const theme of ['light', 'dark']) {
    await page.context().addCookies([{ name: 'gf-theme', value: theme, url: origin }])
    await page.reload()
    const form = page.getByRole('search'), input = page.getByRole('searchbox')
    const bounds = await form.boundingBox()
    const background = await form.evaluate((el) => getComputedStyle(el).backgroundColor)
    await input.hover()
    await expect(form).toHaveCSS('background-color', background)
    await expect(form).toHaveCSS('outline-style', 'none')
    await page.mouse.down()
    await expect(form).toHaveCSS('border-top-width', '0px')
    await page.mouse.up()
    await expect(input).toBeFocused()
    await expect(form).toHaveCSS('outline-style', 'none')
    await expect(input).toHaveCSS('outline-style', 'none')
    await page.getByRole('group', { name: 'Search categories' }).getByRole('button').last().focus()
    await page.keyboard.press('Tab')
    await expect(input).toBeFocused()
    await expect(form).toHaveCSS('outline-style', 'none')
    await expect(form).toHaveCSS('border-top-width', '0px')
    await expect(input).toHaveCSS('outline-style', 'none')
    await expect.poll(() => form.evaluate((el) => getComputedStyle(el).boxShadow)).toContain('0px 0px 0px 2px')
    expect(await form.boundingBox()).toEqual(bounds)
    await input.click()
    await expect(form).toHaveCSS('outline-style', 'none')
    const toggle = page.locator('[data-start-actions] > button').nth(1)
    await toggle.hover()
    await expect(toggle).toHaveCSS('outline-style', 'none')
    await page.mouse.down()
    await expect(toggle).toHaveCSS('border-top-width', '0px')
    await page.mouse.up()
    await expect(page.locator('html')).not.toHaveAttribute('data-gf-theme-reveal')
    await page.locator('[data-start-actions] > button').first().focus()
    await page.keyboard.press('Tab')
    await expect(toggle).toBeFocused()
    await expect(toggle).toHaveCSS('outline-style', 'solid')
    await expect(toggle).toHaveCSS('outline-width', '2px')
    await expect(toggle).toHaveCSS('outline-offset', '-2px')
    const focusSurface = theme === 'light' ? 'rgba(58, 67, 80, 0.76)' : 'rgba(231, 235, 241, 0.76)'
    await expect(toggle).toHaveCSS('background-color', focusSurface)
    await toggle.hover()
    await expect(toggle).toHaveCSS('background-color', focusSurface)
    expect(await toggle.evaluate((el) => getComputedStyle(el).outlineColor)).not.toBe('rgb(0, 0, 0)')
    await page.keyboard.press('Tab')
    const github = page.locator('[data-start-actions] > a')
    await expect(github).toBeFocused()
    await expect(github).toHaveCSS('outline-style', 'solid')
    await expect(github).toHaveCSS('border-top-width', '0px')
    await expect(github).toHaveCSS('background-color', focusSurface)
    await expect(github).toHaveCSS('box-shadow', 'none')
  }
})

test('start: categories, platform URL encoding and composition-safe submission work without legacy suggestions', async ({ page }) => {
  const suggestionRequests: string[] = []
  page.on('request', (request) => { if (/suggest|go-furry.com/.test(request.url())) suggestionRequests.push(request.url()) })
  await english(page)
  await interceptNavigation(page)
  const input = page.getByRole('searchbox')
  const query = 'wolf & 兽人 +?/#'
  const cat = page.getByRole('group', { name: 'Search categories' })
  const providers = page.locator('[data-desktop-platforms]')
  for (const [category, provider, host, parameter] of [
    ['Search', region === 'CN' ? 'Bing' : 'Google', new URL(action).hostname, 'q'],
    ['Search', 'DuckDuckGo', 'duckduckgo.com', 'q'],
    ['Search', 'Bilibili', 'search.bilibili.com', 'keyword'],
    ['Games', 'Epic', 'store.epicgames.com', 'q'],
    ['Art', 'ZCool', 'www.zcool.com.cn', 'word'],
    ['Novel', 'Project Gutenberg', 'www.gutenberg.org', 'query'],
  ]) {
    await cat.getByRole('button', { name: category, exact: true }).click()
    await providers.getByRole('button', { name: provider, exact: true }).click()
    await input.fill(`  ${query}  `)
    const url = await submitted(page, () => input.press('Enter'))
    expect(url.hostname).toBe(host)
    expect(url.searchParams.get(parameter)).toBe(query)
    if (provider === 'Bing') expect(url.searchParams.get('adlt')).toBe('strict')
    if (provider === 'Google') expect(url.searchParams.get('safe')).toBe('active')
    if (provider === 'DuckDuckGo') expect(url.searchParams.get('kp')).toBe('1')
  }
  await cat.getByRole('button', { name: 'Furry', exact: true }).click()
  await input.fill(query)
  let url = await submitted(page, () => input.press('Enter'))
  expect(url.hostname).toBe('en.wikifur.com')
  expect(decodeURIComponent(url.pathname)).toBe(`/wiki/${query}`)
  await page.locator('[data-start-actions] > button').first().click()
  url = await submitted(page, () => input.press('Enter'))
  expect(url.hostname).toBe('zh.wikifur.com')
  await input.dispatchEvent('compositionstart')
  await input.fill('中文输入法')
  const pages = page.context().pages().length
  await input.press('Enter')
  await input.dispatchEvent('keydown', { key: 'Enter', isComposing: true, keyCode: 229, bubbles: true, cancelable: true })
  expect(page.context().pages()).toHaveLength(pages)
  await input.dispatchEvent('compositionend', { data: '中文输入法' })
  expect(await input.evaluate((element) => ['ArrowDown', 'ArrowUp'].map((key) => {
    const event = new KeyboardEvent('keydown', { key, bubbles: true, cancelable: true })
    element.dispatchEvent(event)
    return event.defaultPrevented
  }))).toEqual([false, false])
  await input.press('ArrowDown'); await input.press('ArrowUp'); await input.press('Escape')
  await expect(input).toHaveValue('中文输入法')
  await expect(page.getByRole('listbox')).toHaveCount(0)
  await expect(page.getByRole('progressbar')).toHaveCount(0)
  url = await submitted(page, () => input.press('Enter'))
  expect(decodeURIComponent(url.pathname)).toBe('/wiki/中文输入法')
  await input.fill('   '); await input.press('Enter')
  expect(page.context().pages()).toHaveLength(pages)
  expect(suggestionRequests).toEqual([])
})

test('start: mobile platform slots, actual target, arrows, Escape and breakpoint focus remain usable', async ({ page }) => {
  await page.setViewportSize({ width: 390, height: 844 })
  await english(page)
  const picker = page.locator('[data-mobile-platforms]')
  const trigger = picker.locator('[aria-expanded]')
  await expect(picker.getByRole('button')).toHaveCount(1)
  await trigger.click()
  await expect(trigger).toHaveAttribute('aria-expanded', 'true')
  await expect(picker.getByRole('button')).toHaveCount(7)
  await picker.getByRole('button', { name: 'DuckDuckGo', exact: true }).click()
  await expect(trigger).toHaveText('DuckDuckGo')
  await interceptNavigation(page)
  await page.getByRole('searchbox').fill('实际平台 & wolf')
  const target = await submitted(page, () => page.getByRole('searchbox').press('Enter'))
  expect(target.hostname).toBe('duckduckgo.com')
  expect(target.searchParams.get('q')).toBe('实际平台 & wolf')
  await trigger.click()
  await trigger.press('ArrowDown')
  expect(await picker.evaluate((el) => el.contains(document.activeElement))).toBe(true)
  await page.keyboard.press('Enter')
  await expect(trigger).toBeFocused()
  await expect(trigger).toHaveAttribute('aria-expanded', 'true')
  await trigger.press('Escape')
  await expect(picker.getByRole('button')).toHaveCount(1)
  await expect(trigger).toBeFocused()
  await trigger.click()
  await page.getByRole('searchbox').click()
  await expect(trigger).toHaveAttribute('aria-expanded', 'false')
  await trigger.click()
  await page.locator('[data-start-logo]').click()
  await expect(trigger).toHaveAttribute('aria-expanded', 'false')
  await trigger.click()
  await page.setViewportSize({ width: 1000, height: 800 })
  await expect(page.locator('[data-categories] [aria-pressed="true"]')).toBeFocused()
  await expect(trigger).toHaveAttribute('aria-expanded', 'false')
  await page.setViewportSize({ width: 390, height: 844 })
  await page.getByRole('group', { name: 'Search categories' }).getByRole('button', { name: 'Furry' }).click()
  await expect(trigger).toBeDisabled()
  await expect(trigger).toHaveText('WikiFur')
})

test('start: desktop/mobile controls keep their position with four visible actions and unobstructed CN footer', async ({ page }, testInfo) => {
  await english(page)
  for (const viewport of [{ width: 1440, height: 900 }, { width: 390, height: 844 }, { width: 320, height: 568 }, { width: 844, height: 390 }]) {
    await page.setViewportSize(viewport)
    const group = (await page.locator('[data-start-search]').boundingBox())!
    const header = (await page.locator('header').boundingBox())!
    expect(Math.abs(group.x + group.width / 2 - viewport.width / 2)).toBeLessThanOrEqual(1)
    const center = group.y + group.height / 2
    expect(center).toBeLessThan(viewport.height * 0.43)
    if (viewport.height > 700) { expect(center / viewport.height).toBeGreaterThan(0.24); expect(center / viewport.height).toBeLessThan(0.31) }
    const firstCategory = (await page.locator('[data-categories] button').first().boundingBox())!
    expect(firstCategory.y).toBeGreaterThanOrEqual(header.y + header.height + 8)
    expect(await page.evaluate(() => document.documentElement.scrollWidth)).toBe(viewport.width)
    await expect(page.locator('[data-start-actions] > *')).toHaveCount(4)
    for (const button of await page.locator('[data-start-actions] > *').all()) {
      await expect(button).toBeVisible()
      const bounds = (await button.boundingBox())!
      expect(bounds.x).toBeGreaterThanOrEqual(0)
      expect(bounds.x + bounds.width).toBeLessThanOrEqual(viewport.width)
    }
    if (region === 'CN') {
      const footer = (await page.locator('footer').boundingBox())!
      expect(footer.y).toBeGreaterThan(group.y + group.height + 12)
      expect(footer.x + footer.width).toBeLessThanOrEqual(viewport.width)
    } else await expect(page.locator('footer')).toHaveCount(0)
    if (viewport.width === 390) await page.screenshot({ path: testInfo.outputPath(`start-${region}-mobile-layout.png`) })
  }
})

test('start: native theme reveal uses the button center, final snapshots and legacy timing with clean fallbacks', async ({ page }) => {
  const selectors = ['[data-start-logo]', '[data-start-actions] > a', '[data-categories] [aria-pressed="true"]', '[data-categories] [aria-pressed="false"]', 'form[role="search"]', 'input[type="search"]', '[data-desktop-platforms] [aria-pressed="true"]', '[data-desktop-platforms] [aria-pressed="false"]', 'footer']
  await page.addInitScript((selectors) => {
    const probe = { starts: 0, animations: [] as { frames: unknown; options: unknown; theme: string | null; oldZ: string; newZ: string; oldAnimation: string; newAnimation: string; colors: string[] }[] }
    ;(window as unknown as { revealProbe: typeof probe }).revealProbe = probe
    const start = document.startViewTransition?.bind(document)
    if (start) document.startViewTransition = (update) => { probe.starts++; return start(update) }
    const animate = Element.prototype.animate
    Element.prototype.animate = function (frames, options) {
      const animation = animate.call(this, frames, options)
      if (typeof options === 'object' && options.pseudoElement === '::view-transition-new(root)') {
        const old = getComputedStyle(document.documentElement, '::view-transition-old(root)')
        const next = getComputedStyle(document.documentElement, '::view-transition-new(root)')
        probe.animations.push({ frames, options, theme: document.documentElement.getAttribute('data-theme'),
          oldZ: old.zIndex, newZ: next.zIndex, oldAnimation: old.animationName, newAnimation: next.animationName,
          colors: selectors.map((selector) => { const el = document.querySelector(selector); if (!el) return ''; const s = getComputedStyle(el); return `${s.backgroundColor}|${s.color}` }) })
      }
      return animation
    }
  }, selectors)
  await english(page)
  const button = page.locator('[data-start-actions] > button').nth(1)
  const root = page.locator('html')
  const probe = () => page.evaluate(() => (window as unknown as { revealProbe: { starts: number; animations: { frames: unknown; options: unknown; theme: string; oldZ: string; newZ: string; oldAnimation: string; newAnimation: string; colors: string[] }[] } }).revealProbe)
  const colors = () => page.evaluate((selectors) => selectors.map((selector) => { const el = document.querySelector(selector); if (!el) return ''; const s = getComputedStyle(el); return `${s.backgroundColor}|${s.color}` }), selectors)
  for (const target of ['dark', 'light']) {
    const bounds = (await button.boundingBox())!, viewport = page.viewportSize()!
    const x = bounds.x + bounds.width / 2, y = bounds.y + bounds.height / 2
    const before = (await probe()).starts
    const beforeColors = await colors()
    await button.click({ clickCount: 2, delay: 20 })
    await expect(root).toHaveAttribute('data-theme', target)
    await expect(root).not.toHaveAttribute('data-gf-theme-reveal')
    const result = await probe()
    expect(result.starts).toBe(before + 1)
    expect(result.animations.at(-1)).toEqual({
      frames: { clipPath: [`circle(0px at ${x}px ${y}px)`, `circle(${Math.hypot(Math.max(x, viewport.width - x), Math.max(y, viewport.height - y))}px at ${x}px ${y}px)`] },
      options: { duration: 420, easing: 'cubic-bezier(0.22, 1, 0.36, 1)', pseudoElement: '::view-transition-new(root)', fill: 'both' },
      theme: target, oldZ: '1', newZ: '2', oldAnimation: 'none', newAnimation: 'none',
      colors: await colors(),
    })
    const revealed = result.animations.at(-1)!.colors
    revealed.forEach((color, index) => { if (color && index !== 0) expect(color).not.toBe(beforeColors[index]) })
    expect((await page.context().cookies()).find((cookie) => cookie.name === 'gf-theme')?.value).toBe(target)
  }
  // A resize interrupts the live snapshot without a second theme commit or a
  // stranded top-layer animation. A later click must remain usable.
  await button.click()
  await expect(root).toHaveAttribute('data-theme', 'dark')
  await page.setViewportSize({ width: 1200, height: 800 })
  await expect(root).not.toHaveAttribute('data-gf-theme-reveal')
  const starts = (await probe()).starts
  await page.emulateMedia({ reducedMotion: 'reduce' })
  await button.click()
  await expect(root).toHaveAttribute('data-theme', 'light')
  expect((await probe()).starts).toBe(starts)
  await expect(root).not.toHaveAttribute('data-gf-theme-reveal')
  await page.emulateMedia({ reducedMotion: 'no-preference' })
  await page.evaluate(() => Object.defineProperty(document, 'startViewTransition', { value: undefined, configurable: true }))
  await button.click()
  await expect(root).toHaveAttribute('data-theme', 'dark')
  expect((await probe()).starts).toBe(starts)
  await expect(root).not.toHaveAttribute('data-gf-theme-reveal')
  const response = await page.reload()
  expect(await response!.text()).toContain('data-theme="dark"')
})

test('start: Light/Dark acrylic, placeholders and footer actually change, with smooth flat hover states', async ({ page }) => {
  await english(page)
  const palettes: Record<string, Record<string, string[]>> = {}
  for (const theme of ['light', 'dark']) {
    await page.context().addCookies([{ name: 'gf-theme', value: theme, url: origin }])
    await page.reload()
    const user = page.getByRole('button', { name: 'User menu', exact: true })
    await user.click()
    const menu = page.getByRole('menu')
    await expect(menu).toHaveCSS('opacity', '1')
    palettes[theme] = await page.evaluate(() => {
      const selectors = {
        logo: '[data-start-logo]', toolbar: '[data-start-actions] > a',
        category: '[data-categories] [aria-pressed="false"]', selectedCategory: '[data-categories] [aria-pressed="true"]',
        input: 'form[role="search"]', inputText: 'input[type="search"]',
        platform: '[data-desktop-platforms] [aria-pressed="false"]', selectedPlatform: '[data-desktop-platforms] [aria-pressed="true"]',
        mobilePlatform: '[data-mobile-platforms] button', footer: 'footer', menu: '[role="menu"]', item: '[role="menuitem"]',
      }
      const result: Record<string, string[]> = {}
      for (const [key, selector] of Object.entries(selectors)) {
        const el = document.querySelector(selector)
        if (el) { const s = getComputedStyle(el); result[key] = [s.backgroundColor, s.color] }
      }
      result.placeholder = [getComputedStyle(document.querySelector('input[type="search"]')!, '::placeholder').color]
      return result
    })
    for (const control of await page.locator('[data-start-actions] > *, [role="menu"]').all()) {
      await expect(control).toHaveCSS('box-shadow', 'none')
      await expect(control).toHaveCSS('border-top-width', '0px')
    }
    const github = page.locator('[data-start-actions] > a')
    const background = await github.evaluate((el) => getComputedStyle(el).backgroundColor)
    await expect(github).toHaveCSS('transition-duration', '0.3s')
    await expect(github).toHaveCSS('transition-timing-function', 'cubic-bezier(0.22, 1, 0.36, 1)')
    await github.hover()
    await expect.poll(() => github.evaluate((el) => getComputedStyle(el).backgroundColor)).not.toBe(background)
    await expect(github).toHaveCSS('transform', 'none')
    await expect(github).toHaveCSS('outline-style', 'none')
    await page.mouse.move(0, 0)
    await page.keyboard.press('Escape')
    await expect(menu).toHaveCount(0)
  }
  expect(Object.keys(palettes.light)).toEqual(Object.keys(palettes.dark))
  expect(palettes.light.logo[0]).toBe(palettes.dark.logo[0])
  for (const role of Object.keys(palettes.light)) if (role !== 'logo') expect(palettes.light[role]).not.toEqual(palettes.dark[role])
})

test('start: constant-gray logo button has flat hover, a reversible brand-only popup and keyboard dismissal', async ({ page }) => {
  await english(page)
  const logo = page.locator('[data-start-logo]')
  const trigger = page.getByRole('button', { name: 'GoFurry brand', exact: true })
  const popup = page.locator('[data-start-brand]')
  for (const theme of ['light', 'dark']) {
    await page.context().addCookies([{ name: 'gf-theme', value: theme, url: origin }])
    await page.reload()
    await page.mouse.move(0, 0)
    await expect(logo).toHaveCSS('background-color', 'rgb(215, 220, 226)')
    await expect(trigger).toHaveCSS('background-color', 'rgba(0, 0, 0, 0)')
    await expect(trigger).toHaveCSS('border-top-width', '0px')
    await expect(trigger).toHaveCSS('box-shadow', 'none')
    await expect(trigger).toHaveCSS('transition-duration', '0.3s')
    const actions = await page.locator('[data-start-actions]').boundingBox()
    const geometry = await logo.boundingBox()
    await trigger.hover()
    await expect(trigger).toHaveCSS('background-color', 'rgba(224, 230, 238, 0.14)')
    await expect(trigger).toHaveCSS('outline-style', 'none')
    await trigger.click()
    await expect(trigger).toHaveAttribute('aria-expanded', 'true')
    await expect(trigger).toHaveAttribute('aria-controls', (await popup.getAttribute('id'))!)
    await expect(popup).toHaveCSS('opacity', '1')
    await expect(popup).toHaveText('GoFurry')
    await expect(popup.getByRole('button')).toHaveCount(0)
    expect(await page.locator('[data-start-actions]').boundingBox()).toEqual(actions)
    expect(await logo.boundingBox()).toEqual(geometry)
    await trigger.click()
    await expect(trigger).toHaveAttribute('aria-expanded', 'false')
    await expect(popup).toHaveAttribute('data-ending-style')
    await expect(popup).toHaveCount(0)
    await trigger.focus(); await trigger.press('Enter')
    await expect(popup).toHaveCSS('opacity', '1')
    await expect(trigger).toHaveCSS('outline-style', 'solid')
    await page.keyboard.press('Escape')
    await expect(popup).toHaveCount(0)
    await expect(trigger).toBeFocused()
  }
})

test('start: viewport contract bounds all layers and content without document overflow or changing other routes', async ({ page }) => {
  await english(page)
  for (const [width, height] of [[960, 600], [1280, 720], [1920, 1080], [390, 844], [375, 667], [844, 390], [600, 360]]) {
    await page.setViewportSize({ width, height })
    const dimensions = await page.evaluate(() => {
      const size = (el: Element) => ({ client: [el.clientWidth, el.clientHeight], scroll: [el.scrollWidth, el.scrollHeight] })
      const bounds = (selector: string) => { const r = document.querySelector(selector)!.getBoundingClientRect(); return [r.x, r.y, r.right, r.bottom] }
      return { viewport: [innerWidth, innerHeight], html: size(document.documentElement), body: size(document.body), main: size(document.querySelector('main')!),
        page: bounds('[data-start-page]'), background: bounds('[data-start-background]'), dots: bounds('[data-page-dots]'), header: bounds('header'), search: bounds('[data-start-search]'),
        footer: document.querySelector('footer') ? bounds('footer') : null }
    })
    for (const element of [dimensions.html, dimensions.body, dimensions.main]) {
      expect(element.client).toEqual([width, height])
      expect(element.scroll).toEqual(element.client)
    }
    expect(dimensions.page).toEqual([0, 0, width, height])
    expect(dimensions.background).toEqual(dimensions.page)
    for (const bounds of [dimensions.dots, dimensions.header, dimensions.search, dimensions.footer].filter(Boolean) as number[][]) {
      expect(bounds[0]).toBeGreaterThanOrEqual(0); expect(bounds[1]).toBeGreaterThanOrEqual(0)
      expect(bounds[2]).toBeLessThanOrEqual(width); expect(bounds[3]).toBeLessThanOrEqual(height)
    }
    await page.evaluate(() => window.scrollTo(1000, 1000))
    expect(await page.evaluate(() => [scrollX, scrollY])).toEqual([0, 0])
    // Expanded mobile choices must also fit, rather than being clipped away.
    if (width < 768) {
      const trigger = page.locator('[data-mobile-platforms] [aria-expanded]')
      await trigger.click()
      for (const option of await page.locator('[data-mobile-platforms] button').all()) {
        const r = (await option.boundingBox())!
        expect(r.y + r.height).toBeLessThan(height - 48)
      }
      await page.keyboard.press('Escape')
    }
  }
  await page.setViewportSize({ width: 960, height: 600 })
  await page.goto('/')
  await expect(page.locator('[data-start-page]')).toHaveCount(0)
  await expect(page.locator('body')).toHaveCSS('min-height', '600px')
  await expect(page.locator('[data-page-dots]')).toHaveCSS('max-height', 'none')
})

test('start: user menu has complete exit motion, inert placeholders and keyboard focus on desktop/mobile', async ({ page }) => {
  for (const [width, height, locale, theme] of [[1440, 900, 'zh', 'light'], [1440, 900, 'en', 'dark'], [390, 844, 'zh', 'dark'], [390, 844, 'en', 'light']] as const) {
    await page.setViewportSize({ width, height })
    await page.context().addCookies([{ name: 'gf-locale', value: locale, url: origin }, { name: 'gf-theme', value: theme, url: origin }])
    await page.goto('/start')
    const user = page.getByRole('button', { name: locale === 'zh' ? '用户菜单' : 'User menu', exact: true })
    const menu = page.getByRole('menu')
    const layout = await page.locator('[data-start-search]').boundingBox()
    await user.click()
    await expect(user).toHaveAttribute('aria-expanded', 'true')
    await expect(user).toHaveAttribute('aria-controls', (await menu.getAttribute('id'))!)
    await expect(menu).toHaveCSS('opacity', '1')
    await expect(menu).toHaveCSS('transition-duration', '0.42s')
    const bounds = (await menu.boundingBox())!, anchor = (await user.boundingBox())!
    expect(bounds.x + bounds.width).toBeCloseTo(anchor.x + anchor.width, 0)
    expect(bounds.y).toBeCloseTo(anchor.y + anchor.height + 8, 0)
    expect(await page.locator('[data-start-search]').boundingBox()).toEqual(layout)
    const items = menu.getByRole('menuitem')
    await expect(items).toHaveText(locale === 'zh' ? ['登录', '设置'] : ['Sign in', 'Settings'])
    await expect(menu.locator('svg')).toHaveCount(2)
    for (const item of await items.all()) await expect(item).toBeDisabled()
    await items.first().hover()
    await expect.poll(() => items.first().evaluate((el) => getComputedStyle(el).backgroundColor)).not.toBe('rgba(0, 0, 0, 0)')
    const itemBounds = (await items.first().boundingBox())!
    await page.mouse.click(itemBounds.x + itemBounds.width / 2, itemBounds.y + itemBounds.height / 2)
    await expect(user).toHaveAttribute('aria-expanded', 'true')
    await expect(page.getByRole('dialog')).toHaveCount(0)
    expect(page.context().pages()).toHaveLength(1)
    expect(new URL(page.url()).pathname).toBe('/start')
    await user.click()
    await expect(user).toHaveAttribute('aria-expanded', 'false')
    // Inspect the real CSS exit halfway through, before Base UI unmounts it.
    await expect(menu).toHaveAttribute('data-ending-style')
    const exit = await menu.evaluate((el) => {
      const animations = el.getAnimations().filter((a) => ['opacity', 'transform'].includes((a as CSSTransition).transitionProperty))
      for (const animation of animations) { animation.pause(); animation.currentTime = 210 }
      return { count: animations.length, opacity: Number(getComputedStyle(el).opacity), transform: getComputedStyle(el).transform, connected: el.isConnected }
    })
    expect(exit).toMatchObject({ count: 2, connected: true })
    expect(exit.opacity).toBeGreaterThan(0); expect(exit.opacity).toBeLessThan(1)
    expect(exit.transform).not.toBe('none')
    await menu.evaluate((el) => { for (const animation of el.getAnimations()) animation.play() })
    await expect(menu).toHaveCount(0)
    await user.focus(); await user.press('ArrowDown')
    // Base UI initially focuses the popup when every item is disabled. Arrows
    // still visit those placeholders so keyboard users can discover their state.
    await expect(menu).toBeFocused()
    await page.keyboard.press('ArrowDown')
    await expect(items.first()).toBeFocused()
    await expect(items.first()).toHaveCSS('outline-style', 'solid')
    await items.first().press('ArrowDown')
    await expect(items.last()).toBeFocused()
    await expect(items.last()).toHaveCSS('outline-width', '2px')
    await items.last().press('Enter')
    await expect(user).toHaveAttribute('aria-expanded', 'true')
    await page.keyboard.press('Escape')
    await expect(menu).toHaveCount(0)
    await expect(user).toBeFocused()
    await user.click(); await expect(menu).toHaveCSS('opacity', '1')
    await page.getByRole('searchbox').click()
    await expect(menu).toHaveCount(0)
    await expect(page.getByRole('searchbox')).toBeFocused()
  }
  await page.emulateMedia({ reducedMotion: 'reduce' })
  const user = page.getByRole('button', { name: 'User menu', exact: true })
  await user.click()
  await expect(page.getByRole('menu')).toHaveCSS('transition-duration', '0s')
  await page.keyboard.press('Escape')
  await expect(page.getByRole('menu')).toHaveCount(0)
  await expect(user).toBeFocused()
})
